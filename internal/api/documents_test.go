package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/pdf-extractext/persistence/internal/config"
	"github.com/pdf-extractext/persistence/internal/domain"
	"github.com/pdf-extractext/persistence/internal/service"
)

// fakeDocumentRepository es un repositorio en memoria solo para tests de API.
type fakeDocumentRepository struct{}

func (fakeDocumentRepository) Save(_ context.Context, doc domain.Document) (domain.Document, error) {
	// El fake devuelve el documento tal cual; el servicio es quien genera el ID.
	return doc, nil
}

func TestCreateDocument_Returns201WithFourFields(t *testing.T) {
	// Arrange
	cfg := config.Config{AppName: "pdf-extractext-persistence", Environment: "local"}
	svc := service.NewDocumentService(fakeDocumentRepository{})
	router := NewRouter(cfg, svc)

	payload := `{"filename":"informe.pdf","extracted_text":"texto extraído","checksum":"abc123"}`
	req := httptest.NewRequest(http.MethodPost, "/documents", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	// Act
	router.ServeHTTP(rec, req)

	// Assert: status
	if rec.Code != http.StatusCreated {
		t.Fatalf("esperado status 201, obtenido %d (body: %s)", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type esperado application/json, obtenido %q", ct)
	}

	// Assert: cuerpo con EXACTAMENTE los 4 campos del contrato
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("cuerpo no es JSON válido: %v", err)
	}
	if len(body) != 4 {
		t.Fatalf("esperados exactamente 4 campos en la respuesta, obtenidos %d: %v", len(body), body)
	}

	// id: string hexadecimal de 24 caracteres
	id, ok := body["id"].(string)
	if !ok {
		t.Fatalf("campo 'id' ausente o no es string: %v", body["id"])
	}
	if matched := regexp.MustCompile(`^[0-9a-f]{24}$`).MatchString(id); !matched {
		t.Errorf("id %q no cumple la regex ^[0-9a-f]{24}$", id)
	}

	// eco de los campos enviados
	if body["filename"] != "informe.pdf" {
		t.Errorf("filename esperado %q, obtenido %v", "informe.pdf", body["filename"])
	}
	if body["extracted_text"] != "texto extraído" {
		t.Errorf("extracted_text esperado %q, obtenido %v", "texto extraído", body["extracted_text"])
	}
	if body["checksum"] != "abc123" {
		t.Errorf("checksum esperado %q, obtenido %v", "abc123", body["checksum"])
	}
}
