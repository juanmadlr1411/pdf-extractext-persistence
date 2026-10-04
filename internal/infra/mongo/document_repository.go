package mongo

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/pdf-extractext/persistence/internal/domain"
)

// DocumentRepository implementa service.DocumentRepository sobre MongoDB.
// Mínimo necesario para la issue #4: persistir el documento.
type DocumentRepository struct {
	client *Client
}

// NewDocumentRepository construye el repositorio sobre un Client conectado.
func NewDocumentRepository(client *Client) *DocumentRepository {
	return &DocumentRepository{client: client}
}

// Save inserta el documento en la colección y lo devuelve con su ID.
func (r *DocumentRepository) Save(ctx context.Context, doc domain.Document) (domain.Document, error) {
	res, err := r.client.Collection().InsertOne(ctx, bson.M{
		"filename":       doc.Filename,
		"extracted_text": doc.ExtractedText,
		"checksum":       doc.Checksum,
	}, options.InsertOne())
	if err != nil {
		return domain.Document{}, fmt.Errorf("no se pudo insertar el documento: %w", err)
	}
	if oid, ok := res.InsertedID.(bson.ObjectID); ok {
		doc.ID = oid.Hex()
	}
	return doc, nil
}
