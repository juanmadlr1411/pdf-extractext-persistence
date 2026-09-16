# Plan de Implementación: Bootstrap de `pdf-extractext-persistence`

## Objetivo
Base mínima, limpia y ejecutable de un microservicio FastAPI con Python 3.12 y `uv`. Sin lógica de negocio, sin MongoDB, sin Docker Compose.

## Decisiones de arquitectura
- **Gestor:** `uv` con `pyproject.toml` + `uv.lock`.
- **Layout `src/`:** paquete `src/pdf_extractext_persistence/` importable, instalado en modo editable vía `[tool.uv] package = true` (hatchling como build backend).
- **Config:** módulo `config.py` con `pydantic-settings` (variables de entorno con prefijo `APP_`: `APP_NAME`, `APP_ENV`, `APP_PORT`, etc.).
- **App factory:** `create_app()` en `app.py`; el endpoint `/health` vive en un router propio (`api/health.py`).
- **Sin abstracciones extra:** nada de repositorios, DI ni módulos por dominio todavía.

## Estructura
```
pyproject.toml          uv.lock (generado)   .gitignore   .dockerignore
Dockerfile              README.md
tasks/plan.md           tasks/todo.md
src/pdf_extractext_persistence/
    __init__.py
    config.py           # Settings (pydantic-settings, env vars)
    app.py              # create_app() -> FastAPI
    main.py             # entrypoint: uvicorn
    api/__init__.py
    api/health.py       # GET /health -> {"status": "ok", ...}
tests/
    __init__.py
    test_health.py      # TestClient: /health devuelve 200 y payload correcto
```

## Dependencias
- **Runtime:** `fastapi`, `uvicorn[standard]`, `pydantic-settings`.
- **Dev:** `pytest`, `httpx`.
- Nada de motor/pymongo, redis ni clientes HTTP de otros servicios.

## Lista de tareas
Ver `tasks/todo.md`.

## Riesgos
| Riesgo | Mitigación |
|---|---|
| Dockerfile sin Docker local para validar | Verificación estática + sintaxis estándar uv |
| Over-engineering | Prohibido añadir módulos/archivos fuera de la lista |

## Fuera de alcance (explícito)
CRUD, MongoDB/repositorios, Dragonfly, endpoints de negocio, comunicación entre servicios, Docker Compose, Traefik, tests de integración.
