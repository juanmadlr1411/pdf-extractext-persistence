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

func parseObjectID(id string) (bson.ObjectID, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return bson.NilObjectID, service.ErrDocumentNotFound
	}
	return oid, nil
}

func toBSONDocument(doc domain.Document) bson.M {
	return bson.M{
		"filename":       doc.Filename,
		"extracted_text": doc.ExtractedText,
		"checksum":       doc.Checksum,
	}
}

func extractInsertedID(res *mongo.InsertOneResult) string {
	if res == nil {
		return ""
	}
	if oid, ok := res.InsertedID.(bson.ObjectID); ok {
		return oid.Hex()
	}
	return ""
}

type DocumentRepository struct {
	client *MongoDBClient
}

func NewDocumentRepository(client *MongoDBClient) *DocumentRepository {
	return &DocumentRepository{client: client}
}

// Save inserta a ciegas (anti-TOCTOU: el índice único arbitra) y traduce la
// indisponibilidad del almacén a service.ErrDependencyUnavailable.
func (r *DocumentRepository) Save(ctx context.Context, doc domain.Document) (domain.Document, error) {
	insertResult, err := r.client.Collection().InsertOne(ctx, toBSONDocument(doc), options.InsertOne())
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return domain.Document{}, r.conflictWithExisting(ctx, doc.Checksum)
		}
		return domain.Document{}, translateMongoError(fmt.Errorf("no se pudo insertar el documento: %w", err))
	}
	if id := extractInsertedID(insertResult); id != "" {
		doc.ID = id
	}
	return doc, nil
}

// conflictWithExisting solo se invoca en el branch de clave duplicada.
func (r *DocumentRepository) conflictWithExisting(ctx context.Context, checksum string) error {
	findResult := r.client.Collection().FindOne(ctx, bson.M{"checksum": checksum})
	var documentBSON bson.M
	if err := findResult.Decode(&documentBSON); err != nil {
		return translateMongoError(fmt.Errorf("no se pudo recuperar el documento existente: %w", err))
	}
	return &domain.DuplicateChecksumError{Existing: documentFromBSON(documentBSON)}
}

// translateMongoError traduce fallos de conectividad a service.ErrDependencyUnavailable.
func translateMongoError(err error) error {
	switch {
	case errors.Is(err, mongo.ErrClientDisconnected):
		return wrapAsDependencyUnavailable(err)
	case mongo.IsTimeout(err):
		return wrapAsDependencyUnavailable(err)
	case isNetworkError(err):
		return wrapAsDependencyUnavailable(err)
	default:
		return err
	}
}

// isNetworkError usa net.Error en lugar de mongo.IsNetworkError: este último
// no reconoce los net.Error crudos de una conexión rechazada.
func isNetworkError(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr)
}

// wrapAsDependencyUnavailable preserva la causa original en la cadena de errores.
func wrapAsDependencyUnavailable(err error) error {
	return fmt.Errorf("%w: %w", service.ErrDependencyUnavailable, err)
}

// DefaultMaxListLimit acota el escaneo máximo de documentos para mitigar DoS y consumo excesivo de memoria.
const DefaultMaxListLimit = 100

func (r *DocumentRepository) FindAll(ctx context.Context) ([]domain.Document, error) {
	opts := options.Find().SetLimit(DefaultMaxListLimit)
	cursor, err := r.client.Collection().Find(ctx, bson.M{}, opts)
	if err != nil {
		return nil, translateMongoError(fmt.Errorf("no se pudieron listar los documentos: %w", err))
	}
	defer func() { _ = cursor.Close(ctx) }()

	var docs []domain.Document
	for cursor.Next(ctx) {
		doc, err := decodeDocument(cursor)
		if err != nil {
			return nil, translateMongoError(err)
		}
		docs = append(docs, doc)
	}
	if err := cursor.Err(); err != nil {
		return nil, translateMongoError(fmt.Errorf("error al iterar los documentos: %w", err))
	}
	return docs, nil
}

