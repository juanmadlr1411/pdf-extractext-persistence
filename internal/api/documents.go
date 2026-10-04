package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/pdf-extractext/persistence/internal/domain"
	"github.com/pdf-extractext/persistence/internal/service"
)

// createDocumentRequest es el payload de entrada del endpoint POST /documents.
// La deserialización es estricta: solo se aceptan estos campos.
type createDocumentRequest struct {
	Filename      string `json:"filename"`
	ExtractedText string `json:"extracted_text,omitempty"`
	Checksum      string `json:"checksum"`
}

// documentResponse es el contrato de salida: exactamente estos 4 campos.
type documentResponse struct {
	ID            string `json:"id"`
	Filename      string `json:"filename"`
	ExtractedText string `json:"extracted_text"`
	Checksum      string `json:"checksum"`
}

// createDocumentHandler maneja POST /documents.
func createDocumentHandler(svc *service.DocumentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createDocumentRequest
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, "cuerpo de la petición inválido o malformado")
			return
		}

		if req.Filename == "" || req.Checksum == "" {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, "los campos filename y checksum son obligatorios")
			return
		}

		doc, err := svc.Create(r.Context(), domain.Document{
			Filename:      req.Filename,
			ExtractedText: req.ExtractedText,
			Checksum:      req.Checksum,
		})
		if err != nil {
			if errors.Is(err, service.ErrFilenameTooLong) {
				writeError(w, http.StatusUnprocessableEntity, codeFilenameTooLong,
					"el filename supera la longitud máxima de 100 caracteres")
				return
			}
			writeError(w, http.StatusInternalServerError, codeInvalidRequest, "no se pudo persistir el documento")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(documentResponse{
			ID:            doc.ID,
			Filename:      doc.Filename,
			ExtractedText: doc.ExtractedText,
			Checksum:      doc.Checksum,
		})
	}
}
