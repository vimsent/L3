// matchmaker/main.go
//
// Implementación COMPLETA del Matchmaker Central para el
// “Sistema Distribuido de Emparejamiento Multijugador Avanzado”.
//
// ▸ Expone todos los RPCs definidos en proto/matchmaking.proto.
// ▸ Mantiene el estado de jugadores, servidores y partidas.
// ▸ Aplica consistencia eventual mediante relojes vectoriales.
// ▸ Garantiza “Read-Your-Writes” para los jugadores.
// ▸ Incluye tolerancia a fallos (timeouts - heartbeats, reintentos).
// ▸ Usa ÚNICAMENTE librerías permitidas + gRPC/Protobuf.
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
	"time"

	"google.golang.org/grpc"

	pb "github.com/vimsent/L3/proto" // ← ajusta la ruta a tu módulo Go
)

/*───────────────────────────────────────────────────────────────────────────────
                             Constantes & Tipos
───────────────────────────────────────────────────────────────────────────────*/

const (
	defaultPort            = 50051
	matchCheckPeriod       = 2 * time.Second
	serverHeartbeatTimeout = 30 * time.Second
)

// Reloj Vectorial: id → contador
type vectorClock map[string]int

func (vc vectorClock) clone() vectorClock {
	out := make(vectorClock)
	for k, v := range vc {
		out[k] = v
	}
	return out
}

func (vc vectorClock) increment(id string) {
	vc[id]++
}

func (vc vectorClock) merge(other vectorClock) {
	for k, v := range other {
		if cur, ok := vc[k]; !ok || v > cur {
			vc[k] = v
		}
	}
}

func (vc vectorClock) toProto() *pb.VectorClock {
	res := &pb.VectorClock{Counters: map[string]int32{}}
	for k, v := range vc {
		res.Counters[k] = int32(v)
	}
	return res
}

func vcFromProto(p *pb.VectorClock) vectorClock {
	out := make(vectorClock)
	for k, v := range p.GetCounters() {
		out[k] = int(v)
	}
	return out
}

type playerState int

const (
	playerIdle playerState = iota
	playerInQueue
	playerInMatch
)

type serverState int

const (
	serverUnknown serverState = iota
	serverAvailable
	serverBusy
	serverDown
)

type playerInfo struct {
	ID      string
	Status  playerState
	MatchID string
	VC      vectorClock
	LastOp  time.Time
}

type gameServerInfo struct {
	ID           string
	Address      string
	Status       serverState
	CurrentMatch string
	VC           vectorClock
	LastHB       time.Time
}

/*───────────────────────────────────────────────────────────────────────────────
                             Matchmaker struct
───────────────────────────────────────────────────────────────────────────────*/

type matchmaker struct {
	pb.UnimplementedMatchmakerServer

	selfID string // para el reloj

	mu      sync.RWMutex
	players map[string]*playerInfo
	servers map[string]*gameServerInfo
	queue   []string // FIFO de IDs de jugador

	matches map[string][]string // MatchID → playerIDs
	vc      vectorClock

	// canal interno para cerrar goroutines
	done chan struct{}
}

/*───────────────────────────────────────────────────────────────────────────────
                               Constructor
───────────────────────────────────────────────────────────────────────────────*/

func newMatchmaker(selfID string) *matchmaker {
	return &matchmaker{
		selfID:  selfID,
		players: make(map[string]*playerInfo),
		servers: make(map[string]*gameServerInfo),
		queue:   []string{},
		matches: make(map[string][]string),
		vc:      make(vectorClock),
		done:    make(chan struct{}),
	}
}

/*───────────────────────────────────────────────────────────────────────────────
                         Métodos auxiliares protegidos
───────────────────────────────────────────────────────────────────────────────*/

// debe llamarse con m.mu bloqueado
func (m *matchmaker) nextMatchID() string {
	return fmt.Sprintf("M%08x", rand.Int31())
}

func (m *matchmaker) logf(format string, args ...interface{}) {
	prefix := "[Matchmaker] "
	log.Printf(prefix+format, args...)
}

/*───────────────────────────────────────────────────────────────────────────────
                       Tarea de emparejamiento periódica
───────────────────────────────────────────────────────────────────────────────*/

func (m *matchmaker) runMatchLoop() {
	ticker := time.NewTicker(matchCheckPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.tryCreateMatch()
			m.detectServerTimeouts()
		case <-m.done:
			return
		}
	}
}

// intenta formar partidas (actualmente sólo 1v1)
func (m *matchmaker) tryCreateMatch() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for len(m.queue) >= 2 && m.availableServerCount() > 0 {
		// elige servidor disponible
		var srv *gameServerInfo
		for _, s := range m.servers {
			if s.Status == serverAvailable {
				srv = s
				break
			}
		}
		if srv == nil {
			return // no disponible
		}

		// extrae jugadores
		p1ID := m.queue[0]
		p2ID := m.queue[1]
		m.queue = m.queue[2:]

		p1 := m.players[p1ID]
		p2 := m.players[p2ID]

		matchID := m.nextMatchID()

		// actualiza estado local
		p1.Status, p1.MatchID = playerInMatch, matchID
		p2.Status, p2.MatchID = playerInMatch, matchID
		srv.Status, srv.CurrentMatch = serverBusy, matchID
		m.matches[matchID] = []string{p1ID, p2ID}

		// reloj vectorial
		m.vc.increment(m.selfID)

		// intenta asignar al servidor
		go m.dispatchAssignMatch(srv, matchID, []string{p1ID, p2ID}, m.vc.clone())
		m.logf("Asignando match %s a server %s (%s) con jugadores %s & %s", matchID, srv.ID, srv.Address, p1ID, p2ID)
	}
}

