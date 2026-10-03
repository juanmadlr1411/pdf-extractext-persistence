// Package domain contiene las entidades de negocio puras, agnósticas
// de cualquier tecnología de persistencia o transporte.
package domain

import (
	"errors"
	"fmt"
)

// ErrNotFound se devuelve cuando un PDFDocument no existe en el repositorio.
var ErrNotFound = errors.New("documento no encontrado")

// PDFDocument es la entidad de dominio que representa un PDF con su
// texto extraído. El ID es siempre un string hexadecimal de 24 caracteres;
// el dominio no conoce ObjectID ni detalles de MongoDB.
type PDFDocument struct {
	ID            string
	Filename      string
	ExtractedText string
	Checksum      string
}

// DuplicateChecksumError indica que ya existe un documento con el mismo
// checksum. Porta el documento existente para que las capas superiores
// puedan reaccionar (p. ej. devolverlo sin re-procesar).
type DuplicateChecksumError struct {
	Existing PDFDocument
}

func (e *DuplicateChecksumError) Error() string {
	return fmt.Sprintf("checksum duplicado %q: ya existe el documento %q", e.Existing.Checksum, e.Existing.ID)
}
