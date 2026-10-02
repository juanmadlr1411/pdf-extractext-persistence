// FASE GREEN: Implementación mínima para pasar el test.
package config

import "os"

// Config contiene la configuración del servicio leída de variables de entorno.
type Config struct {
	AppName     string
	Environment string
	Port        string
}

// Load lee la configuración desde APP_NAME, APP_ENV y APP_PORT,
// aplicando valores por defecto cuando no están definidas.
func Load() Config {
	return Config{
		AppName:     envOrDefault("APP_NAME", "pdf-extractext-persistence"),
		Environment: envOrDefault("APP_ENV", "local"),
		Port:        envOrDefault("APP_PORT", "8002"),
	}
}

func envOrDefault(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
