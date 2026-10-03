# pdf-extractext-persistence

Microservicio interno encargado de almacenar y recuperar documentos PDF (`filename`, `extracted_text`, `checksum`). Centraliza el acceso a MongoDB para los servicios de API y Extracción.

> Estado actual: expone `GET /health` y gestiona la conexión a MongoDB con sus índices (`internal/infra/mongo`). El CRUD y el repositorio se implementarán en etapas posteriores.

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

## Ejecutar en local

Hay dos formas de probar el servicio, según qué MongoDB uses:

### Opción 1 — MongoDB propio (la más simple)

Útil para desarrollo rápido sin levantar toda la infraestructura:

```bash
# 1. Levantar un Mongo propio
docker run -d --name mongo-test -p 27017:27017 mongo:7.0

# 2. Correr el servicio contra ese Mongo
MONGO_URI="mongodb://localhost:27017" go run ./cmd/api
```

### Opción 2 — Contra la infraestructura del equipo (docker-compose)

El Mongo del repo `pdf-extractext-infrastructure` **no publica el puerto al
host** (solo el Persistence Service accede a él). Por eso el servicio debe
correr como contenedor dentro de la misma red Docker (`data` del compose)
y con autenticación (usuario/contraseña definidos en el `.env` de ese repo).

Los nombres concretos (red, hostname y contenedor) derivan del `name:` y los
servicios declarados en el `docker-compose.yml` de infraestructura. Verificalos
antes de correr los comandos, por si cambian:

```bash
docker network ls   # buscar <nombre-del-proyecto>_data
docker ps           # buscar el contenedor de MongoDB
```

Con los valores actuales del compose (`name: pdf-extractext`, servicio
`mongodb`, credenciales de desarrollo en `.env`):

```bash
# 1. Levantar la infraestructura (desde el repo infrastructure)
docker compose up -d mongodb

# 2. Construir la imagen del servicio (desde este repo)
docker build -t pdf-extractext-persistence .

# 3. Correr el servicio dentro de la red de datos
#    - <proyecto>_data: red "data" del compose
#    - <usuario>:<password>@mongodb: credenciales del .env y hostname del
#      servicio Mongo dentro de la red
#    (-p 8002:8002 publica el puerto al host para poder hacer curl)
docker run --rm --network "<proyecto>_data" \
  -p 8002:8002 \
  -e MONGO_URI="mongodb://<usuario>:<password>@mongodb:27017" \
  pdf-extractext-persistence
```

Para verificar que el servicio creó el índice único (reemplazá
`<contenedor-mongo>`, usuario y contraseña por los del compose):

```bash
docker exec <contenedor-mongo> mongosh --quiet \
  -u <usuario> -p <password> --authenticationDatabase admin \
  --eval 'db.getSiblingDB("pdf_extractext").pdf_documents.getIndexes()'
# Debe incluir: { key: { checksum: 1 }, name: 'checksum_1', unique: true }
```

> **Ownership de índices**: al arrancar, el servicio garantiza el índice único
> `checksum_1` sobre la colección (operación idempotente, segura de repetir en
> cada arranque). Si no puede conectar a MongoDB o crear los índices, el
> servicio **no levanta el servidor HTTP** (fail-fast).

En cualquiera de las dos opciones, verificá el estado del servicio con:

```bash
curl http://localhost:8002/health
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

Tests de integración (requieren Docker; testcontainers levanta `mongo:7.0` automáticamente):

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

> El contenedor necesita alcanzar un MongoDB: ver "Ejecutar en local" para las opciones (Mongo propio o red `pdf-extractext_data` de la infraestructura del equipo).

> Para correr el contenedor contra el MongoDB de `pdf-extractext-infrastructure`,
> ver la sección "Ejecutar en local", Opción 2.

## Estructura

```
cmd/api/            # Entrypoint (main): config + Mongo lifecycle + servidor HTTP con graceful shutdown
internal/
├── config/         # Configuración vía variables de entorno
├── api/            # Router (chi) y handlers: GET /health
└── infra/mongo/    # Cliente MongoDB: lifecycle y ownership de índices (checksum_1 único)
```
