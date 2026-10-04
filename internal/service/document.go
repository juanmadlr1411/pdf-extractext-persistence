// Package service contiene la lógica de aplicación del Persistence Service.
// Por ahora solo orquesta el repositorio; la validación de invariantes
// está delegada a issues posteriores.
package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"

	"github.com/pdf-extractext/persistence/internal/domain"
)

// MaxFilenameLength es la longitud máxima permitida para el nombre de archivo.
const MaxFilenameLength = 100

// ErrFilenameTooLong indica que el filename supera MaxFilenameLength.
var ErrFilenameTooLong = errors.New("filename supera la longitud máxima permitida")

// DocumentRepository es el puerto de persistencia de documentos.
type DocumentRepository interface {
	Save(ctx context.Context, doc domain.Document) (domain.Document, error)
}

// DocumentService orquesta la creación de documentos.
type DocumentService struct {
	repo DocumentRepository
}

// NewDocumentService construye el servicio con su repositorio inyectado.
func NewDocumentService(repo DocumentRepository) *DocumentService {
	return &DocumentService{repo: repo}
}

// Create valida los invariantes, genera el ID y delega la persistencia.
func (s *DocumentService) Create(ctx context.Context, doc domain.Document) (domain.Document, error) {
	if len(doc.Filename) > MaxFilenameLength {
		return domain.Document{}, ErrFilenameTooLong
	}
	doc.ID = newID()
	return s.repo.Save(ctx, doc)
}

// newID genera un ID hexadecimal de 24 caracteres (12 bytes aleatorios).
func newID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
