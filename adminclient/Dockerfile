# ────────────────────────────────────────────────────────────────
# Multi-stage build to obtain a tiny final image
# ────────────────────────────────────────────────────────────────
# 1️⃣  Builder stage – compila el binario de Go estático
FROM golang:1.22-alpine AS builder

# Activamos algunos paquetes mínimos (git lo exige `go mod download`)
RUN apk add --no-cache git

# Trabajaremos en /app
WORKDIR /app

# Copiamos los archivos de módulos primero para aprovechar el layer cache
COPY go.mod go.sum ./
RUN go mod download

# Copiamos el código fuente de TODA la app (solo lo necesario)
#   – el main del adminclient
#   – código generado de protobuf (en /proto y /**/*.pb.go)
#   – cualquier paquete local usado por adminclient
COPY adminclient ./adminclient
COPY proto        ./proto
COPY internal     ./internal    # si tienes paquetes compartidos

# Compilamos un binario estático:
#   - CGO_ENABLED=0  → evita dependencias de C
#   - -trimpath      → elimina paths absolutos del binario
#   - -ldflags '-s -w' → reduce tamaño eliminando símbolos de depuración
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -v -trimpath -ldflags="-s -w" \
    -o /adminclient ./adminclient

# 2️⃣  Runtime stage – solo incluye el binario
FROM alpine:3.20 AS runtime

# Creamos directorio de trabajo sin permisos de root
WORKDIR /app
RUN adduser -D -u 10001 admin
USER admin

# Copiamos el binario estático desde la etapa builder
COPY --from=builder /adminclient /usr/local/bin/adminclient

# Variable con valor por defecto para facilitar ejecución local;
# en VMs se sobreescribe con  MATCHMAKER_ADDR=matchmaker:50051
ENV MATCHMAKER_ADDR=localhost:50051

# El contenedor *solo* ejecuta el binario
ENTRYPOINT ["/usr/local/bin/adminclient"]

# Puerto expuesto únicamente a modo documental (usa gRPC saliente)
EXPOSE 50051
