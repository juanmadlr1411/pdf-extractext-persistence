package api

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

// assertProblem verifica el contrato RFC 9457 de una respuesta de error:
// Content-Type problem+json y campos exactos {type, title, status, detail, code}.
func assertProblem(t *testing.T, rec *httptest.ResponseRecorder, wantCode string) map[string]any {
	t.Helper()

	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Fatalf("Content-Type esperado application/problem+json, obtenido %q", ct)
	}

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("cuerpo no es JSON válido: %v", err)
	}
	if len(body) != 5 {
		t.Fatalf("esperados exactamente 5 campos (type, title, status, detail, code), obtenidos %d: %v", len(body), body)
	}
	if body["code"] != wantCode {
		t.Errorf("code esperado %q, obtenido %v", wantCode, body["code"])
	}
	if status, ok := body["status"].(float64); !ok || int(status) != rec.Code {
		t.Errorf("status del problema %v no coincide con el código HTTP %d", body["status"], rec.Code)
	}
	if detail, ok := body["detail"].(string); !ok || detail == "" {
		t.Errorf("detail ausente o vacío: %v", body["detail"])
	}
	if title, ok := body["title"].(string); !ok || title == "" {
		t.Errorf("title ausente o vacío: %v", body["title"])
	}
	if urn, ok := body["type"].(string); !ok || urn != "urn:pdf-extractext:"+problemCatalog[wantCode].urn {
		t.Errorf("type esperado %q, obtenido %v", "urn:pdf-extractext:"+problemCatalog[wantCode].urn, body["type"])
	}
	return body
}
