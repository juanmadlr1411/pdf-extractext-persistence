//go:build integration

package mongo_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pdf-extractext/persistence/internal/domain"
	"github.com/pdf-extractext/persistence/internal/infra/mongo"
	"github.com/pdf-extractext/persistence/internal/service"
)

// Requiere Docker: verifica DocumentRepository contra un MongoDB real
// (testcontainers), cubriendo la cadena real del POST, los contratos
// 409/503 (checksum duplicado con documento existente completo y caída
// de la dependencia en vivo) y el probe de readiness.

// connectRepository conecta el cliente, garantiza el índice único de checksum
// y registra su desconexión al finalizar el test.
func connectRepository(t *testing.T, client *mongo.Client) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Connect falló: %v", err)
	}
	if err := client.SetupIndexes(ctx); err != nil {
		t.Fatalf("SetupIndexes falló: %v", err)
	}
	t.Cleanup(func() {
		if err := client.Disconnect(context.Background()); err != nil {
			t.Logf("error al desconectar: %v", err)
		}
	})
}

// newConnectedRepo levanta Mongo, conecta el cliente y devuelve el repositorio listo.
func newConnectedRepo(t *testing.T) *mongo.DocumentRepository {
	t.Helper()
	client := mongo.New(startMongo(t), testDB, testCollection)
	connectRepository(t, client)
	return mongo.NewDocumentRepository(client)
}

// TestDocumentRepository_Check_PingsRealMongo verifica que el probe de
// readiness (HealthChecker) responde correctamente contra Mongo real.
func TestDocumentRepository_Check_PingsRealMongo(t *testing.T) {
	uri := startMongo(t)
	client := mongo.New(uri, testDB, testCollection)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Connect falló: %v", err)
	}
	t.Cleanup(func() {
		if err := client.Disconnect(context.Background()); err != nil {
			t.Logf("Disconnect falló: %v", err)
		}
	})

	pingCtx, pingCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer pingCancel()

	if err := client.Check(pingCtx); err != nil {
		t.Fatalf("Check (ping) debería responder OK contra Mongo real: %v", err)
	}
}

// TestDocumentRepository_Save_PersisteYDevuelveDocumentoConID simula el flujo
// end-to-end del POST: persiste un documento con sus 4 campos contra Mongo real.
func TestDocumentRepository_Save_PersisteYDevuelveDocumentoConID(t *testing.T) {
	client := mongo.New(startMongo(t), testDB, testCollection)
	connectRepository(t, client)
	repo := mongo.NewDocumentRepository(client)

	saved, err := repo.Save(context.Background(), domain.Document{
		Filename:      "acta.pdf",
		ExtractedText: "texto extraído del acta",
		Checksum:      "sha256:abc123",
	})
	if err != nil {
		t.Fatalf("Save falló: %v", err)
	}
	if len(saved.ID) != 24 {
		t.Errorf("ID devuelto %q (len %d), se esperaba hex de 24 caracteres", saved.ID, len(saved.ID))
	}
	if saved.Filename != "acta.pdf" || saved.ExtractedText != "texto extraído del acta" || saved.Checksum != "sha256:abc123" {
		t.Errorf("Save debe preservar los campos del documento: obtenido %+v", saved)
	}
}

