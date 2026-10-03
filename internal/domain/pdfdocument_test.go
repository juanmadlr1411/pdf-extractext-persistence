package domain_test

import (
	"errors"
	"testing"

	"github.com/pdf-extractext/persistence/internal/domain"
)

func TestErrNotFound_IsSentinel(t *testing.T) {
	wrapped := errors.Join(errors.New("capa repositorio"), domain.ErrNotFound)
	if !errors.Is(wrapped, domain.ErrNotFound) {
		t.Error("ErrNotFound debe ser detectable con errors.Is aunque esté envuelto")
	}
}

func TestDuplicateChecksumError_ContainsExistingDocument(t *testing.T) {
	existing := domain.PDFDocument{
		ID:            "64b7f8e2f9a1c2d3e4b5a677",
		Filename:      "doc-existente.pdf",
		ExtractedText: "texto previo",
		Checksum:      "abc123",
	}
	err := &domain.DuplicateChecksumError{Existing: existing}

	if err.Error() == "" {
		t.Error("DuplicateChecksumError debe implementar Error()")
	}
	if err.Existing != existing {
		t.Errorf("Existing = %+v, want %+v", err.Existing, existing)
	}

	var dupErr *domain.DuplicateChecksumError
	if !errors.As(err, &dupErr) {
		t.Error("debe funcionar con errors.As")
	}
	if dupErr.Existing.Checksum != "abc123" {
		t.Errorf("checksum del documento existente = %q, want %q", dupErr.Existing.Checksum, "abc123")
	}
}
