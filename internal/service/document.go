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

// ErrDocumentNotFound es el error centinela que indica que un documento
// no existe en el repositorio.
var ErrDocumentNotFound = errors.New("documento no encontrado")

// DocumentRepository es el puerto de persistencia de documentos.
type DocumentRepository interface {
	Save(ctx context.Context, doc domain.Document) (domain.Document, error)
	FindAll(ctx context.Context) ([]domain.Document, error)
	FindByID(ctx context.Context, id string) (domain.Document, error)
}

// DocumentService orquesta las operaciones sobre documentos.
type DocumentService struct {
	repo DocumentRepository
}

// NewDocumentService construye el servicio con su repositorio inyectado.
func NewDocumentService(repo DocumentRepository) *DocumentService {
	return &DocumentService{repo: repo}
}

// Create genera el ID del documento y delega la persistencia al repositorio.
func (s *DocumentService) Create(ctx context.Context, doc domain.Document) (domain.Document, error) {
	doc.ID = newID()
	return s.repo.Save(ctx, doc)
}

// List devuelve la lista completa de documentos.
func (s *DocumentService) List(ctx context.Context) ([]domain.Document, error) {
	return s.repo.FindAll(ctx)
}

// GetByID devuelve el documento con el ID dado o ErrDocumentNotFound.
func (s *DocumentService) GetByID(ctx context.Context, id string) (domain.Document, error) {
	return s.repo.FindByID(ctx, id)
}

// newID genera un ID hexadecimal de 24 caracteres (12 bytes aleatorios).
func newID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
