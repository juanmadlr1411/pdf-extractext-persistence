//go:build integration

package mongo_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/pdf-extractext/persistence/internal/infra/mongo"
)

// Requiere Docker: testcontainers levanta un contenedor de MongoDB por test.

const (
	testDB         = "pdf_extractext_test"
	testCollection = "pdf_documents_test"
)

// mongoImage permite sobreescribir la imagen de test (MONGO_TEST_IMAGE).
// Por defecto se mantiene el pin del equipo: mongo:8. El override es útil
// en CPUs sin AVX, donde MongoDB 5.0+ no arranca.
func mongoImage() string {
	if img := os.Getenv("MONGO_TEST_IMAGE"); img != "" {
		return img
	}
	return "mongo:8"
}

func startMongoContainer(t *testing.T) (*mongodb.MongoDBContainer, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	container, err := mongodb.Run(ctx, mongoImage())
	if err != nil {
		t.Fatalf("no se pudo levantar el contenedor de MongoDB: %v", err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(context.Background()); err != nil {
			t.Logf("error al terminar el contenedor: %v", err)
		}
	})

	uri, err := container.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("no se pudo obtener el connection string: %v", err)
	}
	return container, uri
}

func startMongo(t *testing.T) string {
	t.Helper()
	_, uri := startMongoContainer(t)
	return uri
}

func TestClient_ConnectAndDisconnect(t *testing.T) {
	uri := startMongo(t)
	client := mongo.New(uri, testDB, testCollection)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Connect falló: %v", err)
	}
	if err := client.Disconnect(ctx); err != nil {
		t.Fatalf("Disconnect falló: %v", err)
	}
}

func TestClient_SetupIndexes_CreatesUniqueChecksumIndex(t *testing.T) {
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

	if err := client.SetupIndexes(ctx); err != nil {
		t.Fatalf("SetupIndexes falló: %v", err)
	}

	cursor, err := client.Collection().Indexes().List(ctx)
	if err != nil {
		t.Fatalf("no se pudieron listar los índices: %v", err)
	}
	defer cursor.Close(ctx)

	var found bool
	for cursor.Next(ctx) {
		var idx bson.M
		if err := cursor.Decode(&idx); err != nil {
			t.Fatalf("error decodificando índice: %v", err)
		}
		if idx["name"] == "checksum_1" {
			found = true
			unique, ok := idx["unique"].(bool)
			if !ok || !unique {
				t.Errorf("índice checksum_1 existe pero no es único: %v", idx["unique"])
			}
		}
	}
	if err := cursor.Err(); err != nil {
		t.Fatalf("error iterando índices: %v", err)
	}
	if !found {
		t.Error("índice checksum_1 no encontrado tras SetupIndexes")
	}
}

func TestClient_SetupIndexes_IsIdempotent(t *testing.T) {
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

	for i := 1; i <= 2; i++ {
		if err := client.SetupIndexes(ctx); err != nil {
			t.Fatalf("SetupIndexes falló en la llamada %d: %v", i, err)
		}
	}
}
