//go:build integration

package mongo_test

import (
	"context"
	"testing"
	"time"

	"github.com/pdf-extractext/persistence/internal/domain"
	"github.com/pdf-extractext/persistence/internal/infra/mongo"
	"github.com/pdf-extractext/persistence/internal/service"
)

// Requiere Docker: reutiliza el helper startMongo de client_integration_test.go
// (mismo paquete y build tag), que levanta un contenedor mongo:8 efímero
// y lo baja con t.Cleanup.

// newConnectedRepo levanta Mongo, conecta el cliente y devuelve el repositorio listo.
func newConnectedRepo(t *testing.T) *mongo.DocumentRepository {
	t.Helper()
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

// TestDocumentRepository_SaveAndFindByID simula el flujo end-to-end de la API:
// persiste un documento y lo recupera por su ID contra Mongo real.
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
