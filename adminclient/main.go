// adminclient/main.go
package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"time"

	pb "github.com/vimsent/L3/proto" // ⬅️  ajusta esta ruta a tu módulo

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// ===== Utilidades de impresión =====

func printSystemStatus(resp *pb.SystemStatusResponse) {
	fmt.Println("\n==================== ESTADO DEL SISTEMA ====================")

	fmt.Println("\n🖥  Servidores de Partida")
	if len(resp.Servers) == 0 {
		fmt.Println("  (ninguno registrado)")
	}
	for _, s := range resp.Servers {
		fmt.Printf("  - ID: %-12s | Estado: %-10s | Addr: %-18s | Partida: %s\n",
			s.Id, s.Status.String(), s.Address, s.CurrentMatchId)
	}

	fmt.Println("\n🎮  Jugadores en Cola")
	if len(resp.Queue) == 0 {
		fmt.Println("  (no hay jugadores esperando)")
	}
	for _, q := range resp.Queue {
		fmt.Printf("  - PlayerID: %-12s | Segundos en cola: %d\n",
			q.PlayerId, q.SecondsInQueue)
	}

	fmt.Println("============================================================\n")
}

// ===== Conversión de texto a enum =====

func parseServerStatus(input string) (pb.ServerStatus, bool) {
	switch strings.ToUpper(strings.TrimSpace(input)) {
	case "DISPONIBLE":
		return pb.ServerStatus_DISPONIBLE, true
	case "OCUPADO":
		return pb.ServerStatus_OCUPADO, true
	case "CAIDO":
		return pb.ServerStatus_CAIDO, true
	default:
		return pb.ServerStatus_UNKNOWN, false
	}
}

// ===== Menú principal =====

func adminMenu(client pb.MatchmakerClient) {
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Println("=========== CLIENTE ADMINISTRADOR ===========")
		fmt.Println("1) Ver estado completo del sistema")
		fmt.Println("2) Cambiar estado de un servidor")
		fmt.Println("3) Salir")
		fmt.Print("Selecciona una opción: ")

		optionRaw, _ := reader.ReadString('\n')
		option := strings.TrimSpace(optionRaw)

		switch option {
		case "1":
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			resp, err := client.AdminGetSystemStatus(ctx, &pb.AdminRequest{})
			if err != nil {
				log.Printf("[AdminClient] ERROR al obtener estado del sistema: %v\n", err)
				continue
			}
			printSystemStatus(resp)

		case "2":
			fmt.Print("   ➤ ID del servidor: ")
			serverIDRaw, _ := reader.ReadString('\n')
			serverID := strings.TrimSpace(serverIDRaw)

			fmt.Print("   ➤ Nuevo estado (DISPONIBLE/OCUPADO/CAIDO): ")
			statusRaw, _ := reader.ReadString('\n')

			newStatus, ok := parseServerStatus(statusRaw)
			if !ok {
				fmt.Println("   ❌  Estado no reconocido. Intenta nuevamente.")
				continue
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			_, err := client.AdminUpdateServerState(ctx, &pb.AdminServerUpdateRequest{
				ServerId:  serverID,
				NewStatus: newStatus,
			})
			if err != nil {
				log.Printf("[AdminClient] ERROR al actualizar estado: %v\n", err)
			} else {
				fmt.Println("   ✅  Estado actualizado con éxito.")
			}

		case "3":
			fmt.Println("Saliendo del cliente administrador. ¡Hasta pronto!")
			return

		default:
			fmt.Println("   ❌  Opción inválida. Intenta nuevamente.")
		}
	}
}

// ===== main =====

func main() {
	// 1. Resolver dirección del Matchmaker
	addr := os.Getenv("MATCHMAKER_ADDR")
	if addr == "" {
		addr = "localhost:50051" // valor por defecto para entorno local
	}

	// 2. Conectar vía gRPC
	conn, err := grpc.Dial(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		log.Fatalf("[AdminClient] No pudo conectar al Matchmaker (%s): %v", addr, err)
	}
	defer conn.Close()

	client := pb.NewMatchmakerClient(conn)
	log.Printf("[AdminClient] Conectado a Matchmaker en %s\n", addr)

	// 3. Manejar Ctrl+C para salir limpiamente
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// 4. Lanzar menú en una goroutine para poder cancelar con señal
	done := make(chan struct{})
	go func() {
		adminMenu(client)
		close(done)
	}()

	select {
	case <-ctx.Done():
		fmt.Println("\n[AdminClient] Señal de interrupción recibida. Cerrando...")
	case <-done:
		// menú terminó normalmente
	}
}
