// player/main.go
//
// Aplicación de consola que representa a un jugador.
// Permite: unirse a la cola de emparejamiento y consultar su estado.
//
// Librerías externas permitidas: google.golang.org/grpc y credentials/insecure
// (tal como exige el enunciado para usar gRPC).

package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/vimsent/L3/internal/clocks"
	slog "github.com/vimsent/L3/internal/log"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	// Cambia esta ruta al paquete que generó protoc.
	matchmakingpb "github.com/vimsent/L3/proto"
)

// Constantes de menú.
const (
	menuJoinQueue   = "1"
	menuGetStatus   = "2"
	menuExit        = "3"
	defaultGameMode = "1v1"
)

var localClock *clocks.Vector

func main() {
	// ──────────────────────────────────────────────────────────────────────────────
	// 1. Configuración inicial ─ ID de jugador y dirección del Matchmaker
	// ──────────────────────────────────────────────────────────────────────────────
	playerID := os.Getenv("PLAYER_ID")
	if playerID == "" {
		// Asignamos ID determinista con prefijo Player + número aleatorio.
		rand.Seed(time.Now().UnixNano())
		playerID = fmt.Sprintf("Player%d", rand.Intn(10000))
	}
	go func() {
		localClock = clocks.New(playerID)
		slog.Info("Clock inicial %s", localClock.String())
	}()

	matchmakerAddr := os.Getenv("MATCHMAKER_ADDR")
	if matchmakerAddr == "" {
		matchmakerAddr = "localhost:50051"
	}

	log.Printf("[Player %s] Iniciando. Matchmaker: %s\n", playerID, matchmakerAddr)

	// ──────────────────────────────────────────────────────────────────────────────
	// 2. Conexión gRPC (insegura para laboratorio, sin TLS)
	// ──────────────────────────────────────────────────────────────────────────────
	conn, err := grpc.Dial(
		matchmakerAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(), // Espera la conexión (útil al arrancar todo con Docker Compose)
	)
	if err != nil {
		log.Fatalf("[Player %s] No se pudo conectar al Matchmaker: %v", playerID, err)
	}
	defer conn.Close()

	client := matchmakingpb.NewMatchmakerClient(conn)

	// Contexto raiz con cancelación al recibir SIGINT/SIGTERM.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go handleOSSignals(cancel)

	// ──────────────────────────────────────────────────────────────────────────────
	// 3. Bucle de menú interactivo
	// ──────────────────────────────────────────────────────────────────────────────
	reader := bufio.NewReader(os.Stdin)
	for {
		printMenu()
		fmt.Print("> ")

		input, _ := reader.ReadString('\n')
		choice := strings.TrimSpace(input)

		switch choice {
		case menuJoinQueue:
			if err := queuePlayer(ctx, client, playerID); err != nil {

				log.Printf("[Player %s] Error al unirse a la cola: %v\n", playerID, err)
			}
		case menuGetStatus:
			if err := getPlayerStatus(ctx, client, playerID); err != nil {
				log.Printf("[Player %s] Error al consultar estado: %v\n", playerID, err)
			}
		case menuExit:
			log.Printf("[Player %s] Saliendo...\n", playerID)
			return
		default:
			fmt.Println("Opción inválida. Intenta nuevamente.")
		}
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Funciones de negocio
// ──────────────────────────────────────────────────────────────────────────────

// queuePlayer realiza llamada RPC QueuePlayer.
func queuePlayer(ctx context.Context, client matchmakingpb.MatchmakerClient, playerID string) error {

	req := &matchmakingpb.PlayerInfoRequest{
		PlayerId: playerID,
		GameMode: defaultGameMode,
	}
	go func() {
		localClock.Tick(playerID)
	}()
	req.Clock = clocksToProto(localClock)

	start := time.Now()
	res, err := client.QueuePlayer(ctx, req)
	if err != nil {
		return err
	}
	localClock.Merge(protoToClocks(res.GetClock()))

	log.Printf("[Player %s] QueuePlayer ➜ status=%s • msg=%q • t=%s\n",
		playerID, res.GetStatus(), res.GetMessage(), time.Since(start))
	return nil
}

// getPlayerStatus realiza llamada RPC GetPlayerStatus.
func getPlayerStatus(ctx context.Context, client matchmakingpb.MatchmakerClient, playerID string) error {
	req := &matchmakingpb.PlayerStatusRequest{
		PlayerId: playerID,
	}

	start := time.Now()
	localClock.Tick(playerID)
	req.Clock = clocksToProto(localClock)

	res, err := client.GetPlayerStatus(ctx, req)
	if err != nil {
		return err
	}
	localClock.Merge(protoToClocks(res.GetClock()))

	// Formateamos salida legible.
	state := res.GetState()
	matchID := res.GetMatchId()
	serverAddr := res.GetServerAddr()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[Player %s] Estado actual: %s", playerID, state))
	if state == "IN_MATCH" {
		sb.WriteString(fmt.Sprintf(" • MatchID=%s • GameServer=%s", matchID, serverAddr))
	}
	log.Printf("%s • t=%s\n", sb.String(), time.Since(start))
	return nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Utilidades
// ──────────────────────────────────────────────────────────────────────────────

func printMenu() {
	fmt.Println()
	fmt.Println("═════════ Menú Jugador ═════════")
	fmt.Printf("%s) Unirse a la cola de emparejamiento\n", menuJoinQueue)
	fmt.Printf("%s) Consultar estado\n", menuGetStatus)
	fmt.Printf("%s) Salir\n", menuExit)
	fmt.Println("════════════════════════════════")
}

// handleOSSignals cancela el contexto al recibir señales de cierre.
func handleOSSignals(cancel context.CancelFunc) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, os.Kill)
	<-c
	log.Println("Señal de cierre recibida: terminando proceso…")
	cancel()
	time.Sleep(500 * time.Millisecond) // Breve espera para limpieza.
	os.Exit(0)
}

// ──────────────────────────────────────────────────────────────────────────────
// Extras: transformación segura de string->int para posibles extensiones
// (ej. parametrizar tamaño de partida, etc.).
// ──────────────────────────────────────────────────────────────────────────────
func parseIntOrDefault(s string, def int) int {
	if v, err := strconv.Atoi(s); err == nil {
		return v
	}
	return def
}
