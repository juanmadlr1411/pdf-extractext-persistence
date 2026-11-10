package consul

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeConn struct {
	local net.Addr
}

func (c *fakeConn) Read([]byte) (int, error)         { return 0, io.EOF }
func (c *fakeConn) Write(p []byte) (int, error)      { return len(p), nil }
func (c *fakeConn) Close() error                     { return nil }
func (c *fakeConn) LocalAddr() net.Addr              { return c.local }
func (c *fakeConn) RemoteAddr() net.Addr             { return &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8500} }
func (c *fakeConn) SetDeadline(time.Time) error      { return nil }
func (c *fakeConn) SetReadDeadline(time.Time) error  { return nil }
func (c *fakeConn) SetWriteDeadline(time.Time) error { return nil }

type nonUDPAddr struct{}

func (nonUDPAddr) Network() string { return "tcp" }
func (nonUDPAddr) String() string  { return "127.0.0.1:8500" }

func TestServiceID(t *testing.T) {
	if got := ServiceID("persistence", "abc123"); got != "persistence-abc123" {
		t.Errorf("ServiceID() = %q, want %q", got, "persistence-abc123")
	}
	if got := ServiceID("", ""); got != "persistence-unknown" {
		t.Errorf("ServiceID() con valores vacíos = %q, want %q", got, "persistence-unknown")
	}
}

func TestServiceIP_UsesOutboundServicesNetwork(t *testing.T) {
	tests := []struct {
		name       string
		consulAddr string
		wantDial   string
	}{
		{name: "host y puerto", consulAddr: "consul:8500", wantDial: "consul:8500"},
		{name: "URL con esquema", consulAddr: "http://consul:8500", wantDial: "consul:8500"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var gotNetwork, gotAddress string
			dial := func(network, address string) (net.Conn, error) {
				gotNetwork = network
				gotAddress = address
				return &fakeConn{local: &net.UDPAddr{IP: net.IPv4(10, 0, 1, 23), Port: 54321}}, nil
			}

			ip, err := serviceIP(dial, tc.consulAddr)
			if err != nil {
				t.Fatalf("serviceIP() error inesperado: %v", err)
			}
			if gotNetwork != "udp" {
				t.Errorf("red del dial = %q, want %q", gotNetwork, "udp")
			}
			if gotAddress != tc.wantDial {
				t.Errorf("dirección del dial = %q, want %q", gotAddress, tc.wantDial)
			}
			if ip != "10.0.1.23" {
				t.Errorf("IP = %q, want %q (debe salir de LocalAddr)", ip, "10.0.1.23")
			}
		})
	}
}

func TestServiceIP_Errors(t *testing.T) {
	t.Run("dial falla", func(t *testing.T) {
		dial := func(string, string) (net.Conn, error) {
			return nil, errors.New("sin ruta")
		}
		if _, err := serviceIP(dial, "consul:8500"); err == nil {
			t.Fatal("serviceIP() esperaba error de dial, obtuvo nil")
		}
	})

	t.Run("local no es UDP", func(t *testing.T) {
		dial := func(string, string) (net.Conn, error) {
			return &fakeConn{local: nonUDPAddr{}}, nil
		}
		if _, err := serviceIP(dial, "consul:8500"); err == nil {
			t.Fatal("serviceIP() esperaba error por dirección local inesperada, obtuvo nil")
		}
	})
}

func TestBuildRegistration_PayloadSnapshot(t *testing.T) {
	reg := BuildRegistration(RegistrationInput{
		ServiceID:   "persistence-host-1",
		ServiceName: "persistence",
		Address:     "10.0.1.23",
		Port:        8002,
		HealthPath:  "/health/ready",
		Version:     "1.0.0",
		Commit:      "abc123",
	})

	got, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		t.Fatalf("marshal del registro: %v", err)
	}

	want, err := os.ReadFile(filepath.Join("testdata", "registration.golden.json"))
	if err != nil {
		t.Fatalf("leer snapshot: %v", err)
	}

	if strings.TrimSpace(string(got)) != strings.TrimSpace(string(want)) {
		t.Errorf("payload de registro inesperado\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestBuildRegistration_UsesInjectedServiceDomain(t *testing.T) {
	reg := BuildRegistration(RegistrationInput{
		ServiceID:     "persistence-host-1",
		ServiceName:   "persistence",
		ServiceDomain: "staging.internal",
		Address:       "10.0.1.23",
		Port:          8002,
		HealthPath:    "/health/ready",
	})

	rule := "Host(`persistence.staging.internal`)"
	tags := strings.Join(reg.Tags, "\n")
	if !strings.Contains(tags, "traefik.http.routers.persistence.rule="+rule) {
		t.Errorf("tags = %v, deben incluir la regla con dominio inyectado", reg.Tags)
	}
	if got := reg.Meta["traefik.http.routers.persistence.rule"]; got != rule {
		t.Errorf("meta rule = %q, want %q", got, rule)
	}
	if strings.Contains(tags, "pdf-extractext.localhost") {
		t.Errorf("tags = %v, no deben contener el dominio hardcodeado", reg.Tags)
	}
}

type fakeRegistrar struct {
	failures int
	attempts int
}

func (f *fakeRegistrar) Register(context.Context, ServiceRegistration) error {
	f.attempts++
	if f.attempts <= f.failures {
		return errors.New("Consul no disponible")
	}
	return nil
}

func (f *fakeRegistrar) Deregister(context.Context, string) error { return nil }

func TestRegisterWithRetry_BacksOffUntilSuccess(t *testing.T) {
	registrar := &fakeRegistrar{failures: 2}
	var delays []time.Duration
	wait := func(_ context.Context, d time.Duration) bool {
		delays = append(delays, d)
		return true
	}

	registerWithRetry(context.Background(), registrar, ServiceRegistration{ID: "persistence-host"}, time.Second, 8*time.Second, wait)

	if registrar.attempts != 3 {
		t.Errorf("intentos = %d, want 3", registrar.attempts)
	}
	wantDelays := []time.Duration{time.Second, 2 * time.Second}
	if len(delays) != len(wantDelays) {
		t.Fatalf("esperas = %v, want %v", delays, wantDelays)
	}
	for i := range wantDelays {
		if delays[i] != wantDelays[i] {
			t.Errorf("espera %d = %v, want %v", i, delays[i], wantDelays[i])
		}
	}
}

func TestRegisterWithRetry_StopsWhenWaitIsInterrupted(t *testing.T) {
	registrar := &fakeRegistrar{failures: 100}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wait := func(_ context.Context, _ time.Duration) bool {
		cancel()
		return false
	}

	registerWithRetry(ctx, registrar, ServiceRegistration{ID: "persistence-host"}, time.Second, 8*time.Second, wait)

	if registrar.attempts != 1 {
		t.Errorf("intentos = %d, want 1 (debe cortar al cancelarse el contexto)", registrar.attempts)
	}
}

func TestWaitFor_RespectsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if waitFor(ctx, time.Second) {
		t.Error("waitFor() debe devolver false con contexto cancelado")
	}
	if !waitFor(context.Background(), time.Millisecond) {
		t.Error("waitFor() debe devolver true cuando vence el timer")
	}
}

