package api

import (
	"encoding/json"
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
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "cuerpo de la petición inválido", http.StatusBadRequest)
			return
		}

		doc, err := svc.Create(r.Context(), domain.Document{
			Filename:      req.Filename,
			ExtractedText: req.ExtractedText,
			Checksum:      req.Checksum,
		})
		if err != nil {
			http.Error(w, "no se pudo persistir el documento", http.StatusInternalServerError)
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
