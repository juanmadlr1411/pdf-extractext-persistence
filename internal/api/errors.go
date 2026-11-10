package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/pdf-extractext/persistence/internal/domain"
	"github.com/pdf-extractext/persistence/internal/service"
)

// Prohibido añadir códigos fuera de esta lista (YAGNI).
const (
	codeInvalidRequest        = "INVALID_REQUEST"
	codeNotFound              = "NOT_FOUND"
	codeDuplicateChecksum     = "DUPLICATE_CHECKSUM"
	codeFilenameTooLong       = "FILENAME_TOO_LONG"
	codeInternalError         = "INTERNAL_ERROR"
	codeDependencyUnavailable = "DEPENDENCY_UNAVAILABLE"
)

type problemMetadata struct {
	urn   string
	title string
}

var problemCatalog = map[string]problemMetadata{
	codeInvalidRequest:        {"invalid-request", "Petición inválida"},
	codeNotFound:              {"not-found", "Recurso no encontrado"},
	codeDuplicateChecksum:     {"duplicate-checksum", "Checksum duplicado"},
	codeFilenameTooLong:       {"filename-too-long", "Filename demasiado largo"},
	codeInternalError:         {"internal-error", "Error interno del servidor"},
	codeDependencyUnavailable: {"dependency-unavailable", "Dependencia no disponible"},
}

type problemResponse struct {
	Type     string            `json:"type"`
	Title    string            `json:"title"`
	Status   int               `json:"status"`
	Detail   string            `json:"detail"`
	Code     string            `json:"code"`
	Document *documentResponse `json:"document,omitempty"`
}

func respondError(w http.ResponseWriter, code, detail string, status int) {
	respondErrorWithDocument(w, code, detail, status, nil)
}

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

// Cualquier error no clasificado cae al fallback INTERNAL_ERROR (500) sin filtrar detalle interno.
func translateError(err error) (status int, code string, message string) {
	var dupErr *domain.DuplicateChecksumError
	switch {
	case errors.Is(err, service.ErrDocumentNotFound):
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
