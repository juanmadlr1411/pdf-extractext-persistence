# pdf-extractext-persistence
Microservicio interno encargado de almacenar y recuperar documentos PDF (filename, texto, checksum). Centraliza el pool de conexiones a MongoDB y provee las operaciones CRUD para los servicios de API y Extracción.

> Estado actual: bootstrap inicial. Solo expone `GET /health`; la lógica de persistencia se implementará en etapas posteriores.

## Requisitos

- Python >= 3.12
- [uv](https://docs.astral.sh/uv/)

## Configuración

Variables de entorno (prefijo `APP_`):

| Variable | Descripción | Por defecto |
|---|---|---|
| `APP_NAME` | Nombre del servicio | `pdf-extractext-persistence` |
| `APP_ENV` | Entorno (`local`, `dev`, `prod`...) | `local` |
| `APP_HOST` | Host de escucha | `0.0.0.0` |
| `APP_PORT` | Puerto de escucha | `8000` |

## Ejecutar en local

```bash
uv sync
uv run python -m pdf_extractext_persistence.main
```

Luego:

```bash
curl http://localhost:8000/health
# {"status":"ok","service":"pdf-extractext-persistence","environment":"local"}
```

## Tests

```bash
uv run pytest
```

## Docker

```bash
docker build -t pdf-extractext-persistence .
docker run --rm -p 8000:8000 pdf-extractext-persistence
```

## Estructura

```
src/pdf_extractext_persistence/
├── config.py       # Settings vía variables de entorno
├── app.py          # Factory de la app FastAPI
├── main.py         # Entrypoint uvicorn
└── api/
    └── health.py   # GET /health
tests/
└── test_health.py  # Test unitario de /health
```
