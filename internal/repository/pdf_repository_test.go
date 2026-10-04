//go:build integration

package repository_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/pdf-extractext/persistence/internal/domain"
	mongoinfra "github.com/pdf-extractext/persistence/internal/infra/mongo"
	"github.com/pdf-extractext/persistence/internal/repository"
)

// Requiere un MongoDB accesible (docker compose up o variable MONGO_TEST_URI).
// Ejecutar con: go test -tags=integration ./internal/repository/ -v

const (
	testDB         = "pdf_extractext_test"
	testCollection = "pdf_documents_test"
)

// setupRepository conecta al MongoDB de pruebas, garantiza el índice único
// de checksum y deja la colección vacía antes de devolver el repositorio.
func setupRepository(t *testing.T) *repository.PDFRepository {
	t.Helper()

	uri := os.Getenv("MONGO_TEST_URI")
	if uri == "" {
		uri = "mongodb://localhost:27017"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	client := mongoinfra.New(uri, testDB, testCollection)
	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Connect falló (¿está MongoDB levantado?): %v", err)
	}
	t.Cleanup(func() {
		if err := client.Disconnect(context.Background()); err != nil {
			t.Logf("Disconnect falló: %v", err)
		}
	})

	if err := client.SetupIndexes(ctx); err != nil {
		t.Fatalf("SetupIndexes falló: %v", err)
	}

	if _, err := client.Collection().DeleteMany(ctx, bson.M{}); err != nil {
		t.Fatalf("no se pudo limpiar la colección: %v", err)
	}

	return repository.NewPDFRepository(client.Collection())
}

func TestPDFRepository_Insert_HappyPath_ReturnsID(t *testing.T) {
	repo := setupRepository(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	doc := domain.PDFDocument{
		Filename:      "factura-001.pdf",
		ExtractedText: "texto extraído de la factura",
		Checksum:      "sha256:aaa111",
	}

	id, err := repo.Insert(ctx, doc)
	if err != nil {
		t.Fatalf("Insert falló: %v", err)
	}

	if len(id) != 24 {
		t.Errorf("ID devuelto = %q (len %d), se esperaba hex de 24 caracteres", id, len(id))
	}
	if _, err := bson.ObjectIDFromHex(id); err != nil {
		t.Errorf("ID devuelto %q no es un ObjectID válido: %v", id, err)
	}
}

func TestPDFRepository_Insert_DuplicateChecksum_ReturnsDomainError(t *testing.T) {
	repo := setupRepository(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	first := domain.PDFDocument{
		Filename:      "original.pdf",
		ExtractedText: "contenido original",
		Checksum:      "sha256:bbb222",
	}
	firstID, err := repo.Insert(ctx, first)
	if err != nil {
		t.Fatalf("Insert del primer documento falló: %v", err)
	}

	duplicate := domain.PDFDocument{
		Filename:      "copia-distinta.pdf", // metadatos distintos
		ExtractedText: "otro texto",
		Checksum:      "sha256:bbb222", // mismo checksum
	}

	if _, err := repo.Insert(ctx, duplicate); err == nil {
		t.Fatal("Insert con checksum duplicado debería fallar")
	} else {
		var dupErr *domain.DuplicateChecksumError
		if !errors.As(err, &dupErr) {
			t.Fatalf("error es de tipo %T, se esperaba *domain.DuplicateChecksumError: %v", err, err)
		}
		if dupErr.Existing.ID != firstID {
			t.Errorf("Existing.ID = %q, want %q", dupErr.Existing.ID, firstID)
		}
		if dupErr.Existing.Filename != first.Filename {
			t.Errorf("Existing.Filename = %q, want %q", dupErr.Existing.Filename, first.Filename)
		}
		if dupErr.Existing.ExtractedText != first.ExtractedText {
			t.Errorf("Existing.ExtractedText = %q, want %q", dupErr.Existing.ExtractedText, first.ExtractedText)
		}
		if dupErr.Existing.Checksum != first.Checksum {
			t.Errorf("Existing.Checksum = %q, want %q", dupErr.Existing.Checksum, first.Checksum)
		}
	}
}
