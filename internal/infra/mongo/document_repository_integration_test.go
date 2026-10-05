//go:build integration

package mongo_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pdf-extractext/persistence/internal/domain"
	"github.com/pdf-extractext/persistence/internal/infra/mongo"
)

// Requiere Docker: verifica DocumentRepository (la cadena real del POST)
// contra un MongoDB real, incluyendo los contratos 409/503 de la issue #6.

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

	if !errors.Is(err, domain.ErrDependencyUnavailable) {
		t.Fatalf("esperaba ErrDependencyUnavailable con Mongo caído, obtuve %v", err)
	}
}
