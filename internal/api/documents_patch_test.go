package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/pdf-extractext/persistence/internal/domain"
)

// --- Tests de la Issue #8: PATCH /documents/{id} ---

func seedDocument() domain.Document {
	return domain.Document{
		ID:            "507f1f77bcf86cd799439011",
		Filename:      "original.pdf",
		ExtractedText: "texto extraído",
		Checksum:      "abc123",
	}
}

// PATCH con filename válido -> 200 con el documento actualizado.
func TestPatchDocument_RenamesDocument(t *testing.T) {
	repo := newFakeDocumentRepository()
	repo.byID["507f1f77bcf86cd799439011"] = seedDocument()
	router := newTestRouter(repo)

	rec := doRequest(t, router, http.MethodPatch, "/documents/507f1f77bcf86cd799439011",
		`{"filename":"renombrado.pdf"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado status 200, obtenido %d (body: %s)", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type esperado application/json, obtenido %q", ct)
	}

	body := decodeBody(t, rec)
	if len(body) != 4 {
		t.Fatalf("esperados exactamente 4 campos, obtenidos %d: %v", len(body), body)
	}
	if body["id"] != "507f1f77bcf86cd799439011" {
		t.Errorf("id esperado %q, obtenido %v", "507f1f77bcf86cd799439011", body["id"])
	}
	if body["filename"] != "renombrado.pdf" {
		t.Errorf("filename esperado %q, obtenido %v", "renombrado.pdf", body["filename"])
	}
	if body["extracted_text"] != "texto extraído" {
		t.Errorf("extracted_text no debe mutar, obtenido %v", body["extracted_text"])
	}
	if body["checksum"] != "abc123" {
		t.Errorf("checksum no debe mutar, obtenido %v", body["checksum"])
	}

	if got := repo.byID["507f1f77bcf86cd799439011"].Filename; got != "renombrado.pdf" {
		t.Errorf("repositorio: filename esperado %q, obtenido %q", "renombrado.pdf", got)
	}
	if repo.updateCalls != 1 {
		t.Errorf("esperada 1 llamada a UpdateFilename, obtenidas %d", repo.updateCalls)
	}
}

// PATCH al mismo valor es idempotente -> 200.
func TestPatchDocument_SameValueIsIdempotent(t *testing.T) {
	repo := newFakeDocumentRepository()
	repo.byID["507f1f77bcf86cd799439011"] = seedDocument()
	router := newTestRouter(repo)

	rec := doRequest(t, router, http.MethodPatch, "/documents/507f1f77bcf86cd799439011",
		`{"filename":"original.pdf"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado status 200, obtenido %d (body: %s)", rec.Code, rec.Body.String())
	}
	if got := repo.byID["507f1f77bcf86cd799439011"].Filename; got != "original.pdf" {
		t.Errorf("filename esperado %q, obtenido %q", "original.pdf", got)
	}
}

// Campos no mutables en el payload se ignoran: checksum y extracted_text
// quedan intactos y la respuesta es 200.
func TestPatchDocument_IgnoresImmutableFields(t *testing.T) {
	repo := newFakeDocumentRepository()
	repo.byID["507f1f77bcf86cd799439011"] = seedDocument()
	router := newTestRouter(repo)

	rec := doRequest(t, router, http.MethodPatch, "/documents/507f1f77bcf86cd799439011",
		`{"filename":"renombrado.pdf","checksum":"HACKEADO","extracted_text":"HACKEADO"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado status 200, obtenido %d (body: %s)", rec.Code, rec.Body.String())
	}
	got := repo.byID["507f1f77bcf86cd799439011"]
	if got.Checksum != "abc123" {
		t.Errorf("checksum no debe mutar, obtenido %q", got.Checksum)
	}
	if got.ExtractedText != "texto extraído" {
		t.Errorf("extracted_text no debe mutar, obtenido %q", got.ExtractedText)
	}
}

// Rechazos 400: body ausente/vacío, JSON malformado, filename ausente o
// vacío. Un rename sin filename es una petición sin intención válida.
func TestPatchDocument_InvalidRequest_Returns400(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"body ausente", ""},
		{"JSON malformado", `{"filename":`},
		{"objeto vacío", `{}`},
		{"filename vacío", `{"filename":""}`},
		{"filename de tipo incorrecto", `{"filename":123}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeDocumentRepository()
			repo.byID["507f1f77bcf86cd799439011"] = seedDocument()
			router := newTestRouter(repo)

			rec := doRequest(t, router, http.MethodPatch, "/documents/507f1f77bcf86cd799439011", tt.body)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("esperado status 400, obtenido %d (body: %s)", rec.Code, rec.Body.String())
			}
			body := decodeBody(t, rec)
			if len(body) != 2 {
				t.Fatalf("envelope de error debe tener exactamente 2 campos, obtenidos %d: %v", len(body), body)
			}
			if body["code"] != "INVALID_REQUEST" {
				t.Errorf("code esperado %q, obtenido %v", "INVALID_REQUEST", body["code"])
			}
			if repo.updateCalls != 0 {
				t.Errorf("el repositorio no debe ser tocado, recibió %d llamadas", repo.updateCalls)
			}
		})
	}
}

