package mongo

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"

	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/pdf-extractext/persistence/internal/domain"
)

// Tests unitarios de clasificación de errores del driver: la capa Repository
// debe traducir los fallos de conectividad a domain.ErrDependencyUnavailable
// (la API los expone como 503 DEPENDENCY_UNAVAILABLE).

func TestTranslateMongoError_ClientDisconnectedEsIndisponibilidad(t *testing.T) {
	err := translateMongoError(mongo.ErrClientDisconnected)

	if !errors.Is(err, domain.ErrDependencyUnavailable) {
		t.Errorf("esperaba que el cliente desconectado fuese ErrDependencyUnavailable, obtuve %v", err)
	}
}

func TestTranslateMongoError_TimeoutEsIndisponibilidad(t *testing.T) {
	raw := fmt.Errorf("InsertOne: %w", context.DeadlineExceeded)

	err := translateMongoError(raw)

	if !errors.Is(err, domain.ErrDependencyUnavailable) {
		t.Errorf("esperaba que un timeout fuese ErrDependencyUnavailable, obtuve %v", err)
	}
}

func TestTranslateMongoError_ErrorDeRedEsIndisponibilidad(t *testing.T) {
	raw := &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}

	err := translateMongoError(raw)

	if !errors.Is(err, domain.ErrDependencyUnavailable) {
		t.Errorf("esperaba que un error de red fuese ErrDependencyUnavailable, obtuve %v", err)
	}
}

func TestTranslateMongoError_ErrorInesperadoNoEsIndisponibilidad(t *testing.T) {
	raw := errors.New("fallo raro del driver")

	err := translateMongoError(raw)

	if err == nil {
		t.Fatal("esperaba un error, obtuve nil")
	}
	if errors.Is(err, domain.ErrDependencyUnavailable) {
		t.Errorf("un error inesperado no debe clasificarse como indisponibilidad, obtuve %v", err)
	}
}
