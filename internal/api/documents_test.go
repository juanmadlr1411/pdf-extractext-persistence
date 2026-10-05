package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/pdf-extractext/persistence/internal/config"
	"github.com/pdf-extractext/persistence/internal/domain"
	"github.com/pdf-extractext/persistence/internal/service"
)

// fakeDocumentRepository es un repositorio en memoria solo para tests de API.
// Guarda documentos indexados por ID y cuenta llamadas para verificar
// que la capa API corta los errores antes de tocar persistencia.
type fakeDocumentRepository struct {
	docs        []domain.Document
	byID        map[string]domain.Document
	saveErr     error // si no es nil, Save lo devuelve (simula fallos del almacén)
	saveCalls   int
	findCalls   int // cuenta FindAll + FindByID
	updateCalls int
	deleteCalls int
}

func newFakeDocumentRepository() *fakeDocumentRepository {
	return &fakeDocumentRepository{byID: make(map[string]domain.Document)}
}

func (f *fakeDocumentRepository) Save(_ context.Context, doc domain.Document) (domain.Document, error) {
	f.saveCalls++
	if f.saveErr != nil {
		return domain.Document{}, f.saveErr
	}
	f.docs = append(f.docs, doc)
	f.byID[doc.ID] = doc
	return doc, nil
}

func (f *fakeDocumentRepository) FindAll(_ context.Context) ([]domain.Document, error) {
	f.findCalls++
	return f.docs, nil
}

func (f *fakeDocumentRepository) FindByID(_ context.Context, id string) (domain.Document, error) {
	f.findCalls++
	doc, ok := f.byID[id]
	if !ok {
		return domain.Document{}, service.ErrDocumentNotFound
	}
	return doc, nil
}

func (f *fakeDocumentRepository) UpdateFilename(_ context.Context, id, filename string) (domain.Document, error) {
	f.updateCalls++
	doc, ok := f.byID[id]
	if !ok {
		return domain.Document{}, service.ErrDocumentNotFound
	}
	doc.Filename = filename
	f.byID[id] = doc
	for i := range f.docs {
		if f.docs[i].ID == id {
			f.docs[i].Filename = filename
		}
	}
	return doc, nil
}

func (f *fakeDocumentRepository) DeleteByID(_ context.Context, id string) error {
	f.deleteCalls++
	if _, ok := f.byID[id]; !ok {
		return service.ErrDocumentNotFound
	}
	delete(f.byID, id)
	for i, d := range f.docs {
		if d.ID == id {
			f.docs = append(f.docs[:i], f.docs[i+1:]...)
			break
		}
	}
	return nil
}

// --- helpers ---

func newTestRouter(repo service.DocumentRepository) http.Handler {
	cfg := config.Config{AppName: "pdf-extractext-persistence", Environment: "local"}
	return NewRouter(cfg, service.NewDocumentService(repo), nil)
}

