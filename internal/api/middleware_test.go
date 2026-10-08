package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/pdf-extractext/persistence/internal/config"
)

// TestPanicRecoveryMiddleware verifica que un pánico en cualquier handler es
// interceptado: la app no crashea y se responde con HTTP 500 y el envelope
// contractual {"code": "INTERNAL_ERROR", "message": "..."} sin filtrar
// información interna (ni el mensaje del pánico ni stacktraces).
func TestPanicRecoveryMiddleware(t *testing.T) {
	t.Parallel()

	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		panic("falla catastrófica: nil pointer en mongo.Client")
	})

	handler := panicRecoveryMiddleware(panicHandler)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/documents", nil)

	// Si el middleware no recupera el pánico, este test falla con panic.
	handler.ServeHTTP(rec, req)

	res := rec.Result()
	if res.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusInternalServerError)
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
	if body["code"] != "INTERNAL_ERROR" {
		t.Errorf("code = %v, want INTERNAL_ERROR", body["code"])
	}
	msg, ok := body["message"].(string)
	if !ok || msg == "" {
		t.Fatalf("message ausente o vacío: %v", body["message"])
	}

	// Sin fugas de información: ni el mensaje del pánico ni stacktrace.
	raw := rec.Body.String()
	for _, leak := range []string{"falla catastrófica", "nil pointer", "goroutine", "panic"} {
		if strings.Contains(strings.ToLower(raw), strings.ToLower(leak)) {
			t.Errorf("la respuesta filtra información interna (%q): %s", leak, raw)
		}
	}
}

// TestPanicRecoveryMiddlewarePassthrough verifica que el middleware no
// interfiere con handlers que no disparan pánico.
func TestPanicRecoveryMiddlewarePassthrough(t *testing.T) {
	t.Parallel()

	okHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	handler := panicRecoveryMiddleware(okHandler)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rec.Code != http.StatusTeapot {
		t.Errorf("status = %d, want %d (el middleware debe dejar pasar la respuesta)", rec.Code, http.StatusTeapot)
	}
	if rec.Body.String() != `{"ok":true}` {
		t.Errorf("body = %q, el middleware no debe alterar el cuerpo", rec.Body.String())
	}
}

// --- Tests del middleware de correlación X-Request-ID (Issue #11) ---

// TestRequestIDMiddleware_PropagatesIncomingHeader verifica que, cuando el
// cliente envía el header X-Request-ID, el middleware propaga exactamente
// el mismo valor al header de la respuesta y al contexto de la petición.
// También comprueba que el header de respuesta ya está fijado antes de que
// el handler se ejecute (requisito: set antes de next.ServeHTTP).
func TestRequestIDMiddleware_PropagatesIncomingHeader(t *testing.T) {
	t.Parallel()

	const incomingID = "req-id-entrante-abc-123"

	var headerAtHandlerTime string
	var idFromContext string

	captureHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headerAtHandlerTime = w.Header().Get("X-Request-ID")
		idFromContext = RequestIDFromContext(r.Context())
	})

	handler := requestIDMiddleware(captureHandler)

	req := httptest.NewRequest(http.MethodGet, "/documents", nil)
	req.Header.Set("X-Request-ID", incomingID)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("X-Request-ID"); got != incomingID {
		t.Errorf("header de respuesta = %q, want %q (debe propagarse sin modificar)", got, incomingID)
	}
	if headerAtHandlerTime != incomingID {
		t.Errorf("header visible dentro del handler = %q, want %q (debe fijarse antes de next.ServeHTTP)", headerAtHandlerTime, incomingID)
	}
	if idFromContext != incomingID {
		t.Errorf("request ID del contexto = %q, want %q", idFromContext, incomingID)
	}
}

// TestRequestIDMiddleware_GeneratesUUIDv4WhenMissing verifica que, cuando el
// cliente NO envía el header X-Request-ID, el middleware genera un UUID v4
// válido, lo inyecta en el header de la respuesta y en el contexto, y que
// cada petición recibe un ID distinto.
func TestRequestIDMiddleware_GeneratesUUIDv4WhenMissing(t *testing.T) {
	t.Parallel()

	var idFromContext string
	captureHandler := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		idFromContext = RequestIDFromContext(r.Context())
	})

	handler := requestIDMiddleware(captureHandler)

	// serve ejecuta una petición y devuelve el ID del header de respuesta
	// junto al ID visible en el contexto durante esa misma petición.
	serve := func() (headerID, contextID string) {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/documents", nil))
		return rec.Header().Get("X-Request-ID"), idFromContext
	}

	firstHeaderID, firstContextID := serve()
	secondHeaderID, _ := serve()

	if firstHeaderID == "" {
		t.Fatal("el middleware debe generar un X-Request-ID cuando el request no lo trae")
	}

	parsed, err := uuid.Parse(firstHeaderID)
	if err != nil {
		t.Fatalf("X-Request-ID generado (%q) no es un UUID válido: %v", firstHeaderID, err)
	}
	if v := parsed.Version(); v != 4 {
		t.Errorf("X-Request-ID generado (%q) es UUID versión %d, want 4", firstHeaderID, v)
	}

	if secondHeaderID == firstHeaderID {
		t.Errorf("cada petición debe recibir un ID distinto: first = second = %q", firstHeaderID)
	}

	if firstContextID != firstHeaderID {
		t.Errorf("request ID del contexto (%q) debe coincidir con el del header (%q)", firstContextID, firstHeaderID)
	}
}

// TestRouter_SetsXRequestIDGlobally verifica la integración: el router chi
// inyecta el middleware de correlación globalmente, por lo que toda
// respuesta incluye el header X-Request-ID.
func TestRouter_SetsXRequestIDGlobally(t *testing.T) {
	t.Parallel()

	cfg := config.Config{AppName: "pdf-extractext-persistence", Environment: "local"}
	router := NewRouter(cfg, nil, nil) // /health no requiere dependencias

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200 en /health, obtenido %d (body: %s)", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("X-Request-ID") == "" {
		t.Error("el router debe inyectar X-Request-ID en toda respuesta (middleware global)")
	}
}
