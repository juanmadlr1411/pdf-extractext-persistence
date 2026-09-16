FROM python:3.12-slim

COPY --from=ghcr.io/astral-sh/uv:latest /uv /uvx /bin/

WORKDIR /app

# Instalar dependencias primero para aprovechar la caché de capas
COPY pyproject.toml uv.lock README.md ./
RUN uv sync --frozen --no-dev --no-install-project

# Copiar el código fuente e instalar el proyecto
COPY src/ ./src/
RUN uv sync --frozen --no-dev

EXPOSE 8002

CMD ["uv", "run", "--no-dev", "uvicorn", "pdf_extractext_persistence.app:create_app", "--factory", "--host", "0.0.0.0", "--port", "8002"]
