package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pdf-extractext/persistence/internal/config"
	"github.com/pdf-extractext/persistence/internal/service"
)

// TestCreateDocument_ContractRejections verifica los rechazos del contrato
// en POST /documents: 400 INVALID_REQUEST (deserialización estricta) y
// 422 FILENAME_TOO_LONG (validación del servicio).
func TestCreateDocument_ContractRejections(t *testing.T) {
	cfg := config.Config{AppName: "pdf-extractext-persistence", Environment: "local"}
	svc := service.NewDocumentService(newFakeDocumentRepository())
	router := NewRouter(cfg, svc)

	longFilename := strings.Repeat("a", 101) + ".pdf"

	tests := []struct {
		name         string
		body         *string // nil = cuerpo faltante
		wantStatus   int
		wantCode     string
		pendingIssue bool // true = validación de invariantes postergada
	}{
		{
			name:       "cuerpo faltante (payload vacío)",
			body:       nil,
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_REQUEST",
		},
		{
			name:       "payload vacío (string vacío)",
			body:       strPtr(""),
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_REQUEST",
		},
		{
			name:       "JSON malformado",
			body:       strPtr(`{"filename": "informe.pdf",`),
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_REQUEST",
		},
		{
			name:       "tipo incorrecto en filename",
			body:       strPtr(`{"filename": 123, "checksum": "abc123"}`),
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_REQUEST",
		},
		{
			name:         "falta campo obligatorio filename",
			body:         strPtr(`{"checksum": "abc123"}`),
			wantStatus:   http.StatusBadRequest,
			wantCode:     "INVALID_REQUEST",
			pendingIssue: true,
		},
		{
			name:         "falta campo obligatorio checksum",
			body:         strPtr(`{"filename": "informe.pdf"}`),
			wantStatus:   http.StatusBadRequest,
			wantCode:     "INVALID_REQUEST",
			pendingIssue: true,
		},
		{
			name:         "filename de 101 caracteres supera el máximo",
			body:         strPtr(`{"filename": "` + longFilename + `", "checksum": "abc123"}`),
			wantStatus:   http.StatusUnprocessableEntity,
			wantCode:     "FILENAME_TOO_LONG",
			pendingIssue: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.pendingIssue {
				t.Skip("TODO: postergado para la issue de validación de invariantes")
			}
			var req *http.Request
			if tt.body == nil {
				req = httptest.NewRequest(http.MethodPost, "/documents", nil)
			} else {
				req = httptest.NewRequest(http.MethodPost, "/documents", bytes.NewBufferString(*tt.body))
			}
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			// Assert: status code
			if rec.Code != tt.wantStatus {
				t.Fatalf("esperado status %d, obtenido %d (body: %s)", tt.wantStatus, rec.Code, rec.Body.String())
			}

			// Assert: Content-Type
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Fatalf("Content-Type esperado application/json, obtenido %q", ct)
			}

			// Assert: envelope de error con EXACTAMENTE {code, message}
			var body map[string]any
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("cuerpo no es JSON válido: %v", err)
			}
			if len(body) != 2 {
				t.Fatalf("esperados exactamente 2 campos (code, message), obtenidos %d: %v", len(body), body)
			}
			if body["code"] != tt.wantCode {
				t.Errorf("code esperado %q, obtenido %v", tt.wantCode, body["code"])
			}
			if _, ok := body["message"].(string); !ok {
				t.Errorf("campo 'message' ausente o no es string: %v", body["message"])
			}
		})
	}
}

func strPtr(s string) *string { return &s }
