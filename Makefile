# ------------------------------------------------------------
# VARIABLES
# ------------------------------------------------------------
# Carpeta donde viven los .proto
PROTO_DIR       := proto

# Alias para docker-compose (v2 usa `docker compose`, v1 `docker-compose`)
COMPOSE         := docker compose

# ------------------------------------------------------------
# LISTA DE TARGETS PÚBLICOS
# ------------------------------------------------------------
.PHONY: proto docker-build docker-up docker-down clean \
        run-player run-gameserver run-matchmaker run-admin \
        docker-jugador1-servidor1 docker-jugador2-servidor2 \
        docker-servidor3 docker-admin-matchmaker \
        deploy-vm1 deploy-vm2 deploy-vm3 deploy-vm4

# ------------------------------------------------------------
# 1) GENERAR CÓDIGO Go A PARTIR DE LOS .proto
# ------------------------------------------------------------
proto:
	@echo "🛠  Generando código gRPC/Protobuf…"
	protoc \
	  --go_opt=paths=source_relative --go_out=. \
	  --go-grpc_opt=paths=source_relative --go-grpc_out=. \
	  $(PROTO_DIR)/*.proto

# ------------------------------------------------------------
# 2) EJECUCIÓN LOCAL (SIN DOCKER) PARA DEBUG
# ------------------------------------------------------------
run-player: proto
	@echo "🏃  Ejecutando PLAYER en local…"
	MATCHMAKER_ADDR=localhost:50051 go run ./player

run-gameserver: proto
	@echo "🏃  Ejecutando GAMESERVER en local…"
	MATCHMAKER_ADDR=localhost:50051 go run ./gameserver

run-matchmaker: proto
	@echo "🏃  Ejecutando MATCHMAKER en local…"
	go run ./matchmaker

run-admin: proto
	@echo "🏃  Ejecutando ADMIN CLIENT en local…"
	MATCHMAKER_ADDR=localhost:50051 go run ./adminclient

# ------------------------------------------------------------
# 3) CONSTRUCCIÓN Y DESPLIEGUE CON DOCKER
# ------------------------------------------------------------
## Construye TODAS las imágenes desde cero (sin cache)
docker-build: proto
	@echo "🧹  Limpiando caché de builder…"
	go mod tidy
	docker builder prune -f
	@echo "🐳  Build de imágenes Docker…"
	$(COMPOSE) build --no-cache

## Levanta todo el stack (matchmaker, 2 players, 3 servers, admin)
docker-up: docker-build
	@echo "🚀  Levantando stack completo…"
	$(COMPOSE) up --force-recreate

## Derriba el stack manteniendo huérfanos fuera
docker-down:
	@echo "🛑  Derribando stack…"
	$(COMPOSE) down --remove-orphans

# ------------------------------------------------------------
# 4) TARGETS ESPECÍFICOS POR VM (solicitados en el enunciado)
# ------------------------------------------------------------
## VM1: Player1 + GameServer1
docker-jugador1-servidor1: docker-build
	$(COMPOSE) up -d player1 gameserver1

## VM2: Player2 + GameServer2
docker-jugador2-servidor2: docker-build
	$(COMPOSE) up -d player2 gameserver2

## VM3: GameServer3
docker-servidor3: docker-build
	$(COMPOSE) up -d gameserver3

## VM4: Matchmaker + AdminClient
docker-admin-matchmaker: docker-build
	$(COMPOSE) up -d matchmaker adminclient

# Atajos cómodos para los correctores ─────────────────────────
deploy-vm1: docker-jugador1-servidor1
deploy-vm2: docker-jugador2-servidor2
deploy-vm3: docker-servidor3
deploy-vm4: docker-admin-matchmaker

# ------------------------------------------------------------
# 5) LIMPIEZA EXTRA
# ------------------------------------------------------------
clean: docker-down
	@echo "🧹  Limpiando artefactos generados…"
	@find $(PROTO_DIR) -name '*.pb.go' -delete
	go clean ./...
