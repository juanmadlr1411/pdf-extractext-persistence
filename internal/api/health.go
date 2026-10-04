package api

import (
	"encoding/json"
	"net/http"

	"github.com/pdf-extractext/persistence/internal/config"
	"github.com/pdf-extractext/persistence/internal/service"
)

// livenessHandler maneja GET /health. Confirma que el servicio está levantado.
// Intencionalmente NO depende de ninguna dependencia externa (Mongo, etc.).
func livenessHandler(cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":      "ok",
			"service":     cfg.AppName,
			"environment": cfg.Environment,
		})
	}
}

// readinessHandler maneja GET /health/ready. Devuelve 200 si la dependencia
// crítica responde al health check; 503 DEPENDENCY_UNAVAILABLE en caso contrario.
func readinessHandler(checker service.HealthChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := checker.Check(r.Context()); err != nil {
			writeError(w, http.StatusServiceUnavailable, codeDependencyUnavailable, "dependencia no disponible")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}
