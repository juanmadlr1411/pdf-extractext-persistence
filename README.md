# pdf-extractext-persistence

Microservicio interno encargado de almacenar y recuperar documentos PDF (`filename`, `extracted_text`, `checksum`). Centraliza el acceso a MongoDB para los servicios de API y Extracción.

> Estado actual: CRUD completo de documentos sobre MongoDB (índice único `checksum_1`) con probes de liveness/readiness y apagado ordenado. El contrato de salida es `{id, filename, extracted_text, checksum}`.

## Requisitos

- Go >= 1.27
- Docker (para ejecutar la imagen)

## Configuración

Variables de entorno:

| Variable                | Descripción                  | Por defecto                  |
| ----------------------- | ---------------------------- | ---------------------------- |
| `APP_NAME`              | Nombre del servicio          | `pdf-extractext-persistence` |
| `APP_ENV`               | Entorno (`local`, `dev`...)  | `local`                      |
| `APP_PORT`              | Puerto de escucha            | `8002`                       |
| `MONGO_URI`             | URI de conexión a MongoDB    | *(obligatoria)*              |
| `MONGO_DATABASE_NAME`   | Database de MongoDB          | `pdf_extractext`             |
| `MONGO_COLLECTION_NAME` | Colección de documentos      | `pdf_documents`              |

## API

| Método | Ruta | Descripción |
| ------ | ---- | ----------- |
| `POST` | `/documents` | Crea un documento. `201`; `400` cuerpo inválido; `409` `DUPLICATE_CHECKSUM` (adjunta el documento existente que provoca el conflicto); `503` dependencia caída; `500`. |
| `GET` | `/documents` | Lista todos los documentos (`200`). |
| `GET` | `/documents/{id}` | Obtiene uno (`200`); `400` id con formato inválido; `404`. |
| `PATCH` | `/documents/{id}` | Renombra (`{"filename": "..."}`); `200`; `400`; `404`; `422` `FILENAME_TOO_LONG`. |
| `DELETE` | `/documents/{id}` | Elimina (`204`); `400` id inválido; `404`. |
| `GET` | `/health` | Liveness (no depende de Mongo): `{"status":"ok","service":"...","environment":"..."}`. |
| `GET` | `/health/ready` (alias `/readyz`) | Readiness: `200`, o `503 {"code":"DEPENDENCY_UNAVAILABLE"}` si Mongo no responde. |

- **Entrada de `POST /documents`:** `{"filename": "...", "extracted_text": "...", "checksum": "..."}`.
- **Contrato de salida:** `{"id": "<ObjectId hex>", "filename": "...", "extracted_text": "...", "checksum": "..."}`.
- **Errores:** problem RFC 9457 (`application/problem+json`) con `{type, title, status, detail, code}` (+ `document` en el 409). Códigos: `INVALID_REQUEST` (400), `NOT_FOUND` (404), `DUPLICATE_CHECKSUM` (409), `FILENAME_TOO_LONG` (422), `INTERNAL_ERROR` (500), `DEPENDENCY_UNAVAILABLE` (503).
- `PATCH` solo acepta `filename` (máx. 100 caracteres); el resto de los campos se ignora.

## Ejecutar en local

Hay dos formas de probar el servicio, según qué MongoDB uses:

### Opción 1 — MongoDB propio (la más simple)

Útil para desarrollo rápido sin levantar toda la infraestructura:

```bash
# 1. Levantar un Mongo propio
docker run -d --name mongo-test -p 27017:27017 mongo:8

# 2. Correr el servicio contra ese Mongo
MONGO_URI="mongodb://localhost:27017" go run ./cmd/api
```

### Opción 2 — Contra la infraestructura del equipo (docker-compose)

El Mongo del compose **no publica el puerto al host**: solo Persistence accede
a él. Por eso el servicio debe correr como contenedor en la red `data`, con
las credenciales del `.env` de infraestructura:

```bash
# 1. Desde el repo infrastructure
docker compose up -d mongodb

# 2. Desde este repo: construir y correr en la red de datos
docker build -t pdf-extractext-persistence .
docker run --rm --network "pdf-extractext_data" -p 8002:8002 \
  -e MONGO_URI="mongodb://<usuario>:<password>@mongodb:27017" \
  pdf-extractext-persistence
```

Para verificar el índice único (credenciales y contenedor del compose):

```bash
docker exec pdf-extractext-mongodb-1 mongosh --quiet \
  -u <usuario> -p <password> --authenticationDatabase admin \
  --eval 'db.getSiblingDB("pdf_extractext").pdf_documents.getIndexes()'
# Debe incluir: { key: { checksum: 1 }, name: 'checksum_1', unique: true }
```

> **Ownership de índices**: el servicio crea el índice único `checksum_1` al arrancar (idempotente) y, si no puede, **no levanta el servidor HTTP** (fail-fast).

En cualquiera de las dos opciones, verificá el estado del servicio con:

```bash
curl http://localhost:8002/health
# {"status":"ok","service":"pdf-extractext-persistence","environment":"local"}
```

## Tests

```bash
go test ./...
```

Tests de integración (requieren Docker; testcontainers levanta `mongo:8` automáticamente):

```bash
go test ./... -tags=integration
```

## Docker

```bash
docker build -t pdf-extractext-persistence .
docker run --rm -p 8002:8002 \
  -e MONGO_URI="mongodb://<host-mongo>:27017" \
  pdf-extractext-persistence
```

> El contenedor necesita alcanzar un MongoDB: ver "Ejecutar en local" (Mongo propio o red `pdf-extractext_data` de infraestructura).

## Estructura

```
cmd/
├── api/            # Entrypoint: config + Mongo lifecycle + servidor HTTP + graceful shutdown
└── healthprobe/    # Probe para la imagen distroless
internal/
├── config/         # Configuración vía variables de entorno
├── api/            # Router (chi), handlers y middlewares (X-Request-ID, panic recovery)
├── domain/         # Entidades (Document, PDFDocument) y errores de dominio
├── service/        # Lógica de aplicación (DocumentService, health)
├── repository/     # Repositorio PDFDocument sobre MongoDB
└── infra/mongo/    # Cliente MongoDB: lifecycle, mapper y ownership de índices (checksum_1 único)
```
