package consul

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultDrainDelay da tiempo a Traefik a dejar de enviar tráfico tras el deregistro.
	DefaultDrainDelay = 5 * time.Second

	// DefaultDisconnectTimeout acota el cierre de MongoDB con su propio presupuesto:
	// si el contexto global ya venció durante el HTTP shutdown, la conexión igual se cierra.
	DefaultDisconnectTimeout = 3 * time.Second

	persistenceName   = "persistence"
	persistenceDomain = "pdf-extractext.localhost"
)

// RegistrationInput reúne los datos con los que se construye el payload de Consul.
type RegistrationInput struct {
	ServiceID     string
	ServiceName   string
	ServiceDomain string
	Address       string
	Port          int
	HealthPath    string
	Version       string
	Commit        string
}

// Registrar abstrae las operaciones de Consul que usa el ciclo de vida.
type Registrar interface {
	Register(context.Context, ServiceRegistration) error
	Deregister(context.Context, string) error
}

// ShutdownHooks ordena las dependencias del apagado para poder testear el orden.
// Wait debe respetar el contexto para que una señal de apagado corte el drenaje.
type ShutdownHooks struct {
	Deregister func(context.Context) error
	Wait       func(context.Context, time.Duration) bool
	Shutdown   func(context.Context) error
	Disconnect func(context.Context) error
}

// ServiceID construye el ID único del servicio como <nombre>-<hostname>.
func ServiceID(serviceName, hostname string) string {
	if serviceName == "" {
		serviceName = persistenceName
	}
	if hostname == "" {
		hostname = "unknown"
	}
	return serviceName + "-" + hostname
}

// ServiceIP resuelve la IP saliente hacia Consul para quedarse con la red services.
func ServiceIP(consulAddr string) (string, error) {
	return serviceIP(net.Dial, consulAddr)
}

func serviceIP(dial func(network, address string) (net.Conn, error), consulAddr string) (string, error) {
	// El contenedor tiene dos redes (services y data): el dial a Consul garantiza la IP de services.
	conn, err := dial("udp", hostPort(consulAddr))
	if err != nil {
		return "", fmt.Errorf("no se pudo resolver la IP hacia Consul: %w", err)
	}
	defer conn.Close()

	local, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok {
		return "", fmt.Errorf("dirección local inesperada: %T", conn.LocalAddr())
	}
	return local.IP.String(), nil
}

// BuildRegistration arma el payload con tags y meta espejo para Traefik.
func BuildRegistration(in RegistrationInput) ServiceRegistration {
	name := in.ServiceName
	if name == "" {
		name = persistenceName
	}
	port := in.Port
	if port == 0 {
		port = 8002
	}
	healthPath := in.HealthPath
	if healthPath == "" {
		healthPath = "/health/ready"
	}
	if !strings.HasPrefix(healthPath, "/") {
		healthPath = "/" + healthPath
	}

	domain := in.ServiceDomain
	if domain == "" {
		domain = persistenceDomain
	}

	router := "traefik.http.routers." + name
	servicePortKey := "traefik.http.services." + name + ".loadbalancer.server.port"
	rule := fmt.Sprintf("Host(`%s.%s`)", name, domain)
	portText := strconv.Itoa(port)

	return ServiceRegistration{
		ID:      in.ServiceID,
		Name:    name,
		Address: in.Address,
		Port:    port,
		Tags: []string{
			"traefik.enable=true",
			router + ".rule=" + rule,
			router + ".entrypoints=internal",
			servicePortKey + "=" + portText,
		},
		Meta: map[string]string{
			"version": in.Version,
			"commit":  in.Commit,
		},
		Check: HealthCheck{
			HTTP:                           fmt.Sprintf("http://%s:%d%s", in.Address, port, healthPath),
			Interval:                       "10s",
			Timeout:                        "2s",
			DeregisterCriticalServiceAfter: "1m",
		},
	}
}

// RegisterWithRetry reintenta el alta con backoff exponencial hasta lograrlo o cancelarse.
func RegisterWithRetry(ctx context.Context, client Registrar, reg ServiceRegistration) {
	registerWithRetry(ctx, client, reg, time.Second, 30*time.Second, waitFor)
}

func registerWithRetry(ctx context.Context, client Registrar, reg ServiceRegistration, initial, max time.Duration, wait func(context.Context, time.Duration) bool) {
	delay := initial
	for attempt := 1; ; attempt++ {
		err := client.Register(ctx, reg)
		if err == nil {
			slog.Info("servicio registrado en Consul", "id", reg.ID)
			return
		}

		slog.Warn("registro en Consul falló; reintentando", "intento", attempt, "error", err, "espera", delay)
		if !wait(ctx, delay) {
			return
		}
		delay *= 2
		if delay > max {
			delay = max
		}
	}
}

func waitFor(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// GracefulShutdown ejecuta deregister -> drenaje -> HTTP shutdown -> Mongo disconnect.
func GracefulShutdown(ctx context.Context, drain time.Duration, hooks ShutdownHooks) error {
	var errs []error

	if hooks.Deregister != nil {
		if err := hooks.Deregister(ctx); err != nil {
			errs = append(errs, fmt.Errorf("deregistrar en Consul: %w", err))
		}
	}

	wait := hooks.Wait
	if wait == nil {
		wait = waitFor
	}
	wait(ctx, drain)

	if hooks.Shutdown != nil {
		if err := hooks.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("apagar servidor HTTP: %w", err))
		}
	}
	if hooks.Disconnect != nil {
		disconnectCtx, cancel := context.WithTimeout(context.Background(), DefaultDisconnectTimeout)
		defer cancel()
		if err := hooks.Disconnect(disconnectCtx); err != nil {
			errs = append(errs, fmt.Errorf("cerrar MongoDB: %w", err))
		}
	}

	return errors.Join(errs...)
}
