# pdf-extractext-persistence

Microservicio interno encargado de almacenar y recuperar documentos PDF (`filename`, `extracted_text`, `checksum`). Centraliza el acceso a MongoDB para los servicios de API y Extracción.

> Estado actual: **bootstrap inicial en Go**. Solo expone `GET /health`; el CRUD y el repositorio Mongo se implementarán en etapas posteriores.

## Requisitos

- Go >= 1.27
- Docker (para ejecutar la imagen)

## Configuración

Variables de entorno:

| Variable    | Descripción                    | Por defecto                  |
| ----------- | ------------------------------ | ---------------------------- |
| `APP_NAME`  | Nombre del servicio            | `pdf-extractext-persistence` |
| `APP_ENV`   | Entorno (`local`, `dev`...)    | `local`                      |
| `APP_PORT`  | Puerto de escucha              | `8002`                       |

## Ejecutar en local

```bash
go run ./cmd/api
```

Luego:

```bash
curl http://localhost:8002/health
# {"environment":"local","service":"pdf-extractext-persistence","status":"ok"}
```

## Tests

```bash
go test ./...
```

## Docker

```bash
docker build -t pdf-extractext-persistence .
docker run --rm -p 8002:8002 pdf-extractext-persistence
```

## Estructura

```
cmd/api/            # Entrypoint (main): config + servidor HTTP con graceful shutdown
internal/
├── config/         # Configuración vía variables de entorno
└── api/            # Router (chi) y handlers: GET /health
```
