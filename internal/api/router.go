// Package api define el router HTTP y los handlers del servicio.
package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/pdf-extractext/persistence/internal/config"
	"github.com/pdf-extractext/persistence/internal/service"
)

// NewRouter construye el router del servicio con todos sus endpoints.
// Recibe sus dependencias inyectadas para facilitar los tests.
func NewRouter(cfg config.Config, docService *service.DocumentService) http.Handler {
	r := chi.NewRouter()

	r.Post("/documents", createDocumentHandler(docService))

	// Handler como closure sobre cfg: no captura estado global.
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
