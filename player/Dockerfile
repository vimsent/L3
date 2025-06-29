# syntax=docker/dockerfile:1

############################################################
# STAGE 1 ─ Builder
############################################################
FROM golang:1.22-bookworm AS builder

# Directorio de trabajo dentro de la imagen
WORKDIR /src

# -----------------------------------------------------------------
# 1) Dependencias
#    Copiar primero go.mod y go.sum permite que la etapa de descarga
#    de módulos quede cacheada mientras el resto del código cambia.
# -----------------------------------------------------------------
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    go mod download

# -----------------------------------------------------------------
# 2) Código fuente
# -----------------------------------------------------------------
COPY . .

# -----------------------------------------------------------------
# 3) Compilación
#    - CGO_ENABLED=0 → binario estático
#    - GOOS=linux GOARCH=amd64 → target Linux 64-bit
#    - -trimpath → elimina rutas absolutas del binario (reproducible)
# -----------------------------------------------------------------
WORKDIR /src/player
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64 \
    go build -v -trimpath -o /player .

############################################################
# STAGE 2 ─ Runtime
############################################################
# Distroless: imagen ultraligera, solo las bibliotecas básicas
FROM gcr.io/distroless/static-debian12

# -----------------------------------------------------------------
# Variables de entorno que se pueden sobreescribir al ejecutar
# el contenedor (docker run -e VAR=valor …).  Permiten adaptar
# la dirección del Matchmaker y la identidad del jugador sin
# recompilar ni modificar el código fuente.
# -----------------------------------------------------------------
ENV MATCHMAKER_ADDR=localhost:50051 \
    PLAYER_ID=Player1 \
    LOG_LEVEL=info

# Copiamos el binario ya compilado desde la etapa anterior
COPY --from=builder /player /player

# No exponemos puertos: el jugador sólo realiza conexiones salientes
ENTRYPOINT ["/player"]
