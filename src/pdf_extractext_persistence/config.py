"""Configuración del servicio mediante variables de entorno (prefijo APP_)."""

from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(env_prefix="APP_")

    name: str = "pdf-extractext-persistence"
    env: str = "local"
    host: str = "0.0.0.0"
    port: int = 8000
