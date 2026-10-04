// Package domain contiene las entidades del dominio del Persistence Service.
package domain

// Document representa un documento persistido con su texto extraído.
type Document struct {
	ID            string
	Filename      string
	ExtractedText string
	Checksum      string
}
