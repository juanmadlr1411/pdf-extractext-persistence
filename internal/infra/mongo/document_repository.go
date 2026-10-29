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

// DocumentRepository implementa service.DocumentRepository sobre MongoDB.
type DocumentRepository struct {
	client *MongoDBClient
}

// NewDocumentRepository construye el repositorio sobre un MongoDBClient conectado.
func NewDocumentRepository(client *MongoDBClient) *DocumentRepository {
	return &DocumentRepository{client: client}
}

// Save inserta a ciegas y delega la exclusividad en el índice único de checksum
// (anti-TOCTOU): solo ante clave duplicada recupera el documento conflictivo.
// Si el almacén no está disponible, satisface service.ErrDependencyUnavailable.
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

// conflictWithExisting recupera el documento que ya posee el checksum para
// construir el error de duplicado con la información completa. Solo se
// invoca en el branch de error de clave duplicada, nunca en el flujo feliz.
func (r *DocumentRepository) conflictWithExisting(ctx context.Context, checksum string) error {
	findResult := r.client.Collection().FindOne(ctx, bson.M{"checksum": checksum})
	var documentBSON bson.M
	if err := findResult.Decode(&documentBSON); err != nil {
		return translateMongoError(fmt.Errorf("no se pudo recuperar el documento existente: %w", err))
	}
	return &domain.DuplicateChecksumError{Existing: documentFromBSON(documentBSON)}
}

// translateMongoError clasifica un error del driver: los fallos de
// conectividad (cliente desconectado, timeouts, red) se traducen a
// service.ErrDependencyUnavailable; cualquier otro error se devuelve intacto
// para que el llamador preserve su contexto de operación.
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

// isNetworkError detecta fallos de transporte hacia el servidor. Se usa
// errors.As sobre net.Error en lugar de mongo.IsNetworkError porque este
// último solo reconoce errores etiquetados por el driver, no los net.Error
// crudos que produce una conexión rechazada.
func isNetworkError(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr)
}

// wrapAsDependencyUnavailable marca el error del driver como indisponibilidad del
// almacén, preservando la causa original en la cadena de errores.
func wrapAsDependencyUnavailable(err error) error {
	return fmt.Errorf("%w: %w", service.ErrDependencyUnavailable, err)
}

// FindAll devuelve todos los documentos de la colección.
func (r *DocumentRepository) FindAll(ctx context.Context) ([]domain.Document, error) {
	cursor, err := r.client.Collection().Find(ctx, bson.M{})
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

// FindByID devuelve el documento con el ObjectId dado o ErrDocumentNotFound.
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

// UpdateFilename renombra el documento y devuelve su estado actualizado,
// o ErrDocumentNotFound si el ID no existe.
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

// DeleteByID elimina el documento con el ID dado; si no existe devuelve
// ErrDocumentNotFound (detectado via DeletedCount).
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

// decodeDocument decodifica el documento actual del cursor al tipo de dominio.
func decodeDocument(cursor *mongo.Cursor) (domain.Document, error) {
	var documentBSON bson.M
	if err := cursor.Decode(&documentBSON); err != nil {
		return domain.Document{}, fmt.Errorf("no se pudo decodificar el documento: %w", err)
	}
	return documentFromBSON(documentBSON), nil
}

// documentFromBSON mapea un documento BSON a la entidad de dominio.
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

// stringField extrae un campo string de un bson.M de forma segura.
func stringField(documentBSON bson.M, key string) string {
	if v, ok := documentBSON[key].(string); ok {
		return v
	}
	return ""
}
