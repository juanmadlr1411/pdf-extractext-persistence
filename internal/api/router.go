// Package api define el router HTTP y los handlers del servicio.
package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/pdf-extractext/persistence/internal/config"
	"github.com/pdf-extractext/persistence/internal/service"
)

// Recibe sus dependencias inyectadas para facilitar los tests.
func NewRouter(cfg config.Config, docService *service.DocumentService, healthChecker service.HealthChecker) http.Handler {
	r := chi.NewRouter()

	// Correlación: primero se registra para que el request ID esté en el
	// contexto disponible para el resto de middlewares, handlers y logs.
	r.Use(requestIDMiddleware)

	r.Use(panicRecoveryMiddleware)

	r.Post("/documents", createDocumentHandler(docService))
	r.Get("/documents", listDocumentsHandler(docService))
	r.Get("/documents/by-checksum/{checksum}", getDocumentByChecksumHandler(docService))
	r.Get("/documents/{id}", getDocumentByIDHandler(docService))
	r.Patch("/documents/{id}", patchDocumentHandler(docService))
	r.Delete("/documents/{id}", deleteDocumentHandler(docService))

	r.Get("/health", livenessHandler(cfg))
	r.Get("/health/ready", readinessHandler(healthChecker))
	// Alias convencional de orquestadores (Kubernetes-style probes).
	r.Get("/readyz", readinessHandler(healthChecker))

	return r
}
