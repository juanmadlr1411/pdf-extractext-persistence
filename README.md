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
| `CONSUL_ENABLED`        | Habilita registro y KV       | `false`                      |
| `CONSUL_HTTP_ADDR`      | Dirección HTTP de Consul     | `consul:8500`                |
| `CONSUL_HTTP_TOKEN`     | Token ACL de Consul          | *(vacío)*                    |
| `SERVICE_NAME`          | Nombre del servicio          | `persistence`                |
| `SERVICE_DOMAIN`        | Dominio para la regla Traefik | `pdf-extractext.localhost`  |
| `SERVICE_PORT`          | Puerto anunciado             | `APP_PORT` (`8002`)          |
| `SERVICE_HEALTH_PATH`   | Check de readiness           | `/health/ready`              |

Copiá `.env.example` a `.env` para desarrollo local. El compose local usa
`CONSUL_ENABLED=false`, por lo que sigue funcionando sin Consul.

### Integración con Consul y Traefik

Con `CONSUL_ENABLED=true` el arranque es:

1. Lee `GET /v1/kv/pdf-extractext/config/persistence/?recurse` **antes** de
   conectar a MongoDB. `MONGO_DATABASE_NAME` y `MONGO_COLLECTION_NAME` respetan
   la precedencia **env > KV > default**. `MONGO_URI` solo se acepta por
   variable de entorno. Si el KV no responde, se usan env/defaults y se emite
   un warning.
2. Resuelve la IP del servicio con `net.Dial("udp", consul:8500)`. Persistence
   está conectado a la red `services` y a la red `data`; el dial a Consul
   garantiza anunciar la IP de la red `services` (alcanzable por Traefik y
   Consul), no la de `data`.
3. Registra el ID `persistence-<hostname>` con tags y meta espejo de Traefik. El
   dominio de la regla `Host(...)` se toma de `SERVICE_DOMAIN` (por defecto
   `pdf-extractext.localhost`):

   ```text
   traefik.enable=true
   traefik.http.routers.persistence.rule=Host(`persistence.pdf-extractext.localhost`)
   traefik.http.routers.persistence.entrypoints=internal
   traefik.http.services.persistence.loadbalancer.server.port=8002
   ```

   El check HTTP apunta a `http://<address>:8002/health/ready` (incluye el
   ping a MongoDB), con intervalo `10s`, timeout `2s` y
   `DeregisterCriticalServiceAfter=1m`.
4. Todas las llamadas a Consul (`register`, `deregister`, `health` y `kv`)
   envían el header `X-Consul-Token`.
5. Si Consul está caído, el servicio arranca igual y reintenta el registro con
   backoff exponencial.
6. Ante `SIGTERM`/`SIGINT` ejecuta: deregister en Consul → espera 5 s para que
   Traefik deje de enviar tráfico → `srv.Shutdown` → `db.Disconnect`.

Para verificar el registro desde la red interna:

```bash
docker exec pdf-extractext-consul-1 wget -qO- \
  --header="X-Consul-Token: $CONSUL_HTTP_TOKEN" \
  http://127.0.0.1:8500/v1/health/service/persistence
```

## API

| Método | Ruta | Descripción |
| ------ | ---- | ----------- |
| `POST` | `/documents` | Crea un documento. `201`; `400` cuerpo inválido; `409` `DUPLICATE_CHECKSUM` (adjunta el documento existente que provoca el conflicto); `503` dependencia caída; `500`. |
| `GET` | `/documents` | Lista todos los documentos (`200`). |
| `GET` | `/documents/{id}` | Obtiene uno (`200`); `400` id con formato inválido; `404`. |
| `GET` | `/documents/by-checksum/{checksum}` | Lookup interno por SHA-256 hex (`200`); `400` checksum inválido; `404`; `503`. |
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

El binario Go lee variables de entorno del proceso y no parsea `.env`. Si
preferís usar el archivo, exportalo antes de arrancar:

```bash
set -a
source .env
set +a
go run ./cmd/api
```

### Opción 2 — Contra la infraestructura del equipo (docker-compose)

El `docker-compose.yml` de este repo publica MongoDB solo en loopback
(`127.0.0.1:27017`) para desarrollo local. La URI interna del contenedor se
toma de `COMPOSE_MONGO_URI`, con default `mongodb://mongo:27017`; así, el
`MONGO_URI=mongodb://localhost:27017` de `.env` (pensado para `go run`) no
afecta al compose. Para apuntar el compose a otra URI, definí
`COMPOSE_MONGO_URI`.

El Mongo del compose de infraestructura **no publica el puerto al host**: solo
Persistence accede a él por la red `data`. Por eso, para usar la infraestructura
del equipo, el servicio debe correr como contenedor en la red `data`, con las
credenciales del `.env` de infraestructura:

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
├── config/         # Configuración vía variables de entorno (+ KV de Consul)
├── consul/         # Cliente HTTP, registro, KV y secuencia de apagado
├── api/            # Router (chi), handlers y middlewares (X-Request-ID, panic recovery)
├── domain/         # Entidades (Document, PDFDocument) y errores de dominio
├── service/        # Lógica de aplicación (DocumentService, health)
├── repository/     # Repositorio PDFDocument sobre MongoDB
└── infra/mongo/    # Cliente MongoDB: lifecycle, mapper y ownership de índices (checksum_1 único)
```
