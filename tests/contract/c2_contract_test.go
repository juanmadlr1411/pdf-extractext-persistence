// Tabla del contrato C2 (docs/specs/persistence-service.md) verificada por
// esta suite. Una fila de la tabla = un test; los fallos citan la regla.
//
//	| Fila  | Método y escenario                            | Status | Cuerpo esperado                                              |
//	|-------|-----------------------------------------------|--------|--------------------------------------------------------------|
//	| C2.1  | POST /documents exitoso                       | 201    | documento exacto {id hex-24, filename, extracted_text, checksum} |
//	| C2.2  | cuerpos/IDs inválidos                         | 400    | envelope estricto {code: INVALID_REQUEST, message}           |
//	| C2.3  | POST con checksum ya persistido (caso estrella)| 409   | envelope {code: DUPLICATE_CHECKSUM, message} + document completo |
//	| C2.4  | dependencia (MongoDB) caída                   | 503    | envelope estricto {code: DEPENDENCY_UNAVAILABLE, message}    |
//	| C2.5  | recurso inexistente / DELETE repetido         | 404    | envelope estricto {code: NOT_FOUND, message}                  |
//	| C2.6  | PATCH con filename > 100 caracteres           | 422    | envelope estricto {code: FILENAME_TOO_LONG, message}         |
//	| C2.7  | GET /documents/{id} existente                 | 200    | documento exacto (4 campos)                                  |
//	| C2.8  | GET /documents (listado)                      | 200    | array JSON; vacío se serializa [] (nunca null)               |
//	| C2.9  | PATCH rename / mismo valor                    | 200    | documento con filename mutado; idempotente                   |
//	| C2.10 | DELETE /documents/{id} existente              | 204    | sin cuerpo                                                   |
//	| C2.11 | trazabilidad                                  | —      | X-Request-ID propagado o generado (UUID v4)                  |
package contract

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/pdf-extractext/persistence/internal/domain"
)

const (
	// validObjectID es un id válido del contrato: hex de 24 minúsculas.
	validObjectID = "507f1f77bcf86cd799439011"
	// missingObjectID tiene formato válido pero no existe en el almacén.
	missingObjectID = "507f1f77bcf86cd7994390ff"

	createPayload = `{"filename":"informe.pdf","extracted_text":"texto extraído","checksum":"sha256:abc123"}`
)

// seededDocument devuelve un documento preexistente típico para disponer
// estado inicial en el almacén.
func seededDocument() domain.Document {
	return domain.Document{
		ID:            validObjectID,
		Filename:      "informe.pdf",
		ExtractedText: "texto extraído",
		Checksum:      "sha256:abc123",
	}
}

// --- Fila C2.1: Happy Path (201) ---

// TestC2_01_CreateDocument_HappyPath verifica la creación exitosa: 201 con
// la serialización exacta del cuerpo de respuesta (4 campos) y el id
// generado cumpliendo la regla de dominio del formato (hex de 24).
func TestC2_01_CreateDocument_HappyPath(t *testing.T) {
	t.Parallel()
	const rule = "C2.1 · POST /documents exitoso → 201"

	store := newContractStore()
	srv := newContractServer(t, store, stubHealthChecker{})

	res := doRequest(t, srv, http.MethodPost, "/documents", createPayload, nil)

	assertStatus(t, rule, res.status, http.StatusCreated)
	assertJSONContentType(t, rule, res)

	body := decodeJSONObject(t, rule, res.body)
	if len(body) != 4 {
		t.Errorf("%s: REGLA VIOLADA — la respuesta 201 serializa exactamente los 4 campos del contrato {id, filename, extracted_text, checksum}; obtenidos %d: %v", rule, len(body), body)
	}
	assertField(t, rule, "filename", body, "informe.pdf")
	assertField(t, rule, "extracted_text", body, "texto extraído")
	assertField(t, rule, "checksum", body, "sha256:abc123")

	id, ok := body["id"].(string)
	if !ok {
		t.Fatalf("%s: REGLA VIOLADA — el campo 'id' debe ser un string; obtenido %v", rule, body["id"])
	}
	assertObjectIDFormat(t, rule, id)
}

// --- Fila C2.2: Errores estándar 400 INVALID_REQUEST ---

