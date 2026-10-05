// Package contract contiene la suite de contract tests lado proveedor que
// verifica la tabla completa del contrato C2 del Persistence Service contra
// el servicio real levantado.
//
// La suite es black-box: ejercita el router chi de producción por HTTP real
// (servidor efímero de httptest) sustituyendo las dependencias externas
// (MongoDB) por un almacén en memoria con semántica contractual equivalente:
// índice único sobre checksum, sentinels de recurso ausente y de almacén no
// disponible.
//
// Regla de oro de la suite: los valores esperados (códigos del catálogo,
// shapes del envelope, campos del documento) se declaran aquí como literales
// del documento de contrato, sin importar constantes de la implementación.
// Si el servicio diverge del contrato, esta suite debe fallar citando la
// regla violada.
package contract

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/pdf-extractext/persistence/internal/api"
	"github.com/pdf-extractext/persistence/internal/config"
	"github.com/pdf-extractext/persistence/internal/domain"
	"github.com/pdf-extractext/persistence/internal/service"
)

// --- Almacén en memoria con semántica contractual (sustituto de MongoDB) ---

// contractStore implementa service.DocumentRepository replicando la semántica
// que el contrato le exige al almacén real: índice único sobre checksum
// (conflicto con documento existente), ErrDocumentNotFound para recursos
// ausentes y errores de indisponibilidad indistinguibles de los del driver.
type contractStore struct {
	mu          sync.Mutex
	byID        map[string]domain.Document
	checksumIdx map[string]string // checksum -> id: índice único como en Mongo
	unavailable bool              // simula MongoDB caído
	saveCalls   int
}

func newContractStore() *contractStore {
	return &contractStore{
		byID:        make(map[string]domain.Document),
		checksumIdx: make(map[string]string),
	}
}

// Save replica el flujo anti-TOCTOU del repositorio real: un único intento
// de inserción; si el índice único de checksum reporta conflicto, devuelve
// el documento existente completo envuelto en *domain.DuplicateChecksumError.
func (s *contractStore) Save(_ context.Context, doc domain.Document) (domain.Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saveCalls++

	if s.unavailable {
		// Igual que el repo real: el sentinel viaja envuelto en la cadena
		// de errores y debe detectarse con errors.Is.
		return domain.Document{}, fmt.Errorf("no se pudo insertar el documento: %w", service.ErrDependencyUnavailable)
	}
	if existingID, exists := s.checksumIdx[doc.Checksum]; exists {
		existing := s.byID[existingID]
		return domain.Document{}, &domain.DuplicateChecksumError{Existing: domain.PDFDocument{
			ID:            existing.ID,
			Filename:      existing.Filename,
			ExtractedText: existing.ExtractedText,
			Checksum:      existing.Checksum,
		}}
	}

	s.byID[doc.ID] = doc
	s.checksumIdx[doc.Checksum] = doc.ID
	return doc, nil
}

// FindAll devuelve todos los documentos (orden no garantizado, como en Mongo).
func (s *contractStore) FindAll(_ context.Context) ([]domain.Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	docs := make([]domain.Document, 0, len(s.byID))
	for _, doc := range s.byID {
		docs = append(docs, doc)
	}
	return docs, nil
}

// FindByID devuelve el documento o ErrDocumentNotFound si no existe.
func (s *contractStore) FindByID(_ context.Context, id string) (domain.Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, ok := s.byID[id]
	if !ok {
		return domain.Document{}, service.ErrDocumentNotFound
	}
	return doc, nil
}

// UpdateFilename renombra o devuelve ErrDocumentNotFound si el ID no existe.
func (s *contractStore) UpdateFilename(_ context.Context, id, filename string) (domain.Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, ok := s.byID[id]
	if !ok {
		return domain.Document{}, service.ErrDocumentNotFound
	}
	doc.Filename = filename
	s.byID[id] = doc
	return doc, nil
}

// DeleteByID elimina o devuelve ErrDocumentNotFound si ya no existe.
func (s *contractStore) DeleteByID(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, ok := s.byID[id]
	if !ok {
		return service.ErrDocumentNotFound
	}
	delete(s.byID, id)
	delete(s.checksumIdx, doc.Checksum)
	return nil
}

// seed dispone estado inicial en el almacén sin pasar por POST.
func (s *contractStore) seed(doc domain.Document) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byID[doc.ID] = doc
	s.checksumIdx[doc.Checksum] = doc.ID
}

// --- Sustituto del health checker de la dependencia crítica ---

// stubHealthChecker simula la dependencia crítica (MongoDB): err nil es sana.
type stubHealthChecker struct {
	err error
}

func (c stubHealthChecker) Check(_ context.Context) error { return c.err }

// --- Servidor del proveedor: router de producción por HTTP real ---

// newContractServer levanta el servicio real (router chi de producción) en
// un servidor HTTP efímero con las dependencias aisladas del exterior.
func newContractServer(t *testing.T, store service.DocumentRepository, checker service.HealthChecker) *httptest.Server {
	t.Helper()
	cfg := config.Config{AppName: "pdf-extractext-persistence", Environment: "local"}
	router := api.NewRouter(cfg, service.NewDocumentService(store), checker)
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return srv
}

