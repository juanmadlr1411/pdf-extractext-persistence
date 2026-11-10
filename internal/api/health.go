package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/pdf-extractext/persistence/internal/config"
	"github.com/pdf-extractext/persistence/internal/service"
)

const readinessTimeout = 2 * time.Second

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

func readinessHandler(checker service.HealthChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), readinessTimeout)
		defer cancel()

		if err := checker.Check(ctx); err != nil {
			respondError(w, codeDependencyUnavailable, "dependencia no disponible", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}
