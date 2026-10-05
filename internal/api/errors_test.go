package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pdf-extractext/persistence/internal/domain"
	"github.com/pdf-extractext/persistence/internal/service"
)

// TestTranslateError verifica que cada error de dominio/servicio se mapea a
// su dupla contractual (status, code) del catálogo cerrado.
func TestTranslateError(t *testing.T) {
	t.Parallel()

	dupErr := &domain.DuplicateChecksumError{Existing: domain.PDFDocument{ID: "abc", Checksum: "deadbeef"}}

	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"not found", domain.ErrNotFound, http.StatusNotFound, "NOT_FOUND"},
		{"not found wrapped", fmt.Errorf("repo: %w", domain.ErrNotFound), http.StatusNotFound, "NOT_FOUND"},
		{"duplicate checksum", dupErr, http.StatusConflict, "DUPLICATE_CHECKSUM"},
		{"duplicate checksum wrapped", fmt.Errorf("service: %w", dupErr), http.StatusConflict, "DUPLICATE_CHECKSUM"},
		{"filename too long", service.ErrFilenameTooLong, http.StatusUnprocessableEntity, "FILENAME_TOO_LONG"},
		{"filename too long wrapped", fmt.Errorf("create: %w", service.ErrFilenameTooLong), http.StatusUnprocessableEntity, "FILENAME_TOO_LONG"},
		{"dependency unavailable", service.ErrDependencyUnavailable, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE"},
		{"dependency unavailable wrapped", fmt.Errorf("mongo ping: %w", service.ErrDependencyUnavailable), http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, code, message := translateError(tc.err)
			if status != tc.wantStatus {
				t.Errorf("status = %d, want %d", status, tc.wantStatus)
			}
			if code != tc.wantCode {
				t.Errorf("code = %q, want %q", code, tc.wantCode)
			}
			if message == "" {
				t.Error("message no debe ser vacío")
			}
			if strings.Contains(message, tc.err.Error()) && tc.wantStatus >= 500 {
				t.Errorf("message filtra detalle interno para status %d: %q", tc.wantStatus, message)
			}
		})
	}
}

// TestTranslateErrorFallback verifica que cualquier error genérico o no
// clasificado cae al fallback INTERNAL_ERROR (500) sin filtrar detalles.
func TestTranslateErrorFallback(t *testing.T) {
	t.Parallel()

	status, code, message := translateError(errors.New("error desconocido: timeout en socket /tmp/x.sock"))

	if status != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", status, http.StatusInternalServerError)
	}
	if code != "INTERNAL_ERROR" {
		t.Errorf("code = %q, want INTERNAL_ERROR", code)
	}
	if strings.Contains(message, "socket") || strings.Contains(message, "timeout") {
		t.Errorf("el fallback filtra detalle interno al cliente: %q", message)
	}
}

// TestRespondErrorEnvelope verifica el helper respondError: envelope exacto
// {code, message}, status correcto, Content-Type JSON, sin campos extra
// (prohibido "details") y sin stacktraces.
func TestRespondErrorEnvelope(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		status int
		code   string
	}{
		{"invalid request", http.StatusBadRequest, "INVALID_REQUEST"},
		{"not found", http.StatusNotFound, "NOT_FOUND"},
		{"duplicate", http.StatusConflict, "DUPLICATE_CHECKSUM"},
		{"filename too long", http.StatusUnprocessableEntity, "FILENAME_TOO_LONG"},
		{"internal", http.StatusInternalServerError, "INTERNAL_ERROR"},
		{"unavailable", http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			respondError(rec, tc.code, "mensaje de prueba", tc.status)

			res := rec.Result()
			if res.StatusCode != tc.status {
				t.Errorf("status = %d, want %d", res.StatusCode, tc.status)
			}
			if ct := res.Header.Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}

			var body map[string]any
			if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
				t.Fatalf("respuesta no es JSON válido: %v", err)
			}
			if len(body) != 2 {
				t.Errorf("el envelope debe tener exactamente 2 campos, tiene %d: %v", len(body), body)
			}
			if body["code"] != tc.code {
				t.Errorf("code = %v, want %q", body["code"], tc.code)
			}
			if body["message"] != "mensaje de prueba" {
				t.Errorf("message = %v, want %q", body["message"], "mensaje de prueba")
			}
			for _, forbidden := range []string{"details", "stacktrace", "stack", "type", "title", "instance"} {
				if _, ok := body[forbidden]; ok {
					t.Errorf("campo prohibido %q presente en el envelope", forbidden)
				}
			}
		})
	}
}
