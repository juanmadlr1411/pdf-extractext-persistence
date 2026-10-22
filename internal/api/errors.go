package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/pdf-extractext/persistence/internal/domain"
	"github.com/pdf-extractext/persistence/internal/service"
)

// Catálogo cerrado de códigos del contrato (RFC 9457, miembro de extensión).
// Prohibido añadir códigos fuera de esta lista (YAGNI).
const (
	codeInvalidRequest        = "INVALID_REQUEST"        // 400
	codeNotFound              = "NOT_FOUND"              // 404
	codeDuplicateChecksum     = "DUPLICATE_CHECKSUM"     // 409
	codeFilenameTooLong       = "FILENAME_TOO_LONG"      // 422
	codeInternalError         = "INTERNAL_ERROR"         // 500
	codeDependencyUnavailable = "DEPENDENCY_UNAVAILABLE" // 503
)

// problemMeta agrupa el URN y el title de cada code del catálogo.
type problemMeta struct {
	urn   string
	title string
}

// problemCatalog mapea cada code a su (URN, title) según RFC 9457 §3.
var problemCatalog = map[string]problemMeta{
	codeInvalidRequest:        {"invalid-request", "Petición inválida"},
	codeNotFound:              {"not-found", "Recurso no encontrado"},
	codeDuplicateChecksum:     {"duplicate-checksum", "Checksum duplicado"},
	codeFilenameTooLong:       {"filename-too-long", "Filename demasiado largo"},
	codeInternalError:         {"internal-error", "Error interno del servidor"},
	codeDependencyUnavailable: {"dependency-unavailable", "Dependencia no disponible"},
}

// problemResponse es el detalle de error RFC 9457: application/problem+json
// con type (URN), title, status, detail + los miembros de extensión code y
// document (este último únicamente en el 409 DUPLICATE_CHECKSUM).
type problemResponse struct {
	Type     string            `json:"type"`
	Title    string            `json:"title"`
	Status   int               `json:"status"`
	Detail   string            `json:"detail"`
	Code     string            `json:"code"`
	Document *documentResponse `json:"document,omitempty"`
}

// respondError serializa el problema contractual con el status dado.
func respondError(w http.ResponseWriter, code, detail string, status int) {
	respondErrorWithDocument(w, code, detail, status, nil)
}

// respondErrorWithDocument serializa el problema adjuntando, si existe, el
// documento que provoca el conflicto (409 DUPLICATE_CHECKSUM).
func respondErrorWithDocument(w http.ResponseWriter, code, detail string, status int, doc *documentResponse) {
	meta := problemCatalog[code]
	problem := problemResponse{
		Type:     "urn:pdf-extractext:" + meta.urn,
		Title:    meta.title,
		Status:   status,
		Detail:   detail,
		Code:     code,
		Document: doc,
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(problem)
}

// writeError se mantiene por compatibilidad con handlers existentes.
// Deprecated: usar respondError.
func writeError(w http.ResponseWriter, status int, code, message string) {
	respondError(w, code, message, status)
}

// translateError mapea errores de dominio/servicio a la dupla contractual
// (status, code) más un mensaje seguro para el cliente. Cualquier error no
// clasificado cae al fallback INTERNAL_ERROR (500) sin filtrar detalle interno.
func translateError(err error) (status int, code string, message string) {
	var dupErr *domain.DuplicateChecksumError
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return http.StatusNotFound, codeNotFound, "el recurso solicitado no existe"
	case errors.As(err, &dupErr):
		return http.StatusConflict, codeDuplicateChecksum, "ya existe un documento con el mismo checksum"
	case errors.Is(err, service.ErrFilenameTooLong):
		return http.StatusUnprocessableEntity, codeFilenameTooLong, "el filename supera la longitud máxima de 100 caracteres"
	case errors.Is(err, service.ErrMissingRequiredField):
		return http.StatusBadRequest, codeInvalidRequest, "faltan campos obligatorios del documento"
	case errors.Is(err, service.ErrDependencyUnavailable):
		return http.StatusServiceUnavailable, codeDependencyUnavailable, "un servicio dependiente no está disponible, inténtelo más tarde"
	default:
		return http.StatusInternalServerError, codeInternalError, "error interno del servidor"
	}
}
