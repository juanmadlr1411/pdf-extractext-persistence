package mongo

import (
	"fmt"

	"github.com/pdf-extractext/persistence/internal/domain"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// MongoPDFDocument es el modelo de persistencia de PDFDocument.
// Es el único lugar del sistema donde se mapea a BSON; los tags
// garantizan que el _id de Mongo nunca se filtre hacia el dominio.
type MongoPDFDocument struct {
	ID            bson.ObjectID `bson:"_id"`
	Filename      string        `bson:"filename"`
	ExtractedText string        `bson:"extracted_text"`
	Checksum      string        `bson:"checksum"`
}

// DomainToMongo traduce un PDFDocument de dominio a su modelo BSON.
// Es un traductor puro: devuelve error si el ID no es un hex válido de
// 24 caracteres (incluido el vacío); nunca genera IDs.
func DomainToMongo(doc domain.PDFDocument) (MongoPDFDocument, error) {
	if doc.ID == "" {
		return MongoPDFDocument{}, fmt.Errorf("DomainToMongo: ID vacío, la generación de IDs no es responsabilidad del mapper")
	}
	oid, err := bson.ObjectIDFromHex(doc.ID)
	if err != nil {
		return MongoPDFDocument{}, fmt.Errorf("DomainToMongo: ID %q inválido: %w", doc.ID, err)
	}
	return MongoPDFDocument{
		ID:            oid,
		Filename:      doc.Filename,
		ExtractedText: doc.ExtractedText,
		Checksum:      doc.Checksum,
	}, nil
}

// MongoToDomain traduce un modelo BSON a la entidad de dominio.
// Devuelve error si el ObjectID es el zero-value; si la colección
// contuviera _id de otro tipo, fallaría antes en la decodificación BSON.
func MongoToDomain(doc MongoPDFDocument) (domain.PDFDocument, error) {
	if doc.ID.IsZero() {
		return domain.PDFDocument{}, fmt.Errorf("MongoToDomain: ObjectID zero-value no mapeable a dominio")
	}
	return domain.PDFDocument{
		ID:            doc.ID.Hex(),
		Filename:      doc.Filename,
		ExtractedText: doc.ExtractedText,
		Checksum:      doc.Checksum,
	}, nil
}