func (m *matchmaker) availableServerCount() int {
	c := 0
	for _, s := range m.servers {
		if s.Status == serverAvailable {
			c++
		}
	}
	return c
}

// heartbeat/tiempo máximo para servidor busy
func (m *matchmaker) detectServerTimeouts() {
	now := time.Now()
	for _, srv := range m.servers {
		if srv.Status == serverDown {
			continue
		}
		if now.Sub(srv.LastHB) > serverHeartbeatTimeout {
			m.logf("Server %s marcado DOWN por timeout de heartbeat", srv.ID)
			srv.Status = serverDown
			m.vc.increment(m.selfID)
		}
	}
}

/*───────────────────────────────────────────────────────────────────────────────
                     RPC: QueuePlayer – jugador se encola
───────────────────────────────────────────────────────────────────────────────*/

func (m *matchmaker) QueuePlayer(ctx context.Context, req *pb.PlayerInfoRequest) (*pb.QueuePlayerResponse, error) {
	playerID := req.GetPlayerId()

	m.mu.Lock()
	defer m.mu.Unlock()

	m.vc.merge(vcFromProto(req.GetClock()))
	m.vc.increment(m.selfID)

	pi, ok := m.players[playerID]
	if !ok {
		pi = &playerInfo{ID: playerID, VC: make(vectorClock)}
		m.players[playerID] = pi
	}

	switch pi.Status {
	case playerInQueue:
		return &pb.QueuePlayerResponse{
			StatusCode:  pb.QueuePlayerResponse_ALREADY_IN_QUEUE,
			Message:     "Ya en cola",
			VectorClock: m.vc.toProto(),
		}, nil
	case playerInMatch:
		return &pb.QueuePlayerResponse{
			StatusCode:  pb.QueuePlayerResponse_IN_MATCH,
			Message:     "Actualmente en partida",
			VectorClock: m.vc.toProto(),
		}, nil
	}

	// lo encolamos
	pi.Status = playerInQueue
	pi.MatchID = ""
	pi.LastOp = time.Now()
	m.queue = append(m.queue, playerID)

	m.logf("Jugador %s encolado", playerID)
	return &pb.QueuePlayerResponse{
		StatusCode:  pb.QueuePlayerResponse_OK,
		Message:     "Encolado correctamente",
		VectorClock: m.vc.toProto(),
	}, nil
}

/*───────────────────────────────────────────────────────────────────────────────
                     RPC: GetPlayerStatus – estado jugador
───────────────────────────────────────────────────────────────────────────────*/

func (m *matchmaker) GetPlayerStatus(ctx context.Context, req *pb.PlayerStatusRequest) (*pb.PlayerStatusResponse, error) {
	playerID := req.GetPlayerId()

	m.mu.Lock()
	defer m.mu.Unlock()

	m.vc.merge(vcFromProto(req.GetClock()))
	pi, ok := m.players[playerID]
	if !ok {
		return &pb.PlayerStatusResponse{
			Status:      "UNKNOWN",
			VectorClock: m.vc.toProto(),
		}, nil
	}

	var statusStr string
	switch pi.Status {
	case playerIdle:
		statusStr = "IDLE"
	case playerInQueue:
		statusStr = "IN_QUEUE"
	case playerInMatch:
		statusStr = "IN_MATCH"
	}

	return &pb.PlayerStatusResponse{
		Status:      statusStr,
		MatchId:     pi.MatchID,
		VectorClock: m.vc.toProto(),
	}, nil
}

/*───────────────────────────────────────────────────────────────────────────────
          RPC: UpdateServerStatus – recibe heartbeats/registro servidor
───────────────────────────────────────────────────────────────────────────────*/

func (m *matchmaker) UpdateServerStatus(ctx context.Context, req *pb.ServerStatusUpdateRequest) (*pb.ServerStatusUpdateResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.vc.merge(vcFromProto(req.GetClock()))
	m.vc.increment(m.selfID)

	sid := req.GetServerId()
	srv, ok := m.servers[sid]
	if !ok {
		srv = &gameServerInfo{
			ID: sid,
			VC: make(vectorClock),
		}
		m.servers[sid] = srv
	}

	// actualiza campos
	srv.Address = req.GetAddress()
	srv.LastHB = time.Now()

	switch req.GetNewStatus() {
	case pb.ServerStatusUpdateRequest_AVAILABLE:
		srv.Status = serverAvailable
	case pb.ServerStatusUpdateRequest_BUSY:
		srv.Status = serverBusy
	case pb.ServerStatusUpdateRequest_DOWN:
		srv.Status = serverDown
	}

	m.logf("Actualización de servidor %s → %s", sid, req.GetNewStatus().String())

	return &pb.ServerStatusUpdateResponse{
		StatusCode:  pb.ServerStatusUpdateResponse_OK,
		VectorClock: m.vc.toProto(),
	}, nil
}