func doRequest(t *testing.T, router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Buffer
	if body == "" {
		reader = &bytes.Buffer{}
	} else {
		reader = bytes.NewBufferString(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("cuerpo no es JSON válido: %v", err)
	}
	return body
}

// --- Tests de la Issue #4 (en verde) ---

func TestCreateDocument_Returns201WithFourFields(t *testing.T) {
	repo := newFakeDocumentRepository()
	router := newTestRouter(repo)

	rec := doRequest(t, router, http.MethodPost, "/documents",
		`{"filename":"informe.pdf","extracted_text":"texto extraído","checksum":"abc123"}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("esperado status 201, obtenido %d (body: %s)", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type esperado application/json, obtenido %q", ct)
	}

	body := decodeBody(t, rec)
	if len(body) != 4 {
		t.Fatalf("esperados exactamente 4 campos, obtenidos %d: %v", len(body), body)
	}
	id, ok := body["id"].(string)
	if !ok {
		t.Fatalf("campo 'id' ausente o no es string: %v", body["id"])
	}
	if !regexp.MustCompile(`^[0-9a-f]{24}$`).MatchString(id) {
		t.Errorf("id %q no cumple la regex ^[0-9a-f]{24}$", id)
	}
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

// --- Tests de la Issue #6: contrato de errores críticos del POST ---

// Caso 1: checksum duplicado -> 409 DUPLICATE_CHECKSUM con el documento
// existente completo en el envelope (estándar plano + document).
func TestCreateDocument_DuplicateChecksum_Returns409WithExistingDocument(t *testing.T) {
	existing := domain.PDFDocument{
		ID:            "507f1f77bcf86cd799439011",
		Filename:      "informe-original.pdf",
		ExtractedText: "texto previamente extraído",
		Checksum:      "sha256:abc123",
	}
	repo := newFakeDocumentRepository()
	repo.saveErr = &domain.DuplicateChecksumError{Existing: existing}
	router := newTestRouter(repo)

	rec := doRequest(t, router, http.MethodPost, "/documents",
		`{"filename":"informe.pdf","extracted_text":"texto extraído","checksum":"sha256:abc123"}`)

	if rec.Code != http.StatusConflict {
		t.Fatalf("esperado status 409, obtenido %d (body: %s)", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type esperado application/json, obtenido %q", ct)
	}

	body := decodeBody(t, rec)
	// Envelope estándar (plano) + documento existente: exactamente 3 claves.
	if len(body) != 3 {
		t.Fatalf("envelope 409 debe tener exactamente 3 claves {code,message,document}, obtenidas %d: %v", len(body), body)
	}
	if body["code"] != "DUPLICATE_CHECKSUM" {
		t.Errorf("code esperado %q, obtenido %v", "DUPLICATE_CHECKSUM", body["code"])
	}
	if msg, ok := body["message"].(string); !ok || msg == "" {
		t.Errorf("message debe ser un string no vacío, obtenido %v", body["message"])
	}

	doc, ok := body["document"].(map[string]any)
	if !ok {
		t.Fatalf("'document' debe ser un objeto JSON, obtenido %v", body["document"])
	}
	// El documento existente llega completo: exactamente los 4 campos.
	if len(doc) != 4 {
		t.Fatalf("documento existente debe tener exactamente 4 campos, obtenidos %d: %v", len(doc), doc)
	}
	if doc["id"] != existing.ID {
		t.Errorf("document.id esperado %q, obtenido %v", existing.ID, doc["id"])
	}
	if doc["filename"] != existing.Filename {
		t.Errorf("document.filename esperado %q, obtenido %v", existing.Filename, doc["filename"])
	}
	if doc["extracted_text"] != existing.ExtractedText {
		t.Errorf("document.extracted_text esperado %q, obtenido %v", existing.ExtractedText, doc["extracted_text"])
	}
	if doc["checksum"] != existing.Checksum {
		t.Errorf("document.checksum esperado %q, obtenido %v", existing.Checksum, doc["checksum"])
	}

	// El conflicto se detecta en el único intento de inserción.
	if repo.saveCalls != 1 {
		t.Errorf("esperada 1 llamada a Save, obtenidas %d", repo.saveCalls)
	}
}

// Caso 2: almacén caído -> 503 DEPENDENCY_UNAVAILABLE, envelope plano exacto.
func TestCreateDocument_DependencyUnavailable_Returns503(t *testing.T) {
	repo := newFakeDocumentRepository()
	// El repositorio real envuelve el sentinel con la causa del driver.
	repo.saveErr = fmt.Errorf("InsertOne: %w", domain.ErrDependencyUnavailable)
	router := newTestRouter(repo)

	rec := doRequest(t, router, http.MethodPost, "/documents",
		`{"filename":"informe.pdf","extracted_text":"texto extraído","checksum":"sha256:abc123"}`)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("esperado status 503, obtenido %d (body: %s)", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type esperado application/json, obtenido %q", ct)
	}

	body := decodeBody(t, rec)
	// Envelope estándar exacto: solo {code, message}, sin document ni extra.
	if len(body) != 2 {
		t.Fatalf("envelope 503 debe tener exactamente 2 claves {code,message}, obtenidas %d: %v", len(body), body)
	}
	if body["code"] != "DEPENDENCY_UNAVAILABLE" {
		t.Errorf("code esperado %q, obtenido %v", "DEPENDENCY_UNAVAILABLE", body["code"])
	}
	if msg, ok := body["message"].(string); !ok || msg == "" {
		t.Errorf("message debe ser un string no vacío, obtenido %v", body["message"])
	}
}

// Caso 3: fallo inesperado del repositorio -> 500 INTERNAL_ERROR.
func TestCreateDocument_UnexpectedError_Returns500(t *testing.T) {
	repo := newFakeDocumentRepository()
	repo.saveErr = errors.New("fallo inesperado")
	router := newTestRouter(repo)

	rec := doRequest(t, router, http.MethodPost, "/documents",
		`{"filename":"informe.pdf","extracted_text":"texto extraído","checksum":"sha256:abc123"}`)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("esperado status 500, obtenido %d (body: %s)", rec.Code, rec.Body.String())
	}

	body := decodeBody(t, rec)
	if len(body) != 2 {
		t.Fatalf("envelope 500 debe tener exactamente 2 claves {code,message}, obtenidas %d: %v", len(body), body)
	}
	if body["code"] != "INTERNAL_ERROR" {
		t.Errorf("code esperado %q, obtenido %v", "INTERNAL_ERROR", body["code"])
	}
}

// --- Tests de la Issue #7 ---

// Caso 1: GET /documents con repositorio vacío -> 200 y array JSON vacío.
func TestListDocuments_EmptyRepo_ReturnsEmptyJSONArray(t *testing.T) {
	repo := newFakeDocumentRepository()
	router := newTestRouter(repo)

	rec := doRequest(t, router, http.MethodGet, "/documents", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado status 200, obtenido %d (body: %s)", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type esperado application/json, obtenido %q", ct)
	}
	// Decisión contractual: lista vacía se serializa como [], nunca null.
	var body []any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("cuerpo no es un array JSON válido: %v (body: %s)", err, rec.Body.String())
	}
	if len(body) != 0 {
		t.Fatalf("esperado array vacío, obtenidos %d elementos: %v", len(body), body)
	}
	if rec.Body.String() == "null\n" || rec.Body.String() == "null" {
		t.Fatal("la lista vacía no debe serializarse como null")
	}
}

// Caso 2: GET /documents con N documentos -> 200 y array con los N documentos.
func TestListDocuments_WithDocs_ReturnsAllDocuments(t *testing.T) {
	repo := newFakeDocumentRepository()
	repo.docs = []domain.Document{
		{ID: "507f1f77bcf86cd799439011", Filename: "a.pdf", ExtractedText: "texto a", Checksum: "sum-a"},
		{ID: "507f1f77bcf86cd799439012", Filename: "b.pdf", ExtractedText: "texto b", Checksum: "sum-b"},
	}
	repo.byID = map[string]domain.Document{
		repo.docs[0].ID: repo.docs[0],
		repo.docs[1].ID: repo.docs[1],
	}
	router := newTestRouter(repo)

	rec := doRequest(t, router, http.MethodGet, "/documents", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado status 200, obtenido %d (body: %s)", rec.Code, rec.Body.String())
	}

	var body []map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("cuerpo no es un array JSON válido: %v", err)
	}
	if len(body) != 2 {
		t.Fatalf("esperados 2 documentos, obtenidos %d: %v", len(body), body)
	}

	for i, want := range repo.docs {
		got := body[i]
		if len(got) != 4 {
			t.Errorf("documento %d: esperados exactamente 4 campos, obtenidos %d: %v", i, len(got), got)
		}
		if got["id"] != want.ID {
			t.Errorf("documento %d: id esperado %q, obtenido %v", i, want.ID, got["id"])
		}
		if got["filename"] != want.Filename {
			t.Errorf("documento %d: filename esperado %q, obtenido %v", i, want.Filename, got["filename"])
		}
		if got["extracted_text"] != want.ExtractedText {
			t.Errorf("documento %d: extracted_text esperado %q, obtenido %v", i, want.ExtractedText, got["extracted_text"])
		}
		if got["checksum"] != want.Checksum {
			t.Errorf("documento %d: checksum esperado %q, obtenido %v", i, want.Checksum, got["checksum"])
		}
	}
}

// Caso 3: GET /documents/{id} con ID de formato inválido -> 400 INVALID_REQUEST
// y el repositorio NO debe ser tocado (fail-fast).
func TestGetDocumentByID_InvalidID_Returns400(t *testing.T) {
	invalidIDs := []string{
		"no-es-un-objectid",
		"507f1f77bcf86cd79943901",   // 23 caracteres
		"507f1f77bcf86cd7994390111", // 25 caracteres
		"507F1F77BCF86CD799439011",  // mayúsculas
		"zz7f1f77bcf86cd799439011",  // caracteres no hex
	}

	for _, id := range invalidIDs {
		t.Run(id, func(t *testing.T) {
			repo := newFakeDocumentRepository()
			router := newTestRouter(repo)

			rec := doRequest(t, router, http.MethodGet, "/documents/"+id, "")

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("esperado status 400, obtenido %d (body: %s)", rec.Code, rec.Body.String())
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Fatalf("Content-Type esperado application/json, obtenido %q", ct)
			}

			body := decodeBody(t, rec)
			if len(body) != 2 {
				t.Fatalf("envelope de error debe tener exactamente 2 campos, obtenidos %d: %v", len(body), body)
			}
			if body["code"] != "INVALID_REQUEST" {
				t.Errorf("code esperado %q, obtenido %v", "INVALID_REQUEST", body["code"])
			}
			if msg, ok := body["message"].(string); !ok || msg == "" {
				t.Errorf("message debe ser un string no vacío, obtenido %v", body["message"])
			}

			// Fail-fast: el handler no debe haber tocado el repositorio.
			if repo.findCalls != 0 {
				t.Errorf("fail-fast violado: el repositorio recibió %d llamadas con un ID inválido", repo.findCalls)
			}
		})
	}
}

// Caso 4: GET /documents/{id} con ID válido pero inexistente -> 404 NOT_FOUND.
func TestGetDocumentByID_NotFound_Returns404(t *testing.T) {
	repo := newFakeDocumentRepository()
	router := newTestRouter(repo)

	rec := doRequest(t, router, http.MethodGet, "/documents/507f1f77bcf86cd7994390ff", "")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("esperado status 404, obtenido %d (body: %s)", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type esperado application/json, obtenido %q", ct)
	}

	body := decodeBody(t, rec)
	if len(body) != 2 {
		t.Fatalf("envelope de error debe tener exactamente 2 campos, obtenidos %d: %v", len(body), body)
	}
	if body["code"] != "NOT_FOUND" {
		t.Errorf("code esperado %q, obtenido %v", "NOT_FOUND", body["code"])
	}
	if msg, ok := body["message"].(string); !ok || msg == "" {
		t.Errorf("message debe ser un string no vacío, obtenido %v", body["message"])
	}

	// El ID era válido: el repositorio SÍ debió ser consultado exactamente una vez.
	if repo.findCalls != 1 {
		t.Errorf("esperada 1 llamada al repositorio, obtenidas %d", repo.findCalls)
	}
}

// Caso 5: GET /documents/{id} con ID válido y existente -> 200 con el documento exacto.
func TestGetDocumentByID_Found_Returns200WithDocument(t *testing.T) {
	want := domain.Document{
		ID:            "507f1f77bcf86cd799439011",
		Filename:      "informe.pdf",
		ExtractedText: "texto extraído",
		Checksum:      "abc123",
	}
	repo := newFakeDocumentRepository()
	repo.byID[want.ID] = want
	router := newTestRouter(repo)

	rec := doRequest(t, router, http.MethodGet, "/documents/"+want.ID, "")

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
	if body["id"] != want.ID {
		t.Errorf("id esperado %q, obtenido %v", want.ID, body["id"])
	}
	if body["filename"] != want.Filename {
		t.Errorf("filename esperado %q, obtenido %v", want.Filename, body["filename"])
	}
	if body["extracted_text"] != want.ExtractedText {
		t.Errorf("extracted_text esperado %q, obtenido %v", want.ExtractedText, body["extracted_text"])
	}
	if body["checksum"] != want.Checksum {
		t.Errorf("checksum esperado %q, obtenido %v", want.Checksum, body["checksum"])
	}

	if repo.findCalls != 1 {
		t.Errorf("esperada 1 llamada al repositorio, obtenidas %d", repo.findCalls)
	}
}
