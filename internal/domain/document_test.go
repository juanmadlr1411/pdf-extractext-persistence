package domain_test

import (
	"errors"
	"testing"

	"github.com/pdf-extractext/persistence/internal/domain"
)

func TestDocument_ZeroValue(t *testing.T) {
	var d domain.Document
	if d.ID != "" || d.Filename != "" || d.ExtractedText != "" || d.Checksum != "" {
		t.Fatalf("se esperaba zero value vacío, obtenido %+v", d)
	}
}

func TestDuplicateChecksumError_ContainsExistingDocument(t *testing.T) {
	existing := domain.Document{
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
