package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"

	"github.com/go-chi/chi/v5"

	"github.com/pdf-extractext/persistence/internal/domain"
	"github.com/pdf-extractext/persistence/internal/service"
)

// objectIDRegex valida el formato de un ObjectId de Mongo (24 hex minúsculas).
var objectIDRegex = regexp.MustCompile(`^[0-9a-f]{24}$`)

// createDocumentRequest es el payload de entrada del endpoint POST /documents.
// La deserialización es estricta: solo se aceptan estos campos.
type createDocumentRequest struct {
	Filename      string `json:"filename"`
	ExtractedText string `json:"extracted_text,omitempty"`
	Checksum      string `json:"checksum"`
}

// updateFilenameRequest es el payload de PATCH /documents/{id}.
// Puntero para distinguir campo ausente/vacío (400) de un valor válido.
// Cualquier campo extra (checksum, extracted_text) se ignora y no muta.
type updateFilenameRequest struct {
	Filename *string `json:"filename"`
}

// documentResponse es el contrato de salida: exactamente estos 4 campos.
type documentResponse struct {
	ID            string `json:"id"`
	Filename      string `json:"filename"`
	ExtractedText string `json:"extracted_text"`
	Checksum      string `json:"checksum"`
}

// toResponse mapea la entidad de dominio al contrato de salida.
func toResponse(doc domain.Document) documentResponse {
	return documentResponse{
		ID:            doc.ID,
		Filename:      doc.Filename,
		ExtractedText: doc.ExtractedText,
		Checksum:      doc.Checksum,
	}
}

// existingToResponse mapea el documento existente del error de checksum
// duplicado (entidad PDFDocument de la issue #3) al contrato de salida.
func existingToResponse(doc domain.PDFDocument) documentResponse {
	return documentResponse{
		ID:            doc.ID,
		Filename:      doc.Filename,
		ExtractedText: doc.ExtractedText,
		Checksum:      doc.Checksum,
	}
}

// writeJSON serializa data con el status dado.
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// createDocumentHandler maneja POST /documents.
func createDocumentHandler(svc *service.DocumentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createDocumentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, "cuerpo de la petición inválido")
			return
		}

		doc, err := svc.Create(r.Context(), domain.Document{
			Filename:      req.Filename,
			ExtractedText: req.ExtractedText,
			Checksum:      req.Checksum,
		})
		if err != nil {
			// Caso estrella del contrato (C2.3): checksum duplicado responde
			// 409 con el envelope de error MÁS el documento existente completo.
			var duplicate *domain.DuplicateChecksumError
			if errors.As(err, &duplicate) {
				status, code, message := translateError(err)
				existing := existingToResponse(duplicate.Existing)
				respondErrorWithDocument(w, code, message, status, &existing)
				return
			}
			// Almacén caído -> 503 DEPENDENCY_UNAVAILABLE; fallo inesperado ->
			// 500 INTERNAL_ERROR. La traducción es por catálogo.
			status, code, message := translateError(err)
			respondError(w, code, message, status)
			return
		}

		writeJSON(w, http.StatusCreated, toResponse(doc))
	}
}

// listDocumentsHandler maneja GET /documents.
// Devuelve la lista completa; sin paginación en la v1.
func listDocumentsHandler(svc *service.DocumentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		docs, err := svc.List(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, codeInternalError, "no se pudieron listar los documentos")
			return
		}

		// Contrato: lista vacía se serializa como [], nunca null.
		resp := make([]documentResponse, 0, len(docs))
		for _, doc := range docs {
			resp = append(resp, toResponse(doc))
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

// getDocumentByIDHandler maneja GET /documents/{id}.
// Valida el formato del ID de forma fail-fast antes de tocar el servicio.
func getDocumentByIDHandler(svc *service.DocumentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		if !objectIDRegex.MatchString(id) {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, "el id debe ser un hexadecimal de 24 caracteres")
			return
		}

		doc, err := svc.GetByID(r.Context(), id)
		if errors.Is(err, service.ErrDocumentNotFound) {
			writeError(w, http.StatusNotFound, codeNotFound, "documento no encontrado")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, codeInternalError, "no se pudo obtener el documento")
			return
		}

		writeJSON(w, http.StatusOK, toResponse(doc))
	}
}

// deleteDocumentHandler maneja DELETE /documents/{id}: 204 al eliminar,
// 404 si ya no existe (idempotencia por ausencia).
func deleteDocumentHandler(svc *service.DocumentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if !objectIDRegex.MatchString(id) {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, "el id debe ser un hexadecimal de 24 caracteres")
			return
		}

		if err := svc.Delete(r.Context(), id); errors.Is(err, service.ErrDocumentNotFound) {
			writeError(w, http.StatusNotFound, codeNotFound, "documento no encontrado")
			return
		} else if err != nil {
			writeError(w, http.StatusInternalServerError, codeInternalError, "no se pudo eliminar el documento")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// patchDocumentHandler maneja PATCH /documents/{id}: solo filename es mutable.
func patchDocumentHandler(svc *service.DocumentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if !objectIDRegex.MatchString(id) {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, "el id debe ser un hexadecimal de 24 caracteres")
			return
		}

		var req updateFilenameRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, "cuerpo de la petición inválido")
			return
		}
		// Un rename sin filename es una petición sin intención válida.
		if req.Filename == nil || *req.Filename == "" {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, "se requiere el campo 'filename'")
			return
		}

		doc, err := svc.UpdateFilename(r.Context(), id, *req.Filename)
		if errors.Is(err, service.ErrFilenameTooLong) {
			writeError(w, http.StatusUnprocessableEntity, codeFilenameTooLong, service.ErrFilenameTooLong.Error())
			return
		}
		if errors.Is(err, service.ErrDocumentNotFound) {
			writeError(w, http.StatusNotFound, codeNotFound, "documento no encontrado")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, codeInternalError, "no se pudo actualizar el documento")
			return
		}

		writeJSON(w, http.StatusOK, toResponse(doc))
	}
}
