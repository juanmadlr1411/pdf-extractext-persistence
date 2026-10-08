package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/pdf-extractext/persistence/internal/config"
	"github.com/pdf-extractext/persistence/internal/service"
)

// readinessTimeout acota el ping a la dependencia crítica.
const readinessTimeout = 2 * time.Second

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

// readinessHandler maneja GET /health/ready (y su alias GET /readyz).
// Devuelve 200 si la dependencia crítica responde al ping dentro del timeout;
// 503 DEPENDENCY_UNAVAILABLE en caso contrario.
func readinessHandler(checker service.HealthChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), readinessTimeout)
		defer cancel()

		if err := checker.Check(ctx); err != nil {
			writeError(w, http.StatusServiceUnavailable, codeDependencyUnavailable, "dependencia no disponible")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}
