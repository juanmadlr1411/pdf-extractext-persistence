package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
