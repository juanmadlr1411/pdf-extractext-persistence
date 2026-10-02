// FASE GREEN: Implementación mínima para pasar el test.
package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/pdf-extractext/persistence/internal/config"
)

// NewRouter construye el router del servicio con todos sus endpoints.
// Es una función pura de Config para facilitar la inyección en tests.
func NewRouter(cfg config.Config) http.Handler {
	r := chi.NewRouter()

	// FASE REFACTOR: handler extraído como closure sobre cfg (la ruta no
	// captura estado global; se puede mover a un método los handlers crezcan).
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":      "ok",
			"service":     cfg.AppName,
			"environment": cfg.Environment,
		})
	})

	return r
}
