package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pdf-extractext/persistence/internal/config"
	"github.com/pdf-extractext/persistence/internal/service"
)

// fakeHealthChecker implementa la interfaz service.HealthChecker
// para simular el estado de una dependencia (Mongo) en tests.
type fakeHealthChecker struct {
	healthy bool
}

func (f *fakeHealthChecker) Check(_ context.Context) error {
	if f.healthy {
		return nil
	}
	return errors.New("fake dependency is unhealthy")
}

// newHealthTestRouter construye el router para tests de health checks.
// docService y healthChecker pueden controlarse por caso de prueba.
func newHealthTestRouter(cfg config.Config, docService *service.DocumentService, healthChecker service.HealthChecker) http.Handler {
	return NewRouter(cfg, docService, healthChecker)
}

// --- Test de liveness: GET /health ---

func TestHealth_Returns200WithOKStatus(t *testing.T) {
	cfg := config.Config{AppName: "pdf-extractext-persistence", Environment: "local"}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	// docService y healthChecker son nil: /health no los necesita.
	newHealthTestRouter(cfg, nil, nil).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type esperado application/json, obtenido %q", ct)
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("cuerpo no es JSON válido: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status esperado %q, obtenido %q", "ok", body["status"])
	}
	if body["service"] != "pdf-extractext-persistence" {
		t.Errorf("service esperado %q, obtenido %q", "pdf-extractext-persistence", body["service"])
	}
	if body["environment"] != "local" {
		t.Errorf("environment esperado %q, obtenido %q", "local", body["environment"])
	}
}

// --- Tests de readiness de la Issue #12 ---

// Caso 1: GET /health/ready con dependencia OK -> 200 OK.
func TestReadiness_MongoOK_Returns200(t *testing.T) {
	cfg := config.Config{AppName: "pdf-extractext-persistence", Environment: "local"}
	fakeChecker := &fakeHealthChecker{healthy: true} // Simula Mongo OK

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	rec := httptest.NewRecorder()

	newHealthTestRouter(cfg, nil, fakeChecker).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d (body: %s)", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type esperado application/json, obtenido %q", ct)
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("cuerpo no es JSON válido: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status esperado %q, obtenido %q", "ok", body["status"])
	}
}

// Caso 2: GET /health/ready con dependencia caída -> 503 DEPENDENCY_UNAVAILABLE.
func TestReadiness_MongoDown_Returns503(t *testing.T) {
	cfg := config.Config{AppName: "pdf-extractext-persistence", Environment: "local"}
	fakeChecker := &fakeHealthChecker{healthy: false} // Simula Mongo caído

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	rec := httptest.NewRecorder()

	newHealthTestRouter(cfg, nil, fakeChecker).ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("esperado 503, obtenido %d (body: %s)", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type esperado application/json, obtenido %q", ct)
	}

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("cuerpo no es JSON válido: %v", err)
	}
	if len(body) != 2 {
		t.Fatalf("envelope de error debe tener exactamente 2 campos, obtenidos %d: %v", len(body), body)
	}
	if body["code"] != "DEPENDENCY_UNAVAILABLE" {
		t.Errorf("code esperado %q, obtenido %v", "DEPENDENCY_UNAVAILABLE", body["code"])
	}
	if msg, ok := body["message"].(string); !ok || msg == "" {
		t.Errorf("message debe ser un string no vacío, obtenido %v", body["message"])
	}
}
