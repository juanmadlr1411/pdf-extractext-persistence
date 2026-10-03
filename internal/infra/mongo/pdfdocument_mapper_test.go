package mongo_test

import (
	"testing"

	"github.com/pdf-extractext/persistence/internal/domain"
	"github.com/pdf-extractext/persistence/internal/infra/mongo"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestMapper_RoundTrip_Idempotent(t *testing.T) {
	oid := bson.NewObjectID()
	original := domain.PDFDocument{
		ID:            oid.Hex(),
		Filename:      "factura.pdf",
		ExtractedText: "contenido extraído",
		Checksum:      "sha256:deadbeef",
	}

	if len(original.ID) != 24 {
		t.Fatalf("precondición: ID debe tener 24 caracteres hex, tiene %d", len(original.ID))
	}

	mongoDoc, err := mongo.DomainToMongo(original)
	if err != nil {
		t.Fatalf("DomainToMongo() err inesperado: %v", err)
	}
	if mongoDoc.ID != oid {
		t.Errorf("DomainToMongo: _id = %v, want %v", mongoDoc.ID, oid)
	}
	if mongoDoc.Filename != original.Filename ||
		mongoDoc.Checksum != original.Checksum ||
		mongoDoc.ExtractedText != original.ExtractedText {
		t.Error("DomainToMongo perdió datos de campos escalares")
	}

	back, err := mongo.MongoToDomain(mongoDoc)
	if err != nil {
		t.Fatalf("MongoToDomain() err inesperado: %v", err)
	}
	if back != original {
		t.Errorf("round trip no idempotente: %+v, want %+v", back, original)
	}
}

func TestDomainToMongo_InvalidID_ReturnsError(t *testing.T) {
	doc := domain.PDFDocument{ID: "no-es-hex-valido", Filename: "a.pdf"}
	if _, err := mongo.DomainToMongo(doc); err == nil {
		t.Error("DomainToMongo debe fallar con un ID que no sea hex de 24 chars")
	}
}

func TestDomainToMongo_EmptyID_ReturnsError(t *testing.T) {
	doc := domain.PDFDocument{Filename: "a.pdf"}
	if _, err := mongo.DomainToMongo(doc); err == nil {
		t.Error("DomainToMongo debe fallar con ID vacío: el mapper es un traductor puro")
	}
}

func TestMongoToDomain_ZeroObjectID_ReturnsError(t *testing.T) {
	doc := mongo.MongoPDFDocument{Filename: "a.pdf"} // ID zero-value
	if _, err := mongo.MongoToDomain(doc); err == nil {
		t.Error("MongoToDomain debe fallar con un ObjectID zero-value")
	}
}
