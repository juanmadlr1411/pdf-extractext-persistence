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
	"github.com/pdf-extractext/persistence/internal/consul"
	"github.com/pdf-extractext/persistence/internal/infra/mongo"
	"github.com/pdf-extractext/persistence/internal/service"
)

var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("configuración inválida", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var consulClient *consul.Client
	if cfg.ConsulEnabled {
		consulClient = consul.NewClient(cfg.ConsulHTTPAddr, cfg.ConsulHTTPToken)

		kvCtx, cancelKV := context.WithTimeout(ctx, 5*time.Second)
		values, err := consul.FetchKVConfig(kvCtx, consulClient, consul.PersistenceConfigPrefix)
		cancelKV()
		if err != nil {
			slog.Warn("no se pudo leer configuración desde Consul KV; se usan env/defaults", "error", err)
		} else {
			cfg.ApplyKV(values)
			slog.Info("configuración cargada desde Consul KV", "database", cfg.MongoDatabase, "collection", cfg.MongoCollection)
		}
	}

	// Fail-fast: sin conexión ni índice único garantizado, el servicio no arranca.
	mongoClient := mongo.New(cfg.MongoURI, cfg.MongoDatabase, cfg.MongoCollection)

	connectCtx, connectCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer connectCancel()
	if err := mongoClient.Connect(connectCtx); err != nil {
		slog.Error("no se pudo conectar a MongoDB", "error", err)
		os.Exit(1)
	}
	if err := mongoClient.SetupIndexes(connectCtx); err != nil {
		slog.Error("no se pudieron garantizar los índices", "error", err)
		_ = mongoClient.Disconnect(context.Background())
		os.Exit(1)
	}
	slog.Info("conexión a MongoDB establecida", "database", cfg.MongoDatabase, "collection", cfg.MongoCollection)

	docService := service.NewDocumentService(mongo.NewDocumentRepository(mongoClient))

	httpServer := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           api.NewRouter(cfg, docService, mongoClient),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		slog.Info("servicio iniciado", "service", cfg.AppName, "env", cfg.Environment, "port", cfg.Port)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("error al levantar el servidor", "error", err)
			os.Exit(1)
		}
	}()

	var serviceID string
	if cfg.ConsulEnabled {
		hostname, err := os.Hostname()
		if err != nil {
			slog.Warn("no se pudo obtener el hostname; se omite el registro en Consul", "error", err)
		} else if address, err := consul.ServiceIP(cfg.ConsulHTTPAddr); err != nil {
			slog.Warn("no se pudo resolver la IP de la red services; se omite el registro en Consul", "error", err)
		} else {
			serviceID = consul.ServiceID(cfg.ServiceName, hostname)
			registration := consul.BuildRegistration(consul.RegistrationInput{
				ServiceID:     serviceID,
				ServiceName:   cfg.ServiceName,
				ServiceDomain: cfg.ServiceDomain,
				Address:       address,
				Port:          cfg.ServicePort,
				HealthPath:    cfg.ServiceHealthPath,
				Version:       version,
				Commit:        commit,
			})
			go consul.RegisterWithRetry(ctx, consulClient, registration)
		}
	}

	<-ctx.Done()
	slog.Info("apagando el servidor...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	deregister := func(context.Context) error { return nil }
	if consulClient != nil && serviceID != "" {
		deregister = func(shutdownCtx context.Context) error {
			if err := consulClient.Deregister(shutdownCtx, serviceID); err != nil {
				return err
			}
			slog.Info("servicio deregistrado de Consul", "id", serviceID)
			return nil
		}
	}

	if err := consul.GracefulShutdown(shutdownCtx, consul.DefaultDrainDelay, consul.ShutdownHooks{
		Deregister: deregister,
		Wait:       time.Sleep,
		Shutdown:   httpServer.Shutdown,
		Disconnect: mongoClient.Disconnect,
	}); err != nil {
		slog.Error("error durante el apagado", "error", err)
		os.Exit(1)
	}
	slog.Info("servidor detenido correctamente")
}