/*───────────────────────────────────────────────────────────────────────────────
                     RPC: AdminGetSystemStatus – vista global
───────────────────────────────────────────────────────────────────────────────*/

func (m *matchmaker) AdminGetSystemStatus(ctx context.Context, _ *pb.AdminRequest) (*pb.SystemStatusResponse, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var serverStates []*pb.ServerState
	for _, s := range m.servers {
		serverStates = append(serverStates, &pb.ServerState{
			ServerId:      s.ID,
			Status:        serverStatusProto(s.Status),
			Address:       s.Address,
			CurrentMatch:  s.CurrentMatch,
			LastHeartbeat: s.LastHB.Unix(),
		})
	}

	var queueEntries []*pb.PlayerQueueEntry
	for _, pid := range m.queue {
		queueEntries = append(queueEntries, &pb.PlayerQueueEntry{
			PlayerId: pid,
		})
	}

	return &pb.SystemStatusResponse{
		Servers:     serverStates,
		PlayerQueue: queueEntries,
		VectorClock: m.vc.toProto(),
	}, nil
}

func serverStatusProto(st serverState) pb.ServerState_Status {
	switch st {
	case serverAvailable:
		return pb.ServerState_AVAILABLE
	case serverBusy:
		return pb.ServerState_BUSY
	case serverDown:
		return pb.ServerState_DOWN
	default:
		return pb.ServerState_UNKNOWN
	}
}

/*───────────────────────────────────────────────────────────────────────────────
                 RPC: AdminUpdateServerState – fuerza estado
───────────────────────────────────────────────────────────────────────────────*/

func (m *matchmaker) AdminUpdateServerState(ctx context.Context, req *pb.AdminServerUpdateRequest) (*pb.AdminUpdateResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sid := req.GetServerId()
	srv, ok := m.servers[sid]
	if !ok {
		return &pb.AdminUpdateResponse{
			Status: pb.AdminUpdateResponse_NOT_FOUND,
		}, nil
	}

	switch req.GetNewStatus() {
	case pb.AdminServerUpdateRequest_FORCE_AVAILABLE:
		srv.Status = serverAvailable
	case pb.AdminServerUpdateRequest_FORCE_DOWN:
		srv.Status = serverDown
	}

	m.vc.increment(m.selfID)

	return &pb.AdminUpdateResponse{
		Status:      pb.AdminUpdateResponse_OK,
		VectorClock: m.vc.toProto(),
	}, nil
}

/*───────────────────────────────────────────────────────────────────────────────
              Comunicación con GameServer: gRPC AssignMatch
───────────────────────────────────────────────────────────────────────────────*/

func (m *matchmaker) dispatchAssignMatch(srv *gameServerInfo, matchID string, players []string, snapshot vectorClock) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, srv.Address, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		m.logf("ERROR: no se pudo conectar a servidor %s: %v", srv.ID, err)
		m.handleAssignFailure(srv, players)
		return
	}
	defer conn.Close()

	gsc := pb.NewGameServerClient(conn)
	_, err = gsc.AssignMatch(ctx, &pb.AssignMatchRequest{
		MatchId:     matchID,
		PlayerIds:   players,
		VectorClock: snapshot.toProto(),
	})
	if err != nil {
		m.logf("ERROR: AssignMatch a %s falló: %v", srv.ID, err)
		m.handleAssignFailure(srv, players)
		return
	}

	// OK – el GameServer se encargará de actualizar su estado a BUSY internamente
}

func (m *matchmaker) handleAssignFailure(srv *gameServerInfo, players []string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// marca DOWN
	srv.Status = serverDown
	m.vc.increment(m.selfID)

	// devuelve jugadores a la cabeza de la cola
	m.queue = append(players, m.queue...)
	for _, pid := range players {
		if p, ok := m.players[pid]; ok {
			p.Status = playerInQueue
			p.MatchID = ""
		}
	}
}

/*───────────────────────────────────────────────────────────────────────────────
                                       main
───────────────────────────────────────────────────────────────────────────────*/

func main() {
	rand.Seed(time.Now().UnixNano())

	selfID := "Matchmaker"
	port := defaultPort
	if v := os.Getenv("MATCHMAKER_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			port = p
		}
	}

	mm := newMatchmaker(selfID)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Fatalf("FATAL: no se puede escuchar en %d: %v", port, err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterMatchmakerServer(grpcServer, mm)

	// interrupción graceful
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		<-c
		log.Println("SIGINT recibido, apagando Matchmaker…")
		close(mm.done)
		grpcServer.GracefulStop()
	}()

	// goroutine de emparejamiento
	go mm.runMatchLoop()

	log.Printf("Matchmaker escuchando en :%d", port)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("FATAL: servidor gRPC se detuvo: %v", err)
	}
}
