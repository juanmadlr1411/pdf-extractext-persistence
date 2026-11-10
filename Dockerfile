# syntax=docker/dockerfile:1

FROM golang:1.27-alpine@sha256:eb61c3873c97416fff0116553fff65d67a3549f72e11d9758849a689cdd8b523 AS builder

WORKDIR /src

# Descargar dependencias primero para aprovechar la caché
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /app/api ./cmd/api && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /app/healthprobe ./cmd/healthprobe

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /app/api /app/api
COPY --from=builder /app/healthprobe /healthprobe

EXPOSE 8002
USER nonroot

# Sin shell: healthprobe verifica /health/ready local.
HEALTHCHECK --interval=10s --timeout=3s --retries=3 \
  CMD ["/healthprobe"]

ENTRYPOINT ["/app/api"]
