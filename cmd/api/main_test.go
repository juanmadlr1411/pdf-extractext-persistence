package main

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestRunService_ShutsDownWhenServerFails(t *testing.T) {
	cause := errors.New("puerto ocupado")
	shutdownCalls := 0

	done := make(chan int, 1)
	go func() {
		done <- runService(context.Background(), func() error { return cause }, func() error {
			shutdownCalls++
			return nil
		})
	}()

	select {
	case exitCode := <-done:
		if exitCode != 1 {
			t.Errorf("exitCode = %d, want 1", exitCode)
		}
	case <-time.After(time.Second):
		t.Fatal("runService no ejecutó el apagado ante un fallo inesperado del servidor")
	}

	if shutdownCalls != 1 {
		t.Errorf("shutdown invocado %d veces, want 1", shutdownCalls)
	}
}

func TestRunService_ShutsDownOnSignal(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	shutdownCalls := 0

	done := make(chan int, 1)
	go func() {
		done <- runService(ctx, func() error {
			<-ctx.Done()
			return http.ErrServerClosed
		}, func() error {
			shutdownCalls++
			return nil
		})
	}()

	cancel()

	select {
	case exitCode := <-done:
		if exitCode != 0 {
			t.Errorf("exitCode = %d, want 0", exitCode)
		}
	case <-time.After(time.Second):
		t.Fatal("runService no terminó tras la señal")
	}

	if shutdownCalls != 1 {
		t.Errorf("shutdown invocado %d veces, want 1", shutdownCalls)
	}
}

func TestRunService_ReportsShutdownFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan int, 1)
	go func() {
		done <- runService(ctx, func() error {
			<-ctx.Done()
			return http.ErrServerClosed
		}, func() error {
			return errors.New("MongoDB no cerró")
		})
	}()

	cancel()

	select {
	case exitCode := <-done:
		if exitCode != 1 {
			t.Errorf("exitCode = %d, want 1", exitCode)
		}
	case <-time.After(time.Second):
		t.Fatal("runService no terminó tras la señal")
	}
}
