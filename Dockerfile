# syntax=docker/dockerfile:1

# --- Stage 1: build ---
FROM golang:1.27-alpine AS builder

WORKDIR /src

# Descargar dependencias primero para aprovechar la caché de capas
COPY go.mod go.sum ./
RUN go mod download

# Compilar servicio y healthprobe (binarios estáticos)
COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /app/api ./cmd/api && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /app/healthprobe ./cmd/healthprobe

# --- Stage 2: runtime mínimo ---
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /app/api /app/api
COPY --from=builder /app/healthprobe /healthprobe

EXPOSE 8002
USER nonroot

# Sin shell: healthprobe verifica /health/ready local.
HEALTHCHECK --interval=10s --timeout=3s --retries=3 \
  CMD ["/healthprobe"]

ENTRYPOINT ["/app/api"]
