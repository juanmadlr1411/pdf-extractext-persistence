// Package repository implementa la persistencia de las entidades de dominio.
package repository

import (
	"context"
	"fmt"

	mongodriver "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/pdf-extractext/persistence/internal/domain"
	mongoinfra "github.com/pdf-extractext/persistence/internal/infra/mongo"
)

// PDFRepository implementa la persistencia de PDFDocument sobre MongoDB.
type PDFRepository struct {
	collection *mongodriver.Collection
}

// NewPDFRepository crea el repositorio sobre una colección ya conectada
// (inyección de dependencias: el repositorio no gestiona la conexión).
func NewPDFRepository(collection *mongodriver.Collection) *PDFRepository {
	return &PDFRepository{collection: collection}
}

// Insert persiste un nuevo PDFDocument y devuelve su ID en formato hex.
//
// El repositorio genera el ObjectID (el mapper es un traductor puro).
//
// Anti-TOCTOU: inserta a ciegas confiando en el índice único de checksum;
// solo si el driver reporta clave duplicada recupera el documento
// conflictivo y lo devuelve envuelto en domain.DuplicateChecksumError.
func (r *PDFRepository) Insert(ctx context.Context, doc domain.PDFDocument) (string, error) {
	doc.ID = bson.NewObjectID().Hex()

	mongoDoc, err := mongoinfra.DomainToMongo(doc)
	if err != nil {
		return "", fmt.Errorf("Insert: error al mapear el documento: %w", err)
	}

	if _, err := r.collection.InsertOne(ctx, mongoDoc); err != nil {
		if mongodriver.IsDuplicateKeyError(err) {
			existing, findErr := r.findByChecksum(ctx, doc.Checksum)
			if findErr != nil {
				return "", fmt.Errorf("Insert: checksum duplicado pero no se pudo recuperar el documento existente: %w", findErr)
			}
			return "", &domain.DuplicateChecksumError{Existing: existing}
		}
		return "", fmt.Errorf("Insert: error al insertar el documento: %w", err)
	}

	return doc.ID, nil
}

// findByChecksum recupera el documento existente con ese checksum.
// Solo se invoca en el branch de error de clave duplicada, nunca en el
// flujo feliz.
func (r *PDFRepository) findByChecksum(ctx context.Context, checksum string) (domain.PDFDocument, error) {
	var mongoDoc mongoinfra.MongoPDFDocument
	if err := r.collection.FindOne(ctx, bson.M{"checksum": checksum}).Decode(&mongoDoc); err != nil {
		return domain.PDFDocument{}, fmt.Errorf("findByChecksum: %w", err)
	}
	existing, err := mongoinfra.MongoToDomain(mongoDoc)
	if err != nil {
		return domain.PDFDocument{}, fmt.Errorf("findByChecksum: error al mapear el documento existente: %w", err)
	}
	return existing, nil
}
