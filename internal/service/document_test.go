package service

import (
	"context"
	"errors"
	"testing"

	"github.com/pdf-extractext/persistence/internal/domain"
)

type fakeRepo struct {
	saveFn           func(context.Context, domain.Document) (domain.Document, error)
	findAllFn        func(context.Context) ([]domain.Document, error)
	findByIDFn       func(context.Context, string) (domain.Document, error)
	updateFilenameFn func(context.Context, string, string) (domain.Document, error)
	deleteByIDFn     func(context.Context, string) error

	saveCalls []domain.Document
}

func (f *fakeRepo) Save(ctx context.Context, doc domain.Document) (domain.Document, error) {
	f.saveCalls = append(f.saveCalls, doc)
	if f.saveFn != nil {
		return f.saveFn(ctx, doc)
	}
	return doc, nil
}

func (f *fakeRepo) FindAll(ctx context.Context) ([]domain.Document, error) {
	if f.findAllFn != nil {
		return f.findAllFn(ctx)
	}
	return []domain.Document{}, nil
}

func (f *fakeRepo) FindByID(ctx context.Context, id string) (domain.Document, error) {
	if f.findByIDFn != nil {
		return f.findByIDFn(ctx, id)
	}
	return domain.Document{}, ErrDocumentNotFound
}

func (f *fakeRepo) UpdateFilename(ctx context.Context, id, filename string) (domain.Document, error) {
	if f.updateFilenameFn != nil {
		return f.updateFilenameFn(ctx, id, filename)
	}
	return domain.Document{}, ErrDocumentNotFound
}

func (f *fakeRepo) DeleteByID(ctx context.Context, id string) error {
	if f.deleteByIDFn != nil {
		return f.deleteByIDFn(ctx, id)
	}
	return ErrDocumentNotFound
}

func TestCreate_DelegatesToRepoWithoutPreGeneratingID(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{
		saveFn: func(ctx context.Context, doc domain.Document) (domain.Document, error) {
			doc.ID = "507f1f77bcf86cd799439011"
			return doc, nil
		},
	}
	svc := NewDocumentService(repo)

	input := domain.Document{
		Filename:      "informe.pdf",
		ExtractedText: "texto extraído",
		Checksum:      "sha256:abc123",
	}
	got, err := svc.Create(context.Background(), input)
	if err != nil {
		t.Fatalf("Create no debería fallar: %v", err)
	}
	if got.ID != "507f1f77bcf86cd799439011" {
		t.Fatalf("se esperaba ID asignado por repo, obtenido %q", got.ID)
	}
	if len(repo.saveCalls) != 1 {
		t.Fatalf("se esperaba 1 llamada a Save, obtenidas %d", len(repo.saveCalls))
	}
	called := repo.saveCalls[0]
	if called.ID != "" {
		t.Fatalf("Create no debe pre-generar ID, Save recibió ID %q", called.ID)
	}
	if called.Filename != input.Filename || called.Checksum != input.Checksum {
		t.Fatalf("Save debe recibir el documento original: %+v", called)
	}
}

func TestCreate_MissingFilename_ReturnsErrMissingRequiredField(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{}
	svc := NewDocumentService(repo)

	_, err := svc.Create(context.Background(), domain.Document{
		Checksum: "sha256:abc123",
	})
	if !errors.Is(err, ErrMissingRequiredField) {
		t.Fatalf("esperado ErrMissingRequiredField, obtenido %v", err)
	}
	if len(repo.saveCalls) != 0 {
		t.Fatalf("no debe llamar a repo.Save cuando hay validación fallida")
	}
}

func TestCreate_EmptyFilename_ReturnsErrMissingRequiredField(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{}
	svc := NewDocumentService(repo)

	_, err := svc.Create(context.Background(), domain.Document{
		Filename: "",
		Checksum: "sha256:abc123",
	})
	if !errors.Is(err, ErrMissingRequiredField) {
		t.Fatalf("esperado ErrMissingRequiredField, obtenido %v", err)
	}
	if len(repo.saveCalls) != 0 {
		t.Fatalf("no debe llamar a repo.Save cuando hay validación fallida")
	}
}

