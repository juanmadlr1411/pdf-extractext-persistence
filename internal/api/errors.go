package api

import (
	"encoding/json"
	"net/http"
)

// Códigos de error del contrato. Base mínima del catálogo cerrado
// (la Issue #10 lo completará).
const (
	codeInvalidRequest  = "INVALID_REQUEST"
	codeFilenameTooLong = "FILENAME_TOO_LONG"
	codeNotFound        = "NOT_FOUND"
	codeInternalError   = "INTERNAL_ERROR"
)

// errorResponse es el envelope de error del contrato: exactamente {code, message}.
type errorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// writeError serializa el envelope de error con el Content-Type del contrato.
func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorResponse{Code: code, Message: message})
}
