// Entrypoint del microservicio de persistencia.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pdf-extractext/persistence/internal/api"
	"github.com/pdf-extractext/persistence/internal/config"
	"github.com/pdf-extractext/persistence/internal/infra/mongo"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("configuración inválida", "error", err)
		os.Exit(1)
	}

	// Fail-fast: sin conexión ni índice único garantizado, el servicio no arranca.
	db := mongo.New(cfg.MongoURI, cfg.MongoDatabase, cfg.MongoCollection)

	connectCtx, connectCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer connectCancel()
	if err := db.Connect(connectCtx); err != nil {
		slog.Error("no se pudo conectar a MongoDB", "error", err)
		os.Exit(1)
	}
	if err := db.SetupIndexes(connectCtx); err != nil {
		slog.Error("no se pudieron garantizar los índices", "error", err)
		_ = db.Disconnect(context.Background())
		os.Exit(1)
	}
	slog.Info("conexión a MongoDB establecida", "database", cfg.MongoDatabase, "collection", cfg.MongoCollection)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           api.NewRouter(cfg),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		slog.Info("servicio iniciado", "service", cfg.AppName, "env", cfg.Environment, "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("error al levantar el servidor", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("apagando el servidor...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("error durante el apagado", "error", err)
		os.Exit(1)
	}
	if err := db.Disconnect(shutdownCtx); err != nil {
		slog.Error("error al cerrar la conexión a MongoDB", "error", err)
		os.Exit(1)
	}
	slog.Info("servidor detenido correctamente")
}
