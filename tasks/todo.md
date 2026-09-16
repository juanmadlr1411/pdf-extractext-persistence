# Task List: Bootstrap de `pdf-extractext-persistence`

## Task 1: Scaffolding del paquete y `pyproject.toml`

**Descripción:** Crear el paquete `src/pdf_extractext_persistence/` y el `pyproject.toml` con hatchling, `requires-python = ">=3.12"` y dependencias mínimas.

**Criterios de aceptación:**
- [x] `pyproject.toml` define nombre, versión, Python >=3.12, deps runtime y grupo dev
- [x] Paquete bajo `src/` instalable

**Verificación:**
- [x] `uv sync` completa sin errores
- [x] `uv run python -c "import pdf_extractext_persistence"` funciona

**Dependencias:** Ninguna
**Alcance estimado:** S (1-2 archivos)

## Task 2: Configuración por variables de entorno

**Descripción:** `config.py` con `Settings(BaseSettings)` (pydantic-settings), prefijo `APP_`, valores por defecto razonables (`app_name`, `app_env`, `app_host`, `app_port`).

**Criterios de aceptación:**
- [x] `Settings` lee variables de entorno con prefijo `APP_`
- [x] Funciona con valores por defecto sin env vars

**Verificación:**
- [x] `uv run python -c "from pdf_extractext_persistence.config import Settings; print(Settings())"` funciona

**Dependencias:** Task 1
**Alcance estimado:** S

## Task 3: App FastAPI mínima + `/health`

**Descripción:** `app.py` con factory `create_app()`, router `api/health.py` con `GET /health` que devuelve `{"status": "ok", ...}`, y `main.py` como entrypoint uvicorn.

**Criterios de aceptación:**
- [x] `create_app()` devuelve una app FastAPI configurada desde `Settings`
- [x] `GET /health` responde 200 con `{"status": "ok", "service": ..., "environment": ...}`

**Verificación:**
- [x] `uv run uvicorn ... --factory` levanta y `curl /health` → 200

**Dependencias:** Tasks 1, 2
**Alcance estimado:** S

## Task 4: Test unitario de `/health`

**Descripción:** `tests/test_health.py` con `TestClient`.

**Criterios de aceptación:**
- [x] Test verifica status 200 y `status == "ok"`

**Verificación:**
- [x] `uv run pytest` verde

**Dependencias:** Task 3
**Alcance estimado:** XS

## Task 5: Archivos de soporte

**Descripción:** `.gitignore`, `.dockerignore`, `Dockerfile` (multi-stage, python:3.12-slim + uv), `README.md` (conservar descripción actual + instrucciones de uso).

**Criterios de aceptación:**
- [x] `.gitignore` Python/uv estándar
- [x] `.dockerignore` excluye `.git`, `.venv`, `tests`, `tasks`
- [x] `Dockerfile` instala deps sin dev y ejecuta uvicorn
- [x] `README.md` con requisitos, ejecución local, tests y Docker

**Verificación:**
- [x] Verificación estática del Dockerfile (`docker build` si Docker está disponible)

**Dependencias:** Task 1
**Alcance estimado:** S

## Checkpoint final
- [x] `uv sync` ok
- [x] `uv run pytest` verde
- [x] App levanta y responde `/health`
- [x] Sin dependencias de negocio
