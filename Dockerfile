# syntax=docker/dockerfile:1

# --- Stage 1: build ---
FROM golang:1.27-alpine AS builder

WORKDIR /src

# Descargar dependencias primero para aprovechar la caché de capas
COPY go.mod go.sum ./
RUN go mod download

# Compilar binario estático
COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /app/api ./cmd/api

# --- Stage 2: runtime mínimo ---
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /app/api /app/api

EXPOSE 8002
USER nonroot

ENTRYPOINT ["/app/api"]
