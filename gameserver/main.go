// gameserver/main.go
//
// Implementación completa de la lógica del Game Server solicitada en el
// laboratorio.  Este proceso expone el RPC `AssignMatch` para que el
// Matchmaker le asigne partidas y, a su vez, actúa como **cliente gRPC**
// hacia el Matchmaker para ir notificando su propio estado
// (`DISPONIBLE`, `OCUPADO`, `CAIDO`).
//
// ▸ Variables de entorno reconocidas
//   ─────────────────────────────────
//   • SERVER_ID         → ID lógico único de la instancia
//                         (p.e. "GameServer1").        [def: GameServer-<rand>]
//   • PORT              → Puerto TCP que expondrá el servicio gRPC. [def: 60051]
//   • MATCHMAKER_ADDR   → host:puerto donde escucha el Matchmaker. [def: localhost:50051]
//   • CRASH_PROB        → Probabilidad (0-1) de “caerse” tras terminar una
//                         partida, para testear tolerancia a fallos.
//                         Se ignora si no se puede convertir a float. [def: 0.1]
//
// ▸ Librerías externas
//   ──────────────────
//   Se utilizan únicamente los paquetes permitidos por el enunciado más los
//   de gRPC / Protobuf oficiales.
//
// ▸ Resumen de flujo
//   ────────────────
//   1. Arranca, crea conexión gRPC cliente con Matchmaker.
//   2. Envía UPDATE( DISPO ) para registrarse.
//   3. Levanta su propio servidor gRPC (implementa AssignMatch).
//   4. Cada vez que recibe AssignMatch:
//        ▸ cambia a OCUPADO, notifica,
//        ▸ simula partida (10-20 s),
//        ▸ con probabilidad CRASH_PROB => simula caída: `os.Exit(1)`,
//        ▸ de lo contrario vuelve a DISPO y notifica.
//   5. Maneja SIGINT/SIGTERM enviando cambio a CAIDO antes de cerrar.
//

package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	pb "github.com/vimsent/L3/proto" // => generado con `go_package` = "github.com/yourrepo/proto;pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// ───────────────────────────────────────────────────────────────────────────────
// Constantes y utilidades
// ───────────────────────────────────────────────────────────────────────────────

const (
	defaultPort          = 60051
	defaultMMAddr        = "localhost:50051"
	defaultCrashProb     = 0.1
	statusAvailable      = "DISPONIBLE"
	statusBusy           = "OCUPADO"
	statusCrashed        = "CAIDO"
	matchDurationMinSecs = 10
	matchDurationMaxSecs = 20
)

// ───────────────────────────────────────────────────────────────────────────────
// Estructura principal del GameServer
// ───────────────────────────────────────────────────────────────────────────────

type gameServer struct {
	pb.UnimplementedGameServerServer

	id            string
	address       string
	crashProb     float64
	matchmakerCli pb.MatchmakerClient

	mu            sync.Mutex
	currentStatus string
	currentMatch  string
}

// newGameServer crea la instancia, registra “DISPONIBLE” y devuelve el puntero.
func newGameServer(id, listenAddr string, crashProb float64, mmcli pb.MatchmakerClient) *gameServer {
	gs := &gameServer{
		id:            id,
		address:       listenAddr,
		crashProb:     crashProb,
		matchmakerCli: mmcli,
		currentStatus: statusAvailable,
	}
	// Primer registro en el Matchmaker.
	if err := gs.sendStatus(statusAvailable, ""); err != nil {
		log.Printf("[GameServer %s] ERROR registrando en Matchmaker: %v", gs.id, err)
	}
	return gs
}

// AssignMatch es el RPC que invoca el Matchmaker.
func (gs *gameServer) AssignMatch(ctx context.Context, req *pb.AssignMatchRequest) (*pb.AssignMatchResponse, error) {
	gs.mu.Lock()
	if gs.currentStatus != statusAvailable {
		gs.mu.Unlock()
		return &pb.AssignMatchResponse{
			StatusCode: pb.AssignMatchResponse_BUSY,
			Message:    "Game server not available",
		}, nil
	}

	// Transición a OCUPADO.
	gs.currentStatus = statusBusy
	gs.currentMatch = req.GetMatchId()
	gs.mu.Unlock()

	log.Printf("[GameServer %s] Recibiendo partida %s con jugadores %v", gs.id, req.GetMatchId(), req.GetPlayerIds())

	// Notifica inmediatamente al Matchmaker que está ocupado.
	if err := gs.sendStatus(statusBusy, gs.currentMatch); err != nil {
		log.Printf("[GameServer %s] WARNING: no pude notificar estado OCUPADO: %v", gs.id, err)
	}

	// Simulación de la partida en una goroutine para no bloquear el RPC.
	go gs.simulateMatch(req.GetMatchId())

	return &pb.AssignMatchResponse{
		StatusCode: pb.AssignMatchResponse_OK,
		Message:    "Match accepted",
	}, nil
}

