// Package config carga la configuración del servicio desde variables de entorno.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	AppName         string
	Environment     string
	Port            string
	MongoURI        string
	MongoDatabase   string
	MongoCollection string

	ConsulHTTPAddr    string
	ConsulHTTPToken   string
	ConsulEnabled     bool
	ServiceName       string
	ServiceDomain     string
	ServicePort       int
	ServiceHealthPath string

	mongoDatabaseFromEnv   bool
	mongoCollectionFromEnv bool
}

// MONGO_URI es obligatoria: si falta, Load devuelve error.
func Load() (Config, error) {
	uri, ok := os.LookupEnv("MONGO_URI")
	if !ok || uri == "" {
		return Config{}, errors.New("MONGO_URI es obligatoria")
	}

	appPort := envOrDefault("APP_PORT", "8002")
	servicePort, err := strconv.Atoi(envOrDefault("SERVICE_PORT", appPort))
	if err != nil {
		return Config{}, fmt.Errorf("SERVICE_PORT inválida: %w", err)
	}

	consulEnabled, err := boolFromEnv("CONSUL_ENABLED")
	if err != nil {
		return Config{}, err
	}

	mongoDatabase, mongoDatabaseFromEnv := lookupNonEmpty("MONGO_DATABASE_NAME")
	if !mongoDatabaseFromEnv {
		mongoDatabase = "pdf_extractext"
	}
	mongoCollection, mongoCollectionFromEnv := lookupNonEmpty("MONGO_COLLECTION_NAME")
	if !mongoCollectionFromEnv {
		mongoCollection = "pdf_documents"
	}

	return Config{
		AppName:                envOrDefault("APP_NAME", "pdf-extractext-persistence"),
		Environment:            envOrDefault("APP_ENV", "local"),
		Port:                   appPort,
		MongoURI:               uri,
		MongoDatabase:          mongoDatabase,
		MongoCollection:        mongoCollection,
		ConsulHTTPAddr:         envOrDefault("CONSUL_HTTP_ADDR", "consul:8500"),
		ConsulHTTPToken:        os.Getenv("CONSUL_HTTP_TOKEN"),
		ConsulEnabled:          consulEnabled,
		ServiceName:            envOrDefault("SERVICE_NAME", "persistence"),
		ServiceDomain:          envOrDefault("SERVICE_DOMAIN", "pdf-extractext.localhost"),
		ServicePort:            servicePort,
		ServiceHealthPath:      envOrDefault("SERVICE_HEALTH_PATH", "/health/ready"),
		mongoDatabaseFromEnv:   mongoDatabaseFromEnv,
		mongoCollectionFromEnv: mongoCollectionFromEnv,
	}, nil
}

// Precedencia env > KV > default.
func (c *Config) ApplyKV(values map[string]string) {
	if !c.mongoDatabaseFromEnv {
		if v := values["MONGO_DATABASE_NAME"]; v != "" {
			c.MongoDatabase = v
		}
	}
	if !c.mongoCollectionFromEnv {
		if v := values["MONGO_COLLECTION_NAME"]; v != "" {
			c.MongoCollection = v
		}
	}
	// MONGO_URI nunca se lee del KV: solo se acepta por variable de entorno.
}

func envOrDefault(key, fallback string) string {
	if v, ok := lookupNonEmpty(key); ok {
		return v
	}
	return fallback
}

func lookupNonEmpty(key string) (string, bool) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return "", false
	}
	return v, true
}

func boolFromEnv(key string) (bool, error) {
	raw, ok := lookupNonEmpty(key)
	if !ok {
		return false, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s inválida (%q): %w", key, raw, err)
	}
	return value, nil
}
