package service

import "context"

// La capa API solo conoce este contrato (inversión de dependencias).
type HealthChecker interface {
	Check(ctx context.Context) error
}
