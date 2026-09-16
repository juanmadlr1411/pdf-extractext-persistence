"""Tests del endpoint GET /health."""

from fastapi.testclient import TestClient

from pdf_extractext_persistence.app import create_app

client = TestClient(create_app())


def test_health_returns_ok() -> None:
    response = client.get("/health")

    assert response.status_code == 200
    body = response.json()
    assert body["status"] == "ok"
    assert body["service"] == "pdf-extractext-persistence"
    assert "environment" in body
