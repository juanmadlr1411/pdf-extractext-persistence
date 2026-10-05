package mongo

import (
	"context"
	"errors"
	"fmt"
	"net"

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
//
// Anti-TOCTOU: inserta a ciegas confiando en el índice único de checksum;
// solo si el driver reporta clave duplicada recupera el documento conflictivo
// y lo devuelve envuelto en *domain.DuplicateChecksumError (tipo del issue #3).
// Si el almacén no está disponible, el error satisface
// errors.Is(err, domain.ErrDependencyUnavailable) y la API responde 503.
func (r *DocumentRepository) Save(ctx context.Context, doc domain.Document) (domain.Document, error) {
	res, err := r.client.Collection().InsertOne(ctx, bson.M{
		"filename":       doc.Filename,
		"extracted_text": doc.ExtractedText,
		"checksum":       doc.Checksum,
	}, options.InsertOne())
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return domain.Document{}, r.conflictWithExisting(ctx, doc.Checksum)
		}
		return domain.Document{}, translateMongoError(fmt.Errorf("no se pudo insertar el documento: %w", err))
	}
	if oid, ok := res.InsertedID.(bson.ObjectID); ok {
		doc.ID = oid.Hex()
	}
	return doc, nil
}

// conflictWithExisting recupera el documento que ya posee el checksum para
// construir el error de duplicado con la información completa. Solo se
// invoca en el branch de error de clave duplicada, nunca en el flujo feliz.
func (r *DocumentRepository) conflictWithExisting(ctx context.Context, checksum string) error {
	res := r.client.Collection().FindOne(ctx, bson.M{"checksum": checksum})
	var raw bson.M
	if err := res.Decode(&raw); err != nil {
		return translateMongoError(fmt.Errorf("no se pudo recuperar el documento existente: %w", err))
	}
	existing := documentFromBSON(raw)
	return &domain.DuplicateChecksumError{Existing: domain.PDFDocument{
		ID:            existing.ID,
		Filename:      existing.Filename,
		ExtractedText: existing.ExtractedText,
		Checksum:      existing.Checksum,
	}}
}

// translateMongoError clasifica un error de MongoDB: los fallos de
// conectividad (cliente desconectado, timeouts, red) se traducen a
// domain.ErrDependencyUnavailable; cualquier otro error se devuelve intacto
// para que el llamador preserve su contexto de operación.
func translateMongoError(err error) error {
	switch {
	case errors.Is(err, mongo.ErrClientDisconnected):
		return dependencyDown(err)
	case mongo.IsTimeout(err):
		return dependencyDown(err)
	case isNetworkError(err):
		return dependencyDown(err)
	default:
		return err
	}
}

// isNetworkError detecta fallos de transporte hacia el servidor.
func isNetworkError(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr)
}

// dependencyDown marca el error del driver como indisponibilidad del almacén,
// preservando la causa original en la cadena de errores.
func dependencyDown(err error) error {
	return fmt.Errorf("%w: %w", domain.ErrDependencyUnavailable, err)
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

// UpdateFilename renombra el documento y devuelve su estado actualizado,
// o ErrDocumentNotFound si el ID no existe.
func (r *DocumentRepository) UpdateFilename(ctx context.Context, id, filename string) (domain.Document, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return domain.Document{}, service.ErrDocumentNotFound
	}

	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	var raw bson.M
	err = r.client.Collection().FindOneAndUpdate(ctx,
		bson.M{"_id": oid},
		bson.M{"$set": bson.M{"filename": filename}},
		opts,
	).Decode(&raw)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return domain.Document{}, service.ErrDocumentNotFound
	}
	if err != nil {
		return domain.Document{}, fmt.Errorf("no se pudo actualizar el documento: %w", err)
	}
	return documentFromBSON(raw), nil
}

// DeleteByID elimina el documento con el ID dado; si no existe devuelve
// ErrDocumentNotFound (detectado via DeletedCount).
func (r *DocumentRepository) DeleteByID(ctx context.Context, id string) error {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return service.ErrDocumentNotFound
	}

	res, err := r.client.Collection().DeleteOne(ctx, bson.M{"_id": oid})
	if err != nil {
		return fmt.Errorf("no se pudo eliminar el documento: %w", err)
	}
	if res.DeletedCount == 0 {
		return service.ErrDocumentNotFound
	}
	return nil
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
