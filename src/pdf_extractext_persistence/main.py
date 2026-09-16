"""Entrypoint del servicio: uvicorn."""

import uvicorn

from pdf_extractext_persistence.config import Settings


def run() -> None:
    settings = Settings()
    uvicorn.run(
        "pdf_extractext_persistence.app:create_app",
        factory=True,
        host=settings.host,
        port=settings.port,
    )


if __name__ == "__main__":
    run()
