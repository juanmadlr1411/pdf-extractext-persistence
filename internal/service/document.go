// Package service contiene la lógica de aplicación del Persistence Service.
// Por ahora solo orquesta el repositorio; la validación de invariantes
// está delegada a issues posteriores.
package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	"github.com/pdf-extractext/persistence/internal/domain"
)

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

// Create genera el ID del documento y delega la persistencia al repositorio.
func (s *DocumentService) Create(ctx context.Context, doc domain.Document) (domain.Document, error) {
	doc.ID = newID()
	return s.repo.Save(ctx, doc)
}

// newID genera un ID hexadecimal de 24 caracteres (12 bytes aleatorios).
func newID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
