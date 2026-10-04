package mongo

import (
	"context"
	"errors"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/pdf-extractext/persistence/internal/domain"
	"github.com/pdf-extractext/persistence/internal/service"
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

// FindAll devuelve todos los documentos de la colección.
func (r *DocumentRepository) FindAll(ctx context.Context) ([]domain.Document, error) {
	cursor, err := r.client.Collection().Find(ctx, bson.M{})
	if err != nil {
		return nil, fmt.Errorf("no se pudieron listar los documentos: %w", err)
	}
	defer func() { _ = cursor.Close(ctx) }()

	var docs []domain.Document
	for cursor.Next(ctx) {
		doc, err := decodeDocument(cursor)
		if err != nil {
			return nil, err
		}
		docs = append(docs, doc)
	}
	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("error al iterar los documentos: %w", err)
	}
	return docs, nil
}

// FindByID devuelve el documento con el ObjectId dado o ErrDocumentNotFound.
// El ID debe tener formato válido; el handler lo garantiza antes de llamar.
func (r *DocumentRepository) FindByID(ctx context.Context, id string) (domain.Document, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return domain.Document{}, service.ErrDocumentNotFound
	}

	res := r.client.Collection().FindOne(ctx, bson.M{"_id": oid})
	if errors.Is(res.Err(), mongo.ErrNoDocuments) {
		return domain.Document{}, service.ErrDocumentNotFound
	}
	if res.Err() != nil {
		return domain.Document{}, fmt.Errorf("no se pudo obtener el documento: %w", res.Err())
	}

	var raw bson.M
	if err := res.Decode(&raw); err != nil {
		return domain.Document{}, fmt.Errorf("no se pudo decodificar el documento: %w", err)
	}
	return documentFromBSON(raw), nil
}

// decodeDocument decodifica el documento actual del cursor al tipo de dominio.
func decodeDocument(cursor *mongo.Cursor) (domain.Document, error) {
	var raw bson.M
	if err := cursor.Decode(&raw); err != nil {
		return domain.Document{}, fmt.Errorf("no se pudo decodificar el documento: %w", err)
	}
	return documentFromBSON(raw), nil
}

// documentFromBSON mapea un documento BSON a la entidad de dominio.
func documentFromBSON(raw bson.M) domain.Document {
	doc := domain.Document{
		Filename:      stringField(raw, "filename"),
		ExtractedText: stringField(raw, "extracted_text"),
		Checksum:      stringField(raw, "checksum"),
	}
	if oid, ok := raw["_id"].(bson.ObjectID); ok {
		doc.ID = oid.Hex()
	}
	return doc
}

// stringField extrae un campo string de un bson.M de forma segura.
func stringField(raw bson.M, key string) string {
	if v, ok := raw[key].(string); ok {
		return v
	}
	return ""
}
