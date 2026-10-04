package service

import "context"

// HealthChecker es el puerto para verificar la disponibilidad de dependencias
// críticas (ej. MongoDB). La capa API solo conoce este contrato.
type HealthChecker interface {
	Check(ctx context.Context) error
}
