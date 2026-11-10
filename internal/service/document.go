// Package service contiene la lógica de aplicación del Persistence Service.
package service

import (
	"context"
	"errors"

	"github.com/pdf-extractext/persistence/internal/domain"
)

var ErrDocumentNotFound = errors.New("documento no encontrado")

var ErrFilenameTooLong = errors.New("el filename supera el máximo de 100 caracteres")

var ErrMissingRequiredField = errors.New("falta un campo obligatorio")

const MaxFilenameLength = 100

var ErrDependencyUnavailable = errors.New("dependencia no disponible")

type DocumentRepository interface {
	Save(ctx context.Context, doc domain.Document) (domain.Document, error)
	FindAll(ctx context.Context) ([]domain.Document, error)
	FindByID(ctx context.Context, id string) (domain.Document, error)
	UpdateFilename(ctx context.Context, id, filename string) (domain.Document, error)
	DeleteByID(ctx context.Context, id string) error
}

type DocumentService struct {
	repo DocumentRepository
}

func NewDocumentService(repo DocumentRepository) *DocumentService {
	return &DocumentService{repo: repo}
}

// El identificador es asignado por la capa de persistencia (MongoDB).
func (s *DocumentService) Create(ctx context.Context, doc domain.Document) (domain.Document, error) {
	if doc.Filename == "" || doc.Checksum == "" {
		return domain.Document{}, ErrMissingRequiredField
	}
	if len(doc.Filename) > MaxFilenameLength {
		return domain.Document{}, ErrFilenameTooLong
	}
	return s.repo.Save(ctx, doc)
}

func (s *DocumentService) List(ctx context.Context) ([]domain.Document, error) {
	return s.repo.FindAll(ctx)
}

func (s *DocumentService) GetByID(ctx context.Context, id string) (domain.Document, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *DocumentService) UpdateFilename(ctx context.Context, id, filename string) (domain.Document, error) {
	if filename == "" {
		return domain.Document{}, ErrMissingRequiredField
	}
	if len(filename) > MaxFilenameLength {
		return domain.Document{}, ErrFilenameTooLong
	}
	return s.repo.UpdateFilename(ctx, id, filename)
}

func (s *DocumentService) Delete(ctx context.Context, id string) error {
	return s.repo.DeleteByID(ctx, id)
}