func TestGracefulShutdown_DeregistersWaitsThenCloses(t *testing.T) {
	var calls []string

	err := GracefulShutdown(context.Background(), DefaultDrainDelay, ShutdownHooks{
		Deregister: func(context.Context) error {
			calls = append(calls, "deregister")
			return nil
		},
		Wait: func(_ context.Context, d time.Duration) bool {
			calls = append(calls, "wait:"+d.String())
			return true
		},
		Shutdown: func(context.Context) error {
			calls = append(calls, "shutdown")
			return nil
		},
		Disconnect: func(context.Context) error {
			calls = append(calls, "disconnect")
			return nil
		},
	})
	if err != nil {
		t.Fatalf("GracefulShutdown() error inesperado: %v", err)
	}

	want := []string{"deregister", "wait:5s", "shutdown", "disconnect"}
	if strings.Join(calls, ",") != strings.Join(want, ",") {
		t.Errorf("orden de apagado = %v, want %v", calls, want)
	}
}

func TestGracefulShutdown_CancelsDrainAndKeepsDisconnectContextAlive(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const drainWait = time.Second

	deregistered := make(chan struct{})
	disconnectCtxErr := make(chan error, 1)
	done := make(chan error, 1)

	go func() {
		done <- GracefulShutdown(ctx, drainWait, ShutdownHooks{
			Deregister: func(context.Context) error {
				close(deregistered)
				return nil
			},
			Shutdown: func(context.Context) error { return nil },
			Disconnect: func(ctx context.Context) error {
				disconnectCtxErr <- ctx.Err()
				return nil
			},
		})
	}()

	<-deregistered
	time.Sleep(20 * time.Millisecond)
	start := time.Now()
	cancel()

	var shutdownErr error
	select {
	case shutdownErr = <-done:
	case <-time.After(drainWait + time.Second):
		t.Fatal("GracefulShutdown() no terminó")
	}

	if shutdownErr != nil {
		t.Fatalf("GracefulShutdown() error inesperado: %v", shutdownErr)
	}
	if elapsed := time.Since(start); elapsed >= drainWait/2 {
		t.Errorf("drain tardó %v, want menos de %v", elapsed, drainWait/2)
	}

	select {
	case err := <-disconnectCtxErr:
		if err != nil {
			t.Errorf("Disconnect recibió ctx.Err() = %v, want nil", err)
		}
	default:
		t.Error("Disconnect no fue invocado")
	}
}

func TestGracefulShutdown_ContinuesAndReportsErrors(t *testing.T) {
	errDeregister := errors.New("Consul caído")
	errShutdown := errors.New("shutdown falló")
	var calls []string

	err := GracefulShutdown(context.Background(), time.Second, ShutdownHooks{
		Deregister: func(context.Context) error {
			calls = append(calls, "deregister")
			return errDeregister
		},
		Wait: func(context.Context, time.Duration) bool {
			calls = append(calls, "wait")
			return true
		},
		Shutdown: func(context.Context) error {
			calls = append(calls, "shutdown")
			return errShutdown
		},
		Disconnect: func(context.Context) error {
			calls = append(calls, "disconnect")
			return nil
		},
	})

	if !errors.Is(err, errDeregister) || !errors.Is(err, errShutdown) {
		t.Errorf("error = %v, debe incluir deregister y shutdown", err)
	}
	if len(calls) != 4 {
		t.Errorf("pasos ejecutados = %v, want los 4 pasos pese a errores", calls)
	}
}
