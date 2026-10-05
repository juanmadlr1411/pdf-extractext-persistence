package api

import (
	"context"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/google/uuid"
)

// HeaderXRequestID es el header HTTP usado para correlacionar peticiones
// entre servicios (trazabilidad distribuida).
const HeaderXRequestID = "X-Request-ID"

// requestIDContextKey es el tipo de la clave de contexto del request ID.
// Un tipo no exportado evita colisiones con claves de otros paquetes.
type requestIDContextKey struct{}

// RequestIDFromContext devuelve el ID de correlación de la petición
// almacenado en el contexto, o "" si no existe. Permite que handlers y
// capas inferiores lo usen como atributo estructurado en logs de slog.
func RequestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(requestIDContextKey{}).(string)
	return id
}

// requestIDMiddleware implementa la correlación de peticiones (Issue #11):
// propaga el header X-Request-ID entrante o genera un UUID v4 si llega
// vacío, lo añade al header de la respuesta ANTES de ejecutar el siguiente
// handler y lo inyecta en el contexto de la petición.
func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get(HeaderXRequestID)
		if requestID == "" {
			requestID = uuid.NewString()
		}

		// El ID se fija antes de next.ServeHTTP para que esté disponible
		// en la respuesta aunque el handler falle a mitad de camino.
		w.Header().Set(HeaderXRequestID, requestID)

		ctx := context.WithValue(r.Context(), requestIDContextKey{}, requestID)
		r = r.WithContext(ctx)

		slog.Info("solicitud iniciada", "request_id", requestID, "method", r.Method, "path", r.URL.Path)
		next.ServeHTTP(w, r)
		slog.Info("solicitud finalizada", "request_id", requestID, "method", r.Method, "path", r.URL.Path)
	})
}

// panicRecoveryMiddleware intercepta cualquier pánico en los handlers,
// evita que la aplicación crashee y responde con el envelope contractual
// INTERNAL_ERROR (500) sin filtrar información interna al cliente.
// El detalle del pánico solo se registra en el log del servidor.
func panicRecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("panic recuperado en handler",
					"method", r.Method,
					"path", r.URL.Path,
					"request_id", RequestIDFromContext(r.Context()),
					"panic", rec,
					"stack", string(debug.Stack()),
				)
				respondError(w, codeInternalError, "error interno del servidor", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