// TestC2_02_InvalidRequest_400 verifica cada variante de petición inválida:
// JSON malformado, cuerpo vacío, filename ausente en PATCH e IDs de recurso
// que no cumplen el formato hex-24. Todas responden 400 con el code
// INVALID_REQUEST del catálogo y el envelope estricto {code, message}.
func TestC2_02_InvalidRequest_400(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"POST con JSON malformado", http.MethodPost, "/documents", `{"filename":`},
		{"POST con cuerpo vacío", http.MethodPost, "/documents", ``},
		{"PATCH sin campo filename", http.MethodPatch, "/documents/" + validObjectID, `{}`},
		{"GET con id de formato inválido", http.MethodGet, "/documents/no-es-un-objectid", ""},
		{"PATCH con id de formato inválido", http.MethodPatch, "/documents/507f1f77bcf86cd7994390", `{"filename":"x.pdf"}`},
		{"DELETE con id de formato inválido", http.MethodDelete, "/documents/507F1F77BCF86CD799439011", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rule := "C2.2 · " + tc.name + " → 400 INVALID_REQUEST"

			store := newContractStore()
			srv := newContractServer(t, store, stubHealthChecker{})

			res := doRequest(t, srv, tc.method, tc.path, tc.body, nil)

			assertStatus(t, rule, res.status, http.StatusBadRequest)
			assertErrorEnvelope(t, rule, res.body, "INVALID_REQUEST")
		})
	}
}

// --- Fila C2.3: Caso estrella 409 DUPLICATE_CHECKSUM ---

// TestC2_03_DuplicateChecksum_409 verifica el conflicto: POST con un
// checksum ya persistido responde 409 con el envelope de error MÁS el
// documento existente completo en la clave 'document' (exactamente 3
// claves {code, message, document}). El conflicto se detecta en el único
// intento de inserción (anti-TOCTOU observable).
func TestC2_03_DuplicateChecksum_409(t *testing.T) {
	t.Parallel()
	const rule = "C2.3 · POST con checksum duplicado → 409 DUPLICATE_CHECKSUM + documento existente"

	store := newContractStore()
	existing := domain.Document{
		ID:            validObjectID,
		Filename:      "original.pdf",
		ExtractedText: "texto previamente extraído",
		Checksum:      "sha256:conflicto",
	}
	store.seed(existing)
	srv := newContractServer(t, store, stubHealthChecker{})

	res := doRequest(t, srv, http.MethodPost, "/documents",
		`{"filename":"copia.pdf","extracted_text":"otro texto","checksum":"sha256:conflicto"}`, nil)

	assertStatus(t, rule, res.status, http.StatusConflict)
	assertJSONContentType(t, rule, res)

	body := decodeJSONObject(t, rule, res.body)
	if len(body) != 3 {
		t.Fatalf("%s: REGLA VIOLADA — el 409 viaja con el envelope de error MÁS el documento existente: exactamente 3 claves {code, message, document}; obtenidas %d: %v", rule, len(body), body)
	}
	assertField(t, rule, "code", body, "DUPLICATE_CHECKSUM")
	if msg, ok := body["message"].(string); !ok || msg == "" {
		t.Errorf("%s: REGLA VIOLADA — message debe ser un string no vacío; obtenido %v", rule, body["message"])
	}

	doc, ok := body["document"].(map[string]any)
	if !ok {
		t.Fatalf("%s: REGLA VIOLADA — 'document' debe ser el objeto del documento existente; obtenido %v", rule, body["document"])
	}
	assertContractDocument(t, rule, doc, existing)

	if store.saveCalls != 1 {
		t.Errorf("%s: REGLA VIOLADA — el conflicto se detecta en el único intento de inserción (anti-TOCTOU); Save invocado %d veces", rule, store.saveCalls)
	}
}

// --- Fila C2.4: 503 DEPENDENCY_UNAVAILABLE ---