// TestDocumentRepository_SaveAndFindByID verifica el ciclo completo:
// persistir y recuperar por ID contra Mongo real.
func TestDocumentRepository_SaveAndFindByID(t *testing.T) {
	repo := newConnectedRepo(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	want := domain.Document{
		Filename:      "informe.pdf",
		ExtractedText: "texto extraído",
		Checksum:      "abc123-integration",
	}

	saved, err := repo.Save(ctx, want)
	if err != nil {
		t.Fatalf("Save falló: %v", err)
	}
	if saved.ID == "" {
		t.Fatal("Save debería devolver el documento con ID asignado por Mongo")
	}

	got, err := repo.FindByID(ctx, saved.ID)
	if err != nil {
		t.Fatalf("FindByID falló: %v", err)
	}
	if got.ID != saved.ID {
		t.Errorf("id esperado %q, obtenido %q", saved.ID, got.ID)
	}
	if got.Filename != want.Filename {
		t.Errorf("filename esperado %q, obtenido %q", want.Filename, got.Filename)
	}
	if got.ExtractedText != want.ExtractedText {
		t.Errorf("extracted_text esperado %q, obtenido %q", want.ExtractedText, got.ExtractedText)
	}
	if got.Checksum != want.Checksum {
		t.Errorf("checksum esperado %q, obtenido %q", want.Checksum, got.Checksum)
	}
}

// TestDocumentRepository_FindAll_ReturnsSavedDocuments verifica la lectura
// de la lista completa contra Mongo real.
func TestDocumentRepository_FindAll_ReturnsSavedDocuments(t *testing.T) {
	repo := newConnectedRepo(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, checksum := range []string{"sum-int-1", "sum-int-2"} {
		_, err := repo.Save(ctx, domain.Document{
			Filename:      "doc-" + checksum + ".pdf",
			ExtractedText: "texto",
			Checksum:      checksum,
		})
		if err != nil {
			t.Fatalf("Save falló para %q: %v", checksum, err)
		}
	}

	docs, err := repo.FindAll(ctx)
	if err != nil {
		t.Fatalf("FindAll falló: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("esperados 2 documentos, obtenidos %d: %v", len(docs), docs)
	}
}

// TestDocumentRepository_FindByID_NotFound verifica que el repositorio mapea
// correctamente el caso "no existe" al error centinela del dominio.
func TestDocumentRepository_FindByID_NotFound(t *testing.T) {
	repo := newConnectedRepo(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := repo.FindByID(ctx, "507f1f77bcf86cd7994390ff")
	if err != service.ErrDocumentNotFound {
		t.Fatalf("esperado ErrDocumentNotFound, obtenido: %v", err)
	}
}

// TestDocumentRepository_Save_ChecksumDuplicadoDevuelveDocumentoExistente
// verifica el contrato 409: clave duplicada -> DuplicateChecksumError con
// el documento existente completo.
func TestDocumentRepository_Save_ChecksumDuplicadoDevuelveDocumentoExistente(t *testing.T) {
	client := mongo.New(startMongo(t), testDB, testCollection)
	connectRepository(t, client)
	repo := mongo.NewDocumentRepository(client)

	first, err := repo.Save(context.Background(), domain.Document{
		Filename:      "original.pdf",
		ExtractedText: "texto original",
		Checksum:      "sha256:duplicado",
	})
	if err != nil {
		t.Fatalf("primer Save falló: %v", err)
	}

	_, err = repo.Save(context.Background(), domain.Document{
		Filename:      "copia.pdf",
		ExtractedText: "otro texto",
		Checksum:      "sha256:duplicado",
	})

	var dup *domain.DuplicateChecksumError
	if !errors.As(err, &dup) {
		t.Fatalf("esperaba *domain.DuplicateChecksumError, obtuve %v", err)
	}
	// El documento existente llega completo: los 4 campos del conflicto.
	if dup.Existing.ID != first.ID {
		t.Errorf("Existing.ID = %q, want %q", dup.Existing.ID, first.ID)
	}
	if dup.Existing.Filename != first.Filename {
		t.Errorf("Existing.Filename = %q, want %q", dup.Existing.Filename, first.Filename)
	}
	if dup.Existing.ExtractedText != first.ExtractedText {
		t.Errorf("Existing.ExtractedText = %q, want %q", dup.Existing.ExtractedText, first.ExtractedText)
	}
	if dup.Existing.Checksum != first.Checksum {
		t.Errorf("Existing.Checksum = %q, want %q", dup.Existing.Checksum, first.Checksum)
	}
}

// TestDocumentRepository_Save_MongoCaidoDevuelveIndisponibilidad verifica el
// contrato 503: con la dependencia caída, Save devuelve ErrDependencyUnavailable.
func TestDocumentRepository_Save_MongoCaidoDevuelveIndisponibilidad(t *testing.T) {
	container, uri := startMongoContainer(t)
	client := mongo.New(uri, testDB, testCollection)
	connectRepository(t, client)
	repo := mongo.NewDocumentRepository(client)

	// Caemos la dependencia: el contenedor desaparece bajo los pies de un
	// cliente que ya estaba conectado y operativo.
	termCtx, termCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer termCancel()
	if err := container.Terminate(termCtx); err != nil {
		t.Fatalf("no se pudo terminar el contenedor: %v", err)
	}

	saveCtx, saveCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer saveCancel()
	_, err := repo.Save(saveCtx, domain.Document{
		Filename:      "acta.pdf",
		ExtractedText: "texto",
		Checksum:      "sha256:mongo-caido",
	})

	if !errors.Is(err, service.ErrDependencyUnavailable) {
		t.Fatalf("esperaba ErrDependencyUnavailable con Mongo caído, obtuve %v", err)
	}
}
