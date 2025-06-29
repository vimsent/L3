# gameserver/Dockerfile
# ------------------------------------------------------------
# Multi-stage build that compiles the GameServer binary in an
# Alpine-based builder image and copies the final executable
# into a minimal runtime image.
# ------------------------------------------------------------
# syntax=docker/dockerfile:1

############################
# 1. Builder stage
############################
FROM golang:1.22-alpine AS builder

# Produce a statically-linked Linux binary
ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64

WORKDIR /build

# ——— Layer-cache optimisation ———
# Copy only module manifests first to leverage Docker cache
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source code
COPY . .

# Compile → output placed in /out
RUN go build -v -o /out/gameserver ./main.go

############################
# 2. Runtime stage
############################
FROM alpine:3.20

# Minimal runtime deps (TLS certificates, non-root user)
RUN apk add --no-cache ca-certificates \
 && adduser -D -g '' appuser

USER appuser
WORKDIR /home/appuser

# Copy the statically-linked binary from builder
COPY --from=builder /out/gameserver .

# gRPC puerto por defecto para la entidad GameServer
EXPOSE 50052

# Variables de entorno que se pueden sobreescribir en tiempo de ejecución
ENV MATCHMAKER_ADDR=matchmaker:50051 \
    GAMESERVER_ID=GameServer1 \
    PORT=50052

# Lanzar el servidor
ENTRYPOINT ["./gameserver"]
