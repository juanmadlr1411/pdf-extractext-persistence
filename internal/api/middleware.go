package api

import (
	"log"
	"net/http"
	"runtime/debug"
)

// panicRecoveryMiddleware intercepta cualquier pánico en los handlers,
// evita que la aplicación crashee y responde con el envelope contractual
// INTERNAL_ERROR (500) sin filtrar información interna al cliente.
// El detalle del pánico solo se registra en el log del servidor.
func panicRecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("panic recuperado en %s %s: %v\n%s", r.Method, r.URL.Path, rec, debug.Stack())
				respondError(w, codeInternalError, "error interno del servidor", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
