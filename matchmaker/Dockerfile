# ------------------------------------------------------------
# matchmaker/Dockerfile
# Imagen multi-stage para compilar y ejecutar el Matchmaker
# ------------------------------------------------------------

# ---------- Stage 1 : build ----------
FROM golang:1.22 AS builder

# Variables de build
ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64

# Crear directorio de trabajo
WORKDIR /app

# Copiar dependencias primero para aprovechar el cache
COPY go.mod go.sum ./
RUN go mod download

# Copiar el resto del proyecto que necesita el Matchmaker
# (se asume que el contexto de build es la raíz del repo)
COPY proto ./proto
COPY matchmaker ./matchmaker

# Compilar el binario estático
RUN go build -trimpath -o /matchmaker ./matchmaker

# ---------- Stage 2 : runtime ----------
FROM scratch

# Puerto gRPC expuesto
EXPOSE 50051

# Copiar binario desde el stage de build
COPY --from=builder /matchmaker /matchmaker

# Variable opcional para sobreescribir puerto si se necesita
ENV MATCHMAKER_PORT=50051

# Ejecutar
ENTRYPOINT ["/matchmaker"]
