// Package domain contiene las entidades del dominio del Persistence Service.
package domain

import (
	"fmt"
)

// Document representa un documento persistido con su texto extraído.
type Document struct {
	ID            string
	Filename      string
	ExtractedText string
	Checksum      string
}

// DuplicateChecksumError porta el documento existente para que las capas
// superiores puedan devolverlo sin re-procesar.
type DuplicateChecksumError struct {
	Existing Document
}

func (e *DuplicateChecksumError) Error() string {
	return fmt.Sprintf("checksum duplicado %q: ya existe el documento %q", e.Existing.Checksum, e.Existing.ID)
}