// ID de formato inválido -> 400 fail-fast sin tocar el repositorio.
func TestPatchDocument_InvalidID_Returns400(t *testing.T) {
	repo := newFakeDocumentRepository()
	router := newTestRouter(repo)

	rec := doRequest(t, router, http.MethodPatch, "/documents/no-es-un-objectid",
		`{"filename":"renombrado.pdf"}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("esperado status 400, obtenido %d (body: %s)", rec.Code, rec.Body.String())
	}
	body := decodeBody(t, rec)
	if body["code"] != "INVALID_REQUEST" {
		t.Errorf("code esperado %q, obtenido %v", "INVALID_REQUEST", body["code"])
	}
	if repo.updateCalls != 0 {
		t.Errorf("fail-fast violado: el repositorio recibió %d llamadas con un ID inválido", repo.updateCalls)
	}
}

// ID válido pero inexistente -> 404 NOT_FOUND.
func TestPatchDocument_NotFound_Returns404(t *testing.T) {
	repo := newFakeDocumentRepository()
	router := newTestRouter(repo)

	rec := doRequest(t, router, http.MethodPatch, "/documents/507f1f77bcf86cd7994390ff",
		`{"filename":"renombrado.pdf"}`)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("esperado status 404, obtenido %d (body: %s)", rec.Code, rec.Body.String())
	}
	body := decodeBody(t, rec)
	if len(body) != 2 {
		t.Fatalf("envelope de error debe tener exactamente 2 campos, obtenidos %d: %v", len(body), body)
	}
	if body["code"] != "NOT_FOUND" {
		t.Errorf("code esperado %q, obtenido %v", "NOT_FOUND", body["code"])
	}
}

// Filename de 101 caracteres -> 422 FILENAME_TOO_LONG (validación en Service).
func TestPatchDocument_FilenameTooLong_Returns422(t *testing.T) {
	repo := newFakeDocumentRepository()
	repo.byID["507f1f77bcf86cd799439011"] = seedDocument()
	router := newTestRouter(repo)

	longFilename := strings.Repeat("a", 101) + ".pdf"
	payload, err := json.Marshal(map[string]string{"filename": longFilename})
	if err != nil {
		t.Fatalf("no se pudo construir el payload: %v", err)
	}

	rec := doRequest(t, router, http.MethodPatch, "/documents/507f1f77bcf86cd799439011", string(payload))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("esperado status 422, obtenido %d (body: %s)", rec.Code, rec.Body.String())
	}
	body := decodeBody(t, rec)
	if len(body) != 2 {
		t.Fatalf("envelope de error debe tener exactamente 2 campos, obtenidos %d: %v", len(body), body)
	}
	if body["code"] != "FILENAME_TOO_LONG" {
		t.Errorf("code esperado %q, obtenido %v", "FILENAME_TOO_LONG", body["code"])
	}
	if got := repo.byID["507f1f77bcf86cd799439011"].Filename; got != "original.pdf" {
		t.Errorf("el filename no debe mutar ante un 422, obtenido %q", got)
	}
	if repo.updateCalls != 0 {
		t.Errorf("el repositorio no debe ser tocado ante un 422, recibió %d llamadas", repo.updateCalls)
	}
}
