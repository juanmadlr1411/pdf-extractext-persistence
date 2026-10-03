// Package config carga la configuración del servicio desde variables de entorno.
package config

import (
	"errors"
	"os"
)

// Config contiene la configuración del servicio leída de variables de entorno.
type Config struct {
	AppName         string
	Environment     string
	Port            string
	MongoURI        string
	MongoDatabase   string
	MongoCollection string
}

// Load lee la configuración desde las variables de entorno, aplicando valores
// por defecto cuando corresponde. MONGO_URI es obligatoria: si falta, Load
// devuelve error.
func Load() (Config, error) {
	uri, ok := os.LookupEnv("MONGO_URI")
	if !ok || uri == "" {
		return Config{}, errors.New("MONGO_URI es obligatoria")
	}

	return Config{
		AppName:         envOrDefault("APP_NAME", "pdf-extractext-persistence"),
		Environment:     envOrDefault("APP_ENV", "local"),
		Port:            envOrDefault("APP_PORT", "8002"),
		MongoURI:        uri,
		MongoDatabase:   envOrDefault("MONGO_DATABASE_NAME", "pdf_extractext"),
		MongoCollection: envOrDefault("MONGO_COLLECTION_NAME", "pdf_documents"),
	}, nil
}

func envOrDefault(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