// TestC2_04_DependencyUnavailable_503 verifica la indisponibilidad de la
// dependencia crítica: tanto la escritura contra el almacén caído como el
// readiness check responden 503 con el code DEPENDENCY_UNAVAILABLE y el
// envelope estricto de 2 claves (sin document ni campos extra).
func TestC2_04_DependencyUnavailable_503(t *testing.T) {
	t.Parallel()

	t.Run("POST con almacén caído", func(t *testing.T) {
		t.Parallel()
		const rule = "C2.4 · POST con dependencia caída → 503 DEPENDENCY_UNAVAILABLE"

		store := newContractStore()
		store.unavailable = true
		srv := newContractServer(t, store, stubHealthChecker{})

		res := doRequest(t, srv, http.MethodPost, "/documents", createPayload, nil)

		assertStatus(t, rule, res.status, http.StatusServiceUnavailable)
		assertErrorEnvelope(t, rule, res.body, "DEPENDENCY_UNAVAILABLE")
	})

	t.Run("readiness con dependencia caída", func(t *testing.T) {
		t.Parallel()
		const rule = "C2.4 · GET /health/ready con dependencia caída → 503 DEPENDENCY_UNAVAILABLE"

		store := newContractStore()
		srv := newContractServer(t, store, stubHealthChecker{err: errDependencyDown})

		res := doRequest(t, srv, http.MethodGet, "/health/ready", "", nil)

		assertStatus(t, rule, res.status, http.StatusServiceUnavailable)
		assertErrorEnvelope(t, rule, res.body, "DEPENDENCY_UNAVAILABLE")
	})
}

// errDependencyDown simula el fallo del health check de la dependencia.
var errDependencyDown = errors.New("dependencia crítica caída")

// --- Fila C2.5: 404 NOT_FOUND ---

// TestC2_05_NotFound_404 verifica la ausencia de recurso: GET y PATCH sobre
// un id válido pero inexistente, y la semántica del DELETE repetido (lo que
// ya no existe devuelve 404). Todas con el code NOT_FOUND del catálogo.
func TestC2_05_NotFound_404(t *testing.T) {
	t.Parallel()

	t.Run("GET inexistente", func(t *testing.T) {
		t.Parallel()
		const rule = "C2.5 · GET /documents/{id} inexistente → 404 NOT_FOUND"
		srv := newContractServer(t, newContractStore(), stubHealthChecker{})

		res := doRequest(t, srv, http.MethodGet, "/documents/"+missingObjectID, "", nil)

		assertStatus(t, rule, res.status, http.StatusNotFound)
		assertErrorEnvelope(t, rule, res.body, "NOT_FOUND")
	})

	t.Run("PATCH inexistente", func(t *testing.T) {
		t.Parallel()
		const rule = "C2.5 · PATCH /documents/{id} inexistente → 404 NOT_FOUND"
		srv := newContractServer(t, newContractStore(), stubHealthChecker{})

		res := doRequest(t, srv, http.MethodPatch, "/documents/"+missingObjectID, `{"filename":"nuevo.pdf"}`, nil)

		assertStatus(t, rule, res.status, http.StatusNotFound)
		assertErrorEnvelope(t, rule, res.body, "NOT_FOUND")
	})

	t.Run("DELETE repetido (idempotencia por ausencia)", func(t *testing.T) {
		t.Parallel()
		const rule = "C2.5 · DELETE /documents/{id} repetido → 404 NOT_FOUND"
		store := newContractStore()
		store.seed(seededDocument())
		srv := newContractServer(t, store, stubHealthChecker{})

		first := doRequest(t, srv, http.MethodDelete, "/documents/"+validObjectID, "", nil)
		assertStatus(t, rule, first.status, http.StatusNoContent)

		second := doRequest(t, srv, http.MethodDelete, "/documents/"+validObjectID, "", nil)
		assertStatus(t, rule, second.status, http.StatusNotFound)
		assertErrorEnvelope(t, rule, second.body, "NOT_FOUND")
	})
}

// --- Fila C2.6: 422 FILENAME_TOO_LONG ---

// TestC2_06_FilenameTooLong_422 verifica el invariante de dominio de la
// longitud del filename en la mutación: más de 100 caracteres responde 422
// con el code FILENAME_TOO_LONG del catálogo y envelope estricto.
func TestC2_06_FilenameTooLong_422(t *testing.T) {
	t.Parallel()
	const rule = "C2.6 · PATCH con filename de 101 caracteres → 422 FILENAME_TOO_LONG"

	store := newContractStore()
	store.seed(seededDocument())
	srv := newContractServer(t, store, stubHealthChecker{})

	tooLong := strings.Repeat("a", 101)
	res := doRequest(t, srv, http.MethodPatch, "/documents/"+validObjectID,
		`{"filename":"`+tooLong+`"}`, nil)

	assertStatus(t, rule, res.status, http.StatusUnprocessableEntity)
	assertErrorEnvelope(t, rule, res.body, "FILENAME_TOO_LONG")
}

// --- Fila C2.7: GET por id, documento exacto ---