func TestCreate_MissingChecksum_ReturnsErrMissingRequiredField(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{}
	svc := NewDocumentService(repo)

	_, err := svc.Create(context.Background(), domain.Document{
		Filename: "informe.pdf",
	})
	if !errors.Is(err, ErrMissingRequiredField) {
		t.Fatalf("esperado ErrMissingRequiredField, obtenido %v", err)
	}
	if len(repo.saveCalls) != 0 {
		t.Fatalf("no debe llamar a repo.Save cuando hay validación fallida")
	}
}

func TestCreate_EmptyChecksum_ReturnsErrMissingRequiredField(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{}
	svc := NewDocumentService(repo)

	_, err := svc.Create(context.Background(), domain.Document{
		Filename: "informe.pdf",
		Checksum: "",
	})
	if !errors.Is(err, ErrMissingRequiredField) {
		t.Fatalf("esperado ErrMissingRequiredField, obtenido %v", err)
	}
	if len(repo.saveCalls) != 0 {
		t.Fatalf("no debe llamar a repo.Save cuando hay validación fallida")
	}
}

func TestCreate_FilenameTooLong_ReturnsErrFilenameTooLong(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{}
	svc := NewDocumentService(repo)

	longName := ""
	for i := 0; i < MaxFilenameLength+1; i++ {
		longName += "a"
	}

	_, err := svc.Create(context.Background(), domain.Document{
		Filename: longName,
		Checksum: "sha256:abc123",
	})
	if !errors.Is(err, ErrFilenameTooLong) {
		t.Fatalf("esperado ErrFilenameTooLong, obtenido %v", err)
	}
	if len(repo.saveCalls) != 0 {
		t.Fatalf("no debe llamar a repo.Save cuando hay validación fallida")
	}
}

func TestUpdateFilename_Success(t *testing.T) {
	t.Parallel()
	want := domain.Document{ID: "507f1f77bcf86cd799439011", Filename: "nuevo.pdf", ExtractedText: "x", Checksum: "sha256:a"}
	repo := &fakeRepo{
		updateFilenameFn: func(ctx context.Context, id, filename string) (domain.Document, error) {
			if id != want.ID || filename != want.Filename {
				t.Fatalf("parámetros incorrectos: id=%s filename=%s", id, filename)
			}
			return want, nil
		},
	}
	svc := NewDocumentService(repo)

	got, err := svc.UpdateFilename(context.Background(), want.ID, want.Filename)
	if err != nil {
		t.Fatalf("UpdateFilename no debería fallar: %v", err)
	}
	if got != want {
		t.Fatalf("resultado incorrecto: got %+v want %+v", got, want)
	}
}

func TestUpdateFilename_EmptyFilename_ReturnsErrMissingRequiredField(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{}
	svc := NewDocumentService(repo)

	_, err := svc.UpdateFilename(context.Background(), "507f1f77bcf86cd799439011", "")
	if !errors.Is(err, ErrMissingRequiredField) {
		t.Fatalf("esperado ErrMissingRequiredField, obtenido %v", err)
	}
}

func TestUpdateFilename_FilenameTooLong_ReturnsErrFilenameTooLong(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{}
	svc := NewDocumentService(repo)

	longName := ""
	for i := 0; i < MaxFilenameLength+1; i++ {
		longName += "a"
	}

	_, err := svc.UpdateFilename(context.Background(), "507f1f77bcf86cd799439011", longName)
	if !errors.Is(err, ErrFilenameTooLong) {
		t.Fatalf("esperado ErrFilenameTooLong, obtenido %v", err)
	}
}

func TestUpdateFilename_NotFound_PropagatesErrDocumentNotFound(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{
		updateFilenameFn: func(ctx context.Context, id, filename string) (domain.Document, error) {
			return domain.Document{}, ErrDocumentNotFound
		},
	}
	svc := NewDocumentService(repo)

	_, err := svc.UpdateFilename(context.Background(), "507f1f77bcf86cd7994390ff", "nuevo.pdf")
	if !errors.Is(err, ErrDocumentNotFound) {
		t.Fatalf("esperado ErrDocumentNotFound, obtenido %v", err)
	}
}

