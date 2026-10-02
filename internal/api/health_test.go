package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pdf-extractext/persistence/internal/config"
)

// FASE RED: Este test fallará inicialmente (NewRouter aún no existe).
func TestHealth_Returns200WithOKStatus(t *testing.T) {
	cfg := config.Config{AppName: "pdf-extractext-persistence", Environment: "local"}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	NewRouter(cfg).ServeHTTP(rec, req)

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