// TestC2_07_GetDocument_200 verifica la lectura de un documento existente:
// 200 con la serialización exacta de los 4 campos del contrato.
func TestC2_07_GetDocument_200(t *testing.T) {
	t.Parallel()
	const rule = "C2.7 · GET /documents/{id} existente → 200 con documento exacto"

	store := newContractStore()
	want := seededDocument()
	store.seed(want)
	srv := newContractServer(t, store, stubHealthChecker{})

	res := doRequest(t, srv, http.MethodGet, "/documents/"+validObjectID, "", nil)

	assertStatus(t, rule, res.status, http.StatusOK)
	assertJSONContentType(t, rule, res)
	assertContractDocument(t, rule, decodeJSONObject(t, rule, res.body), want)
}

// --- Fila C2.8: Listado ---

// TestC2_08_ListDocuments_200 verifica el listado: 200 con array JSON donde
// la lista vacía se serializa como [] (nunca null) y cada documento serializa
// exactamente los 4 campos del contrato.
func TestC2_08_ListDocuments_200(t *testing.T) {
	t.Parallel()

	t.Run("almacén vacío → [] nunca null", func(t *testing.T) {
		t.Parallel()
		const rule = "C2.8 · GET /documents con almacén vacío → 200 con []"

		srv := newContractServer(t, newContractStore(), stubHealthChecker{})

		res := doRequest(t, srv, http.MethodGet, "/documents", "", nil)

		assertStatus(t, rule, res.status, http.StatusOK)
		assertJSONContentType(t, rule, res)
		if strings.TrimSpace(res.body) == "null" {
			t.Fatalf("%s: REGLA VIOLADA — la lista vacía no debe serializarse como null", rule)
		}
		var list []map[string]any
		if err := jsonDecodeArray(res.body, &list); err != nil {
			t.Fatalf("%s: REGLA VIOLADA — el cuerpo debe ser un array JSON: %v (body: %s)", rule, err, res.body)
		}
		if len(list) != 0 {
			t.Errorf("%s: REGLA VIOLADA — el almacén vacío devuelve un array vacío; obtenidos %d elementos: %v", rule, len(list), list)
		}
	})

	t.Run("con documentos → N documentos de 4 campos", func(t *testing.T) {
		t.Parallel()
		const rule = "C2.8 · GET /documents con N documentos → 200 con N documentos"

		store := newContractStore()
		first := seededDocument()
		second := domain.Document{
			ID:            "507f1f77bcf86cd799439022",
			Filename:      "acta.pdf",
			ExtractedText: "texto del acta",
			Checksum:      "sha256:otro",
		}
		store.seed(first)
		store.seed(second)
		srv := newContractServer(t, store, stubHealthChecker{})

		res := doRequest(t, srv, http.MethodGet, "/documents", "", nil)

		assertStatus(t, rule, res.status, http.StatusOK)

		var list []map[string]any
		if err := jsonDecodeArray(res.body, &list); err != nil {
			t.Fatalf("%s: REGLA VIOLADA — el cuerpo debe ser un array JSON: %v", rule, err)
		}
		if len(list) != 2 {
			t.Fatalf("%s: REGLA VIOLADA — el listado devuelve todos los documentos; esperados 2, obtenidos %d: %v", rule, len(list), list)
		}

		byID := map[string]map[string]any{first.ID: nil, second.ID: nil}
		for _, item := range list {
			id, _ := item["id"].(string)
			if _, known := byID[id]; !known {
				t.Errorf("%s: REGLA VIOLADA — el listado contiene un documento desconocido id=%v", rule, item["id"])
				continue
			}
			byID[id] = item
		}
		assertContractDocument(t, rule, byID[first.ID], first)
		assertContractDocument(t, rule, byID[second.ID], second)
	})
}

// --- Fila C2.9: PATCH — mutación controlada e idempotencia ---