func TestList_ReturnsDocsAndPropagatesErrors(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		want := []domain.Document{{ID: "1", Filename: "a.pdf", Checksum: "sha256:a"}}
		repo := &fakeRepo{
			findAllFn: func(ctx context.Context) ([]domain.Document, error) {
				return want, nil
			},
		}
		svc := NewDocumentService(repo)
		got, err := svc.List(context.Background())
		if err != nil {
			t.Fatalf("List no debería fallar: %v", err)
		}
		if len(got) != len(want) {
			t.Fatalf("resultado incorrecto: got %+v want %+v", got, want)
		}
	})

	t.Run("dependency unavailable", func(t *testing.T) {
		t.Parallel()
		repo := &fakeRepo{
			findAllFn: func(ctx context.Context) ([]domain.Document, error) {
				return nil, ErrDependencyUnavailable
			},
		}
		svc := NewDocumentService(repo)
		_, err := svc.List(context.Background())
		if !errors.Is(err, ErrDependencyUnavailable) {
			t.Fatalf("esperado ErrDependencyUnavailable, obtenido %v", err)
		}
	})
}

func TestGetByID_SuccessAndPropagatesErrors(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		want := domain.Document{ID: "507f1f77bcf86cd799439011", Filename: "a.pdf", Checksum: "sha256:a"}
		repo := &fakeRepo{
			findByIDFn: func(ctx context.Context, id string) (domain.Document, error) {
				if id != want.ID {
					t.Fatalf("id incorrecto: %s", id)
				}
				return want, nil
			},
		}
		svc := NewDocumentService(repo)
		got, err := svc.GetByID(context.Background(), want.ID)
		if err != nil {
			t.Fatalf("GetByID no debería fallar: %v", err)
		}
		if got != want {
			t.Fatalf("resultado incorrecto: got %+v want %+v", got, want)
		}
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		repo := &fakeRepo{
			findByIDFn: func(ctx context.Context, id string) (domain.Document, error) {
				return domain.Document{}, ErrDocumentNotFound
			},
		}
		svc := NewDocumentService(repo)
		_, err := svc.GetByID(context.Background(), "507f1f77bcf86cd7994390ff")
		if !errors.Is(err, ErrDocumentNotFound) {
			t.Fatalf("esperado ErrDocumentNotFound, obtenido %v", err)
		}
	})
}

func TestDelete_SuccessAndPropagatesErrors(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		wantID := "507f1f77bcf86cd799439011"
		repo := &fakeRepo{
			deleteByIDFn: func(ctx context.Context, id string) error {
				if id != wantID {
					t.Fatalf("id incorrecto: %s", id)
				}
				return nil
			},
		}
		svc := NewDocumentService(repo)
		if err := svc.Delete(context.Background(), wantID); err != nil {
			t.Fatalf("Delete no debería fallar: %v", err)
		}
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		repo := &fakeRepo{
			deleteByIDFn: func(ctx context.Context, id string) error {
				return ErrDocumentNotFound
			},
		}
		svc := NewDocumentService(repo)
		err := svc.Delete(context.Background(), "507f1f77bcf86cd7994390ff")
		if !errors.Is(err, ErrDocumentNotFound) {
			t.Fatalf("esperado ErrDocumentNotFound, obtenido %v", err)
		}
	})

	t.Run("dependency unavailable", func(t *testing.T) {
		t.Parallel()
		repo := &fakeRepo{
			deleteByIDFn: func(ctx context.Context, id string) error {
				return ErrDependencyUnavailable
			},
		}
		svc := NewDocumentService(repo)
		err := svc.Delete(context.Background(), "507f1f77bcf86cd799439011")
		if !errors.Is(err, ErrDependencyUnavailable) {
			t.Fatalf("esperado ErrDependencyUnavailable, obtenido %v", err)
		}
	})
}
