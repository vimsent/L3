# Sistema Distribuido de Emparejamiento Multijugador  
Laboratorio 3 · Sistemas Distribuidos · UTFSM · 2025-1

> Proyecto escrito en **Go ≥ 1.22** usando **gRPC**, **Protocol Buffers** y **Docker**.  
> Compatible con ejecución local (terminales independientes) y en múltiples VMs mediante variables de entorno.

---

## 1 · Clonar el repositorio

```bash
git clone https://github.com/<tu-usuario>/<repo-lab3>.git
cd <repo-lab3>
```
---

## 2 · Instalar dependencias de sistema
| Componente                         | Versión mínima        | Linux (apt)                                                                                                                           | macOS (brew)            | Windows + WSL                                         |
| ---------------------------------- | --------------------- | ------------------------------------------------------------------------------------------------------------------------------------- | ----------------------- | ----------------------------------------------------- |
| Go                                 | **1.22**              | `sudo apt install golang`                                                                                                             | `brew install go`       | Instalar desde [https://go.dev/dl](https://go.dev/dl) |
| protoc                             | **3.21**              | `sudo apt install protobuf-compiler`                                                                                                  | `brew install protobuf` | Descarga binarios                                     |
| protoc-gen-go & protoc-gen-go-grpc | emparejados a Go 1.22 | `go install google.golang.org/protobuf/cmd/protoc-gen-go@latest`<br>`go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest` | Igual                   | Igual                                                 |
| Docker + Docker Compose v2         | cualquiera reciente   | [https://docs.docker.com/engine/install/](https://docs.docker.com/engine/install/)                                                    | `brew install docker`   | Docker Desktop                                        |

Asegúrate de que $GOPATH/bin (por defecto ~/go/bin) esté en tu $PATH para que las herramientas protoc-gen-* sean visibles.

---
## Comprueba versiones:
```bash
go version
protoc --version
docker compose version   # («compose» es sub-comando de Docker v2)
```
## 3 · Generar código gRPC:
```bash
make proto        # ejecuta: protoc --go_out=. --go-grpc_out=. ./proto/matchmaking.proto

```
## 4 · Compilar binarios nativos (opcional)
```bash
# Compila todos los paquetes
go run ./player/main.go        # jugador
go run ./gameserver/main.go    # servidor de partida
go run ./matchmaker/main.go    # matchmaker
go run ./adminclient/main.go   # cliente admin

```
Al ejecutar localmente exporta la variable de entorno que apunte al Matchmaker:
```bash
export MATCHMAKER_ADDR=localhost:50051
```

## 5 · Construir imágenes Docker:
```bash
make docker-build   # equivale a: docker compose build
```
Esto produce las imágenes:

· matchmaker:latest

· player:latest

· gameserver:latest

· adminclient:latest

## 6 · Ejecución local con Docker Compose
```bash
make docker-up      # igual a: docker compose up
```
· Expone el Matchmaker en localhost:50051.
· Cada contenedor toma su rol (2 players, 3 servers, adminclient).

Detenerlos:
```bash
make docker-down    # docker compose down
```

## 7 · Despliegue en las Máquinas Virtuales de evaluación
La cátedra solicita 4 VMs. En cada VM basta un único comando make:
| VM      | Entidades                | Comando           |
| ------- | ------------------------ | ----------------- |
| **VM1** | Player 1 & GameServer 1  | `make deploy-vm1` |
| **VM2** | Player 2 & GameServer 2  | `make deploy-vm2` |
| **VM3** | GameServer 3             | `make deploy-vm3` |
| **VM4** | Matchmaker & AdminClient | `make deploy-vm4` |

Los targets anteriores están definidos en el Makefile y ejecutan:

docker network create lab3-net (si no existe).

docker run --name <servicio> --env MATCHMAKER_ADDR=<host>:50051 --network lab3-net <imagen>

El MATCHMAKER_ADDR apunta a la IP/hostname de VM 4 dentro de la red privada de las VMs, p.ej. 10.11.4.4:50051.
Ajusta la IP si tu infraestructura usa otra sub-red.

## 8 · Variables de entorno clave
| Variable          | Quién la usa                    | Valor por defecto | Ejemplo en producción |
| ----------------- | ------------------------------- | ----------------- | --------------------- |
| `MATCHMAKER_ADDR` | Player, GameServer, AdminClient | `localhost:50051` | `10.11.4.4:50051`     |
| `SERVER_ID`       | GameServer                      | random UUID       | `GameServer3`         |
| `PLAYER_ID`       | Player                          | random UUID       | `Player2`             |

*No es necesario modificar código: basta exportar estas variables o pasarlas con -e a docker run.

## 9 · Pruebas rápidas:
```bash
# Entra al adminclient
docker exec -it adminclient /app/adminclient   # menú interactivo

# Ver logs en tiempo real de un GameServer
docker logs -f gameserver1
```

## 10 · Problemas frecuentes:
| Síntoma                         | Causa probable                | Solución                                             |
| ------------------------------- | ----------------------------- | ---------------------------------------------------- |
| `panic: failed to dial`         | `MATCHMAKER_ADDR` mal seteado | Exporta la variable correcta o usa `--env` en Docker |
| `protoc: command not found`     | No instalaste `protoc`        | Ver sección 2                                        |
| Docker dice «network not found» | Red eliminada manualmente     | `docker network create lab3-net`                     |

## 11 · Autores:
· Vicente Luongo Codecido - 202073637-5
