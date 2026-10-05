package api

import (
	"encoding/json"
	"net/http"
)

// Códigos de error del contrato. Base mínima del catálogo cerrado
// (la Issue #10 lo completará).
const (
	codeInvalidRequest        = "INVALID_REQUEST"
	codeFilenameTooLong       = "FILENAME_TOO_LONG"
	codeNotFound              = "NOT_FOUND"
	codeInternalError         = "INTERNAL_ERROR"
	codeDependencyUnavailable = "DEPENDENCY_UNAVAILABLE"
	codeDuplicateChecksum     = "DUPLICATE_CHECKSUM"
)

// errorResponse es el envelope de error del contrato: exactamente {code, message}.
// document se adjunta únicamente en el 409 DUPLICATE_CHECKSUM para devolver
// el documento existente que provoca el conflicto (issue #6).
type errorResponse struct {
	Code     string            `json:"code"`
	Message  string            `json:"message"`
	Document *documentResponse `json:"document,omitempty"`
}

// writeError serializa el envelope de error con el Content-Type del contrato.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeErrorWithDocument(w, status, code, message, nil)
}

// writeErrorWithDocument serializa el envelope de error adjuntando, si existe,
// el documento que provoca el conflicto (409 DUPLICATE_CHECKSUM).
func writeErrorWithDocument(w http.ResponseWriter, status int, code, message string, doc *documentResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorResponse{Code: code, Message: message, Document: doc})
}
