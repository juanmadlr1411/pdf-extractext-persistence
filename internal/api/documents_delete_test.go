package api

import (
	"net/http"
	"testing"
)

// --- Tests de la Issue #9: DELETE /documents/{id} ---

// DELETE sobre un documento existente -> 204 sin cuerpo.
func TestDeleteDocument_Existing_Returns204(t *testing.T) {
	repo := newFakeDocumentRepository()
	repo.byID["507f1f77bcf86cd799439011"] = seedDocument()
	router := newTestRouter(repo)

	rec := doRequest(t, router, http.MethodDelete, "/documents/507f1f77bcf86cd799439011", "")

	if rec.Code != http.StatusNoContent {
		t.Fatalf("esperado status 204, obtenido %d (body: %s)", rec.Code, rec.Body.String())
	}
	if rec.Body.Len() != 0 {
		t.Errorf("el 204 debe ir sin cuerpo, obtenido %q", rec.Body.String())
	}
	if _, ok := repo.byID["507f1f77bcf86cd799439011"]; ok {
		t.Error("el documento no fue eliminado del repositorio")
	}
	if repo.deleteCalls != 1 {
		t.Errorf("esperada 1 llamada a DeleteByID, obtenidas %d", repo.deleteCalls)
	}
}

// Segundo DELETE sobre el mismo id -> 404 NOT_FOUND (ya eliminado).
func TestDeleteDocument_SecondDelete_Returns404(t *testing.T) {
	repo := newFakeDocumentRepository()
	repo.byID["507f1f77bcf86cd799439011"] = seedDocument()
	router := newTestRouter(repo)

	first := doRequest(t, router, http.MethodDelete, "/documents/507f1f77bcf86cd799439011", "")
	if first.Code != http.StatusNoContent {
		t.Fatalf("primer DELETE: esperado 204, obtenido %d", first.Code)
	}

	second := doRequest(t, router, http.MethodDelete, "/documents/507f1f77bcf86cd799439011", "")
	if second.Code != http.StatusNotFound {
		t.Fatalf("segundo DELETE: esperado 404, obtenido %d (body: %s)", second.Code, second.Body.String())
	}
	body := decodeBody(t, second)
	if len(body) != 2 {
		t.Fatalf("envelope de error debe tener exactamente 2 campos, obtenidos %d: %v", len(body), body)
	}
	if body["code"] != "NOT_FOUND" {
		t.Errorf("code esperado %q, obtenido %v", "NOT_FOUND", body["code"])
	}
}

// ID de formato inválido -> 400 fail-fast sin tocar el repositorio.
func TestDeleteDocument_InvalidID_Returns400(t *testing.T) {
	repo := newFakeDocumentRepository()
	router := newTestRouter(repo)

	rec := doRequest(t, router, http.MethodDelete, "/documents/no-es-un-objectid", "")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("esperado status 400, obtenido %d (body: %s)", rec.Code, rec.Body.String())
	}
	body := decodeBody(t, rec)
	if len(body) != 2 {
		t.Fatalf("envelope de error debe tener exactamente 2 campos, obtenidos %d: %v", len(body), body)
	}
	if body["code"] != "INVALID_REQUEST" {
		t.Errorf("code esperado %q, obtenido %v", "INVALID_REQUEST", body["code"])
	}
	if repo.deleteCalls != 0 {
		t.Errorf("fail-fast violado: el repositorio recibió %d llamadas con un ID inválido", repo.deleteCalls)
	}
}