// TestC2_09_PatchDocument_200 verifica las reglas de mutación: el rename
// solo muta filename (extracted_text y checksum intactos, mismo id) y un
// PATCH al mismo valor es idempotente (200 con el documento sin cambios).
func TestC2_09_PatchDocument_200(t *testing.T) {
	t.Parallel()

	t.Run("rename muta solo filename", func(t *testing.T) {
		t.Parallel()
		const rule = "C2.9 · PATCH rename → 200 mutando únicamente filename"

		store := newContractStore()
		store.seed(seededDocument())
		srv := newContractServer(t, store, stubHealthChecker{})

		res := doRequest(t, srv, http.MethodPatch, "/documents/"+validObjectID,
			`{"filename":"renombrado.pdf"}`, nil)

		assertStatus(t, rule, res.status, http.StatusOK)
		assertJSONContentType(t, rule, res)

		want := seededDocument()
		want.Filename = "renombrado.pdf"
		assertContractDocument(t, rule, decodeJSONObject(t, rule, res.body), want)
	})

	t.Run("PATCH al mismo valor es idempotente", func(t *testing.T) {
		t.Parallel()
		const rule = "C2.9 · PATCH al mismo valor → 200 con el documento idéntico"

		store := newContractStore()
		want := seededDocument()
		store.seed(want)
		srv := newContractServer(t, store, stubHealthChecker{})

		res := doRequest(t, srv, http.MethodPatch, "/documents/"+validObjectID,
			`{"filename":"informe.pdf"}`, nil)

		assertStatus(t, rule, res.status, http.StatusOK)
		assertContractDocument(t, rule, decodeJSONObject(t, rule, res.body), want)
	})
}

// --- Fila C2.10: DELETE 204 ---

// TestC2_10_DeleteDocument_204 verifica la eliminación exitosa: 204 y sin
// cuerpo (el contrato no define cuerpo para el 204).
func TestC2_10_DeleteDocument_204(t *testing.T) {
	t.Parallel()
	const rule = "C2.10 · DELETE /documents/{id} existente → 204 sin cuerpo"

	store := newContractStore()
	store.seed(seededDocument())
	srv := newContractServer(t, store, stubHealthChecker{})

	res := doRequest(t, srv, http.MethodDelete, "/documents/"+validObjectID, "", nil)

	assertStatus(t, rule, res.status, http.StatusNoContent)
	assertNoBody(t, rule, res.body)
}

// --- Fila C2.11: Trazabilidad X-Request-ID ---

// TestC2_11_XRequestID verifica la propagación de headers de correlación:
// el X-Request-ID entrante se devuelve exactamente igual; sin header
// entrante el servicio genera un UUID v4; y el header está presente
// también en las respuestas de error.
func TestC2_11_XRequestID(t *testing.T) {
	t.Parallel()

	t.Run("propaga el header entrante", func(t *testing.T) {
		t.Parallel()
		const rule = "C2.11 · X-Request-ID entrante se propaga exactamente en la respuesta"
		const incoming = "c2-trace-id-entrada-123"

		srv := newContractServer(t, newContractStore(), stubHealthChecker{})

		res := doRequest(t, srv, http.MethodPost, "/documents", createPayload,
			map[string]string{"X-Request-ID": incoming})

		assertStatus(t, rule, res.status, http.StatusCreated)
		if got := res.header.Get("X-Request-ID"); got != incoming {
			t.Errorf("%s: REGLA VIOLADA — el header de respuesta = %q, debe propagar exactamente %q", rule, got, incoming)
		}
	})

	t.Run("genera UUID v4 si el header no llega", func(t *testing.T) {
		t.Parallel()
		const rule = "C2.11 · sin X-Request-ID entrante el servicio genera un UUID v4"

		srv := newContractServer(t, newContractStore(), stubHealthChecker{})

		res := doRequest(t, srv, http.MethodGet, "/health", "", nil)

		assertStatus(t, rule, res.status, http.StatusOK)
		generated := res.header.Get("X-Request-ID")
		if generated == "" {
			t.Fatalf("%s: REGLA VIOLADA — toda respuesta debe llevar X-Request-ID", rule)
		}
		parsed, err := uuid.Parse(generated)
		if err != nil {
			t.Fatalf("%s: REGLA VIOLADA — el ID generado (%q) debe ser un UUID válido: %v", rule, generated, err)
		}
		if v := parsed.Version(); v != 4 {
			t.Errorf("%s: REGLA VIOLADA — el ID generado (%q) debe ser UUID v4; versión %d", rule, generated, v)
		}
	})

	t.Run("presente también en respuestas de error", func(t *testing.T) {
		t.Parallel()
		const rule = "C2.11 · las respuestas de error también llevan X-Request-ID"

		srv := newContractServer(t, newContractStore(), stubHealthChecker{})

		res := doRequest(t, srv, http.MethodGet, "/documents/"+missingObjectID, "", nil)

		assertStatus(t, rule, res.status, http.StatusNotFound)
		if res.header.Get("X-Request-ID") == "" {
			t.Errorf("%s: REGLA VIOLADA — la respuesta de error debe incluir el header de correlación", rule)
		}
	})
}
