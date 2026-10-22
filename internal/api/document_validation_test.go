package api

import (
	"bytes"
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
	router := NewRouter(cfg, svc, nil)

	longFilename := strings.Repeat("a", 101) + ".pdf"

	tests := []struct {
		name       string
		body       *string // nil = cuerpo faltante
		wantStatus int
		wantCode   string
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
			name:       "falta campo obligatorio filename",
			body:       strPtr(`{"checksum": "abc123"}`),
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_REQUEST",
		},
		{
			name:       "falta campo obligatorio checksum",
			body:       strPtr(`{"filename": "informe.pdf"}`),
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_REQUEST",
		},
		{
			name:       "filename de 101 caracteres supera el máximo",
			body:       strPtr(`{"filename": "` + longFilename + `", "checksum": "abc123"}`),
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "FILENAME_TOO_LONG",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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

			// Assert: problem RFC 9457 con el code esperado.
			assertProblem(t, rec, tt.wantCode)
		})
	}
}

func strPtr(s string) *string { return &s }
