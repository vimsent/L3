# Sistema Distribuido de Emparejamiento Multijugador  
Laboratorio 3 · Sistemas Distribuidos · UTFSM · 2025-1

> Proyecto escrito en **Go ≥ 1.22** usando **gRPC**, **Protocol Buffers** y **Docker**.  
> Compatible con ejecución local (terminales independientes) y en múltiples VMs mediante variables de entorno.

---

## 1 · Clonar el repositorio

```bash
git clone https://github.com/<tu-usuario>/<repo-lab3>.git
cd <repo-lab3>


| Componente                         | Versión mínima   | Linux (apt)                                                                                                                           | macOS (brew)            | Windows + WSL                                         |
| ---------------------------------- | ---------------- | ------------------------------------------------------------------------------------------------------------------------------------- | ----------------------- | ----------------------------------------------------- |
| Go                                 | **1.22**         | `sudo apt install golang`                                                                                                             | `brew install go`       | Instalar desde [https://go.dev/dl](https://go.dev/dl) |
| protoc                             | **3.21**         | `sudo apt install protobuf-compiler`                                                                                                  | `brew install protobuf` | Descargar binarios                                    |
| protoc-gen-go / protoc-gen-go-grpc | acorde a Go 1.22 | `go install google.golang.org/protobuf/cmd/protoc-gen-go@latest`<br>`go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest` | Igual                   | Igual                                                 |
| Docker + Docker Compose v2         | —                | [https://docs.docker.com/engine/install/](https://docs.docker.com/engine/install/)                                                    | `brew install docker`   | Docker Desktop                                        |