// --- Cliente HTTP real ---

// contractResponse captura la respuesta observable por el consumidor.
type contractResponse struct {
	status int
	header http.Header
	body   string
}

// doRequest ejecuta una petición HTTP real contra el servicio levantado.
func doRequest(t *testing.T, srv *httptest.Server, method, path, body string, headers map[string]string) contractResponse {
	t.Helper()

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, srv.URL+path, reader)
	if err != nil {
		t.Fatalf("no se pudo construir la petición %s %s: %v", method, path, err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("la petición HTTP real %s %s falló: %v", method, path, err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("no se pudo leer el cuerpo de la respuesta %s %s: %v", method, path, err)
	}
	return contractResponse{status: res.StatusCode, header: res.Header, body: string(raw)}
}

// --- Aserciones: cada fallo cita la regla del contrato C2 violada ---

// objectIDPattern es la regla de dominio del id: hexadecimal de 24 minúsculas.
var objectIDPattern = regexp.MustCompile(`^[0-9a-f]{24}$`)

// assertStatus verifica el status HTTP que la fila del contrato asigna.
func assertStatus(t *testing.T, rule string, got, want int) {
	t.Helper()
	if got != want {
		t.Errorf("%s: REGLA VIOLADA — status %d, el contrato exige %d", rule, got, want)
	}
}

// assertJSONContentType verifica el Content-Type de las respuestas con cuerpo.
func assertJSONContentType(t *testing.T, rule string, res contractResponse) {
	t.Helper()
	if ct := res.header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("%s: REGLA VIOLADA — Content-Type %q, el contrato exige application/json", rule, ct)
	}
}

// decodeJSONObject exige que el cuerpo sea un objeto JSON válido.
func decodeJSONObject(t *testing.T, rule, raw string) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.NewDecoder(strings.NewReader(raw)).Decode(&body); err != nil {
		t.Fatalf("%s: REGLA VIOLADA — el cuerpo debe ser un objeto JSON válido: %v (body: %s)", rule, err, raw)
	}
	return body
}

// jsonDecodeArray decodifica un cuerpo como array JSON.
func jsonDecodeArray(raw string, into *[]map[string]any) error {
	return json.NewDecoder(strings.NewReader(raw)).Decode(into)
}

// assertField compara un campo individual del cuerpo contra el contrato.
func assertField(t *testing.T, rule, key string, body map[string]any, want string) {
	t.Helper()
	if got := body[key]; got != want {
		t.Errorf("%s: REGLA VIOLADA — campo %q = %v, el contrato exige %q", rule, key, got, want)
	}
}

// assertErrorEnvelope verifica el envelope de error ESTRICTO: exactamente
// {code, message} con el code del catálogo. En los errores estándar ningún
// campo extra está permitido (ni details, ni stacktrace, ni document).
func assertErrorEnvelope(t *testing.T, rule, raw, wantCode string) map[string]any {
	t.Helper()
	body := decodeJSONObject(t, rule, raw)
	if len(body) != 2 {
		t.Errorf("%s: REGLA VIOLADA — el envelope debe ser estricto {code, message} sin campos extra; claves obtenidas %d: %v", rule, len(body), body)
	}
	if body["code"] != wantCode {
		t.Errorf("%s: REGLA VIOLADA — code %v, el catálogo del contrato exige %q", rule, body["code"], wantCode)
	}
	if msg, ok := body["message"].(string); !ok || msg == "" {
		t.Errorf("%s: REGLA VIOLADA — message debe ser un string no vacío; obtenido %v", rule, body["message"])
	}
	return body
}

// assertContractDocument verifica la serialización EXACTA de un documento:
// exactamente los 4 campos {id, filename, extracted_text, checksum}.
func assertContractDocument(t *testing.T, rule string, body map[string]any, want domain.Document) {
	t.Helper()
	if len(body) != 4 {
		t.Errorf("%s: REGLA VIOLADA — un documento se serializa con exactamente 4 campos {id, filename, extracted_text, checksum}; obtenidos %d: %v", rule, len(body), body)
	}
	assertField(t, rule, "id", body, want.ID)
	assertField(t, rule, "filename", body, want.Filename)
	assertField(t, rule, "extracted_text", body, want.ExtractedText)
	assertField(t, rule, "checksum", body, want.Checksum)
}

// assertObjectIDFormat verifica la regla de dominio del id: hex de 24.
func assertObjectIDFormat(t *testing.T, rule, id string) {
	t.Helper()
	if !objectIDPattern.MatchString(id) {
		t.Errorf("%s: REGLA VIOLADA — id %q no cumple el formato del contrato (^[0-9a-f]{24}$)", rule, id)
	}
}

// assertNoBody verifica que la respuesta viaja sin cuerpo (204).
func assertNoBody(t *testing.T, rule, body string) {
	t.Helper()
	if body != "" {
		t.Errorf("%s: REGLA VIOLADA — la respuesta debe viajar sin cuerpo; obtenido %q", rule, body)
	}
}