// El ID debe tener formato válido; el handler lo garantiza antes de llamar.
func (r *DocumentRepository) FindByID(ctx context.Context, id string) (domain.Document, error) {
	oid, err := parseObjectID(id)
	if err != nil {
		return domain.Document{}, err
	}

	findResult := r.client.Collection().FindOne(ctx, bson.M{"_id": oid})
	if errors.Is(findResult.Err(), mongo.ErrNoDocuments) {
		return domain.Document{}, service.ErrDocumentNotFound
	}
	if findResult.Err() != nil {
		return domain.Document{}, translateMongoError(fmt.Errorf("no se pudo obtener el documento: %w", findResult.Err()))
	}

	var documentBSON bson.M
	if err := findResult.Decode(&documentBSON); err != nil {
		return domain.Document{}, translateMongoError(fmt.Errorf("no se pudo decodificar el documento: %w", err))
	}
	return documentFromBSON(documentBSON), nil
}

func (r *DocumentRepository) FindByChecksum(ctx context.Context, checksum string) (domain.Document, error) {
	findResult := r.client.Collection().FindOne(ctx, bson.M{"checksum": checksum})
	if errors.Is(findResult.Err(), mongo.ErrNoDocuments) {
		return domain.Document{}, service.ErrDocumentNotFound
	}
	if findResult.Err() != nil {
		return domain.Document{}, translateMongoError(fmt.Errorf("no se pudo obtener el documento por checksum: %w", findResult.Err()))
	}

	var documentBSON bson.M
	if err := findResult.Decode(&documentBSON); err != nil {
		return domain.Document{}, translateMongoError(fmt.Errorf("no se pudo decodificar el documento por checksum: %w", err))
	}
	return documentFromBSON(documentBSON), nil
}

func (r *DocumentRepository) UpdateFilename(ctx context.Context, id, filename string) (domain.Document, error) {
	oid, err := parseObjectID(id)
	if err != nil {
		return domain.Document{}, err
	}

	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	var documentBSON bson.M
	err = r.client.Collection().FindOneAndUpdate(ctx,
		bson.M{"_id": oid},
		bson.M{"$set": bson.M{"filename": filename}},
		opts,
	).Decode(&documentBSON)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return domain.Document{}, service.ErrDocumentNotFound
	}
	if err != nil {
		return domain.Document{}, translateMongoError(fmt.Errorf("no se pudo actualizar el documento: %w", err))
	}
	return documentFromBSON(documentBSON), nil
}

func (r *DocumentRepository) DeleteByID(ctx context.Context, id string) error {
	oid, err := parseObjectID(id)
	if err != nil {
		return err
	}

	deleteResult, err := r.client.Collection().DeleteOne(ctx, bson.M{"_id": oid})
	if err != nil {
		return translateMongoError(fmt.Errorf("no se pudo eliminar el documento: %w", err))
	}
	if deleteResult.DeletedCount == 0 {
		return service.ErrDocumentNotFound
	}
	return nil
}

func decodeDocument(cursor *mongo.Cursor) (domain.Document, error) {
	var documentBSON bson.M
	if err := cursor.Decode(&documentBSON); err != nil {
		return domain.Document{}, fmt.Errorf("no se pudo decodificar el documento: %w", err)
	}
	return documentFromBSON(documentBSON), nil
}

func documentFromBSON(documentBSON bson.M) domain.Document {
	doc := domain.Document{
		Filename:      stringField(documentBSON, "filename"),
		ExtractedText: stringField(documentBSON, "extracted_text"),
		Checksum:      stringField(documentBSON, "checksum"),
	}
	if oid, ok := documentBSON["_id"].(bson.ObjectID); ok {
		doc.ID = oid.Hex()
	}
	return doc
}

func stringField(documentBSON bson.M, key string) string {
	if v, ok := documentBSON[key].(string); ok {
		return v
	}
	return ""
}