// simulateMatch duerme entre 10-20 s y luego actualiza estado.
func (gs *gameServer) simulateMatch(matchID string) {
	duration := time.Duration(matchDurationMinSecs+rand.Intn(matchDurationMaxSecs-matchDurationMinSecs+1)) * time.Second
	log.Printf("[GameServer %s] Simulando partida %s durante %v", gs.id, matchID, duration)
	time.Sleep(duration)

	// ¿Se “cae”?
	if rand.Float64() < gs.crashProb {
		log.Printf("[GameServer %s] ¡Simulando CAÍDA después de la partida %s!", gs.id, matchID)
		_ = gs.sendStatus(statusCrashed, "")
		os.Exit(1)
		return
	}

	// Si no se cayó, vuelve a DISPONIBLE.
	gs.mu.Lock()
	gs.currentStatus = statusAvailable
	gs.currentMatch = ""
	gs.mu.Unlock()

	if err := gs.sendStatus(statusAvailable, ""); err != nil {
		log.Printf("[GameServer %s] ERROR al volver a DISPONIBLE: %v", gs.id, err)
	} else {
		log.Printf("[GameServer %s] Partida %s finalizada. Estado DISPONIBLE.", gs.id, matchID)
	}
}

// sendStatus encapsula la llamada UpdateServerStatus al Matchmaker.
func (gs *gameServer) sendStatus(status, matchID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := gs.matchmakerCli.UpdateServerStatus(ctx, &pb.ServerStatusUpdateRequest{
		ServerId:  gs.id,
		NewStatus: status,
		Address:   gs.address,
		MatchId:   matchID,
	})
	return err
}

// ───────────────────────────────────────────────────────────────────────────────
// Funciones auxiliares de inicialización
// ───────────────────────────────────────────────────────────────────────────────

// loadEnv obtiene configuración desde las variables de entorno.
func loadEnv() (id string, port int, matchmakerAddr string, crashProb float64) {
	id = os.Getenv("SERVER_ID")
	if id == "" {
		// Genera ID pseudoaleatorio si no se proporciona.
		id = fmt.Sprintf("GameServer-%d", rand.Intn(10000))
	}

	port = defaultPort
	if p := os.Getenv("PORT"); p != "" {
		if n, err := strconv.Atoi(p); err == nil {
			port = n
		}
	}

	matchmakerAddr = os.Getenv("MATCHMAKER_ADDR")
	if matchmakerAddr == "" {
		matchmakerAddr = defaultMMAddr
	}

	crashProb = defaultCrashProb
	if cpStr := os.Getenv("CRASH_PROB"); cpStr != "" {
		if cp, err := strconv.ParseFloat(cpStr, 64); err == nil && cp >= 0 && cp <= 1 {
			crashProb = cp
		}
	}
	return
}

// ───────────────────────────────────────────────────────────────────────────────
// main
// ───────────────────────────────────────────────────────────────────────────────

func main() {
	rand.Seed(time.Now().UnixNano())

	// 1. Cargar configuración.
	id, port, mmAddr, crashProb := loadEnv()
	listenAddr := fmt.Sprintf("0.0.0.0:%d", port)

	// 2. Crear conexión al Matchmaker.
	connMM, err := grpc.Dial(mmAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("[GameServer %s] No pude conectar al Matchmaker en %s: %v", id, mmAddr, err)
	}
	defer connMM.Close()
	mmClient := pb.NewMatchmakerClient(connMM)

	// 3. Crear GameServer y registrar.
	gs := newGameServer(id, listenAddr, crashProb, mmClient)

	// 4. Levantar servidor gRPC local.
	lis, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatalf("[GameServer %s] No pude escuchar en %s: %v", id, listenAddr, err)
	}

	s := grpc.NewServer()
	pb.RegisterGameServerServer(s, gs) // registra servicio

	log.Printf("[GameServer %s] Escuchando en %s (Matchmaker: %s, CrashProb: %.2f)",
		id, listenAddr, mmAddr, crashProb)

	// 5. Manejar señales para apagado limpio.
	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
		<-ch
		log.Printf("[GameServer %s] Recibida señal de terminación. Notificando CAIDO…", id)
		_ = gs.sendStatus(statusCrashed, "")
		s.GracefulStop()
	}()

	// 6. ¡A servir!
	if err := s.Serve(lis); err != nil {
		log.Fatalf("[GameServer %s] Error en Serve(): %v", id, err)
	}
}
