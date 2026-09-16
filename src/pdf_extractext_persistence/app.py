"""Factory de la aplicación FastAPI."""

from fastapi import FastAPI

from pdf_extractext_persistence import __version__
from pdf_extractext_persistence.api.health import router as health_router
from pdf_extractext_persistence.config import Settings


def create_app() -> FastAPI:
    settings = Settings()

    app = FastAPI(title=settings.name, version=__version__)
    app.state.settings = settings
    app.include_router(health_router)

    return app
