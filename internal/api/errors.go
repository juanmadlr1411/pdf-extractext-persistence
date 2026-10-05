package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/pdf-extractext/persistence/internal/domain"
	"github.com/pdf-extractext/persistence/internal/service"
)

// Catálogo cerrado de códigos de error del contrato API.
// Prohibido añadir códigos fuera de esta lista (YAGNI).
const (
	codeInvalidRequest        = "INVALID_REQUEST"         // 400
	codeNotFound              = "NOT_FOUND"               // 404
	codeDuplicateChecksum     = "DUPLICATE_CHECKSUM"      // 409
	codeFilenameTooLong       = "FILENAME_TOO_LONG"       // 422
	codeInternalError         = "INTERNAL_ERROR"          // 500
	codeDependencyUnavailable = "DEPENDENCY_UNAVAILABLE"  // 503
)

// errorResponse es el envelope de error del contrato: exactamente {code, message}.
// Sin "details", sin stacktraces, sin RFC 9457.
type errorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// respondError serializa el envelope de error contractual con el status dado.
func respondError(w http.ResponseWriter, code, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorResponse{Code: code, Message: message})
}

// writeError se mantiene por compatibilidad con handlers existentes.
// Deprecated: usar respondError.
func writeError(w http.ResponseWriter, status int, code, message string) {
	respondError(w, code, message, status)
}

// translateError mapea errores de dominio/servicio a la dupla contractual
// (status, code) más un mensaje seguro para el cliente. Cualquier error no
// clasificado cae al fallback INTERNAL_ERROR (500) sin filtrar detalle interno.
func translateError(err error) (status int, code, message string) {
	var dupErr *domain.DuplicateChecksumError
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return http.StatusNotFound, codeNotFound, "el recurso solicitado no existe"
	case errors.As(err, &dupErr):
		return http.StatusConflict, codeDuplicateChecksum, "ya existe un documento con el mismo checksum"
	case errors.Is(err, service.ErrFilenameTooLong):
		return http.StatusUnprocessableEntity, codeFilenameTooLong, "el filename supera la longitud máxima de 100 caracteres"
	case errors.Is(err, service.ErrDependencyUnavailable):
		return http.StatusServiceUnavailable, codeDependencyUnavailable, "un servicio dependiente no está disponible, inténtelo más tarde"
	default:
		return http.StatusInternalServerError, codeInternalError, "error interno del servidor"
	}
}