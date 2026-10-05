// Package api define el router HTTP y los handlers del servicio.
package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/pdf-extractext/persistence/internal/config"
	"github.com/pdf-extractext/persistence/internal/service"
)

// NewRouter construye el router del servicio con todos sus endpoints.
// Recibe sus dependencias inyectadas para facilitar los tests.
func NewRouter(cfg config.Config, docService *service.DocumentService, healthChecker service.HealthChecker) http.Handler {
	r := chi.NewRouter()

	r.Post("/documents", createDocumentHandler(docService))
	r.Get("/documents", listDocumentsHandler(docService))
	r.Get("/documents/{id}", getDocumentByIDHandler(docService))
	r.Patch("/documents/{id}", patchDocumentHandler(docService))
	r.Delete("/documents/{id}", deleteDocumentHandler(docService))

	// Liveness: no depende de ninguna dependencia externa.
	r.Get("/health", livenessHandler(cfg))
	// Readiness: verifica el estado de la dependencia crítica (Mongo).
	r.Get("/health/ready", readinessHandler(healthChecker))

	return r
}
