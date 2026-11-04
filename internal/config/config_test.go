package config

import "testing"

func TestLoad_FailsWhenMongoURIMissing(t *testing.T) {
	t.Setenv("MONGO_URI", "")

	if _, err := Load(); err == nil {
		t.Fatal("esperado error cuando MONGO_URI no está definida, obtenido nil")
	}
}

func TestLoad_AppliesMongoDefaults(t *testing.T) {
	t.Setenv("MONGO_URI", "mongodb://localhost:27017")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	if cfg.MongoURI != "mongodb://localhost:27017" {
		t.Errorf("MongoURI esperado %q, obtenido %q", "mongodb://localhost:27017", cfg.MongoURI)
	}
	if cfg.MongoDatabase != "pdf_extractext" {
		t.Errorf("MongoDatabase por defecto esperado %q, obtenido %q", "pdf_extractext", cfg.MongoDatabase)
	}
	if cfg.MongoCollection != "pdf_documents" {
		t.Errorf("MongoCollection por defecto esperado %q, obtenido %q", "pdf_documents", cfg.MongoCollection)
	}
}

func TestLoad_RespectsMongoOverrides(t *testing.T) {
	t.Setenv("MONGO_URI", "mongodb://mongo:27017")
	t.Setenv("MONGO_DATABASE_NAME", "custom_db")
	t.Setenv("MONGO_COLLECTION_NAME", "custom_coll")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	if cfg.MongoDatabase != "custom_db" {
		t.Errorf("MongoDatabase esperado %q, obtenido %q", "custom_db", cfg.MongoDatabase)
	}
	if cfg.MongoCollection != "custom_coll" {
		t.Errorf("MongoCollection esperado %q, obtenido %q", "custom_coll", cfg.MongoCollection)
	}
}

func TestLoad_AppliesConsulDefaults(t *testing.T) {
	t.Setenv("MONGO_URI", "mongodb://localhost:27017")
	t.Setenv("CONSUL_ENABLED", "")
	t.Setenv("CONSUL_HTTP_ADDR", "")
	t.Setenv("CONSUL_HTTP_TOKEN", "")
	t.Setenv("SERVICE_NAME", "")
	t.Setenv("SERVICE_PORT", "")
	t.Setenv("SERVICE_HEALTH_PATH", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	if cfg.ConsulEnabled {
		t.Error("ConsulEnabled debe ser false por defecto")
	}
	if cfg.ConsulHTTPAddr != "consul:8500" {
		t.Errorf("ConsulHTTPAddr esperado %q, obtenido %q", "consul:8500", cfg.ConsulHTTPAddr)
	}
	if cfg.ConsulHTTPToken != "" {
		t.Errorf("ConsulHTTPToken esperado vacío, obtenido %q", cfg.ConsulHTTPToken)
	}
	if cfg.ServiceName != "persistence" {
		t.Errorf("ServiceName esperado %q, obtenido %q", "persistence", cfg.ServiceName)
	}
	if cfg.ServicePort != 8002 {
		t.Errorf("ServicePort esperado %d, obtenido %d", 8002, cfg.ServicePort)
	}
	if cfg.ServiceHealthPath != "/health/ready" {
		t.Errorf("ServiceHealthPath esperado %q, obtenido %q", "/health/ready", cfg.ServiceHealthPath)
	}
}

func TestLoad_RespectsConsulOverrides(t *testing.T) {
	t.Setenv("MONGO_URI", "mongodb://localhost:27017")
	t.Setenv("APP_PORT", "9100")
	t.Setenv("CONSUL_ENABLED", "true")
	t.Setenv("CONSUL_HTTP_ADDR", "http://consul:8500")
	t.Setenv("CONSUL_HTTP_TOKEN", "token-dev")
	t.Setenv("SERVICE_NAME", "persistence-test")
	t.Setenv("SERVICE_PORT", "9002")
	t.Setenv("SERVICE_HEALTH_PATH", "/readyz")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	if !cfg.ConsulEnabled {
		t.Error("ConsulEnabled debe ser true")
	}
	if cfg.ConsulHTTPAddr != "http://consul:8500" {
		t.Errorf("ConsulHTTPAddr esperado %q, obtenido %q", "http://consul:8500", cfg.ConsulHTTPAddr)
	}
	if cfg.ConsulHTTPToken != "token-dev" {
		t.Errorf("ConsulHTTPToken esperado %q, obtenido %q", "token-dev", cfg.ConsulHTTPToken)
	}
	if cfg.ServiceName != "persistence-test" {
		t.Errorf("ServiceName esperado %q, obtenido %q", "persistence-test", cfg.ServiceName)
	}
	if cfg.ServicePort != 9002 {
		t.Errorf("ServicePort esperado %d, obtenido %d", 9002, cfg.ServicePort)
	}
	if cfg.ServiceHealthPath != "/readyz" {
		t.Errorf("ServiceHealthPath esperado %q, obtenido %q", "/readyz", cfg.ServiceHealthPath)
	}
}

func TestLoad_ServicePortDefaultsToAppPort(t *testing.T) {
	t.Setenv("MONGO_URI", "mongodb://localhost:27017")
	t.Setenv("APP_PORT", "8010")
	t.Setenv("SERVICE_PORT", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	if cfg.ServicePort != 8010 {
		t.Errorf("ServicePort esperado %d, obtenido %d", 8010, cfg.ServicePort)
	}
}

func TestLoad_RejectsInvalidConsulEnabled(t *testing.T) {
	t.Setenv("MONGO_URI", "mongodb://localhost:27017")
	t.Setenv("CONSUL_ENABLED", "quizás")

	if _, err := Load(); err == nil {
		t.Fatal("esperado error con CONSUL_ENABLED inválido, obtenido nil")
	}
}

func TestLoad_RejectsInvalidServicePort(t *testing.T) {
	t.Setenv("MONGO_URI", "mongodb://localhost:27017")
	t.Setenv("SERVICE_PORT", "no-numérico")

	if _, err := Load(); err == nil {
		t.Fatal("esperado error con SERVICE_PORT inválido, obtenido nil")
	}
}

func TestApplyKV_Precedence(t *testing.T) {
	tests := []struct {
		name           string
		envDatabase    string
		envCollection  string
		kv             map[string]string
		wantDatabase   string
		wantCollection string
	}{
		{
			name:           "sin env ni KV usa defaults",
			kv:             map[string]string{},
			wantDatabase:   "pdf_extractext",
			wantCollection: "pdf_documents",
		},
		{
			name:           "KV pisa los defaults",
			kv:             map[string]string{"MONGO_DATABASE_NAME": "kv_db", "MONGO_COLLECTION_NAME": "kv_docs"},
			wantDatabase:   "kv_db",
			wantCollection: "kv_docs",
		},
		{
			name:           "env pisa a KV",
			envDatabase:    "env_db",
			envCollection:  "env_docs",
			kv:             map[string]string{"MONGO_DATABASE_NAME": "kv_db", "MONGO_COLLECTION_NAME": "kv_docs"},
			wantDatabase:   "env_db",
			wantCollection: "env_docs",
		},
		{
			name:           "valores KV vacíos no pisan los defaults",
			kv:             map[string]string{"MONGO_DATABASE_NAME": "", "MONGO_COLLECTION_NAME": ""},
			wantDatabase:   "pdf_extractext",
			wantCollection: "pdf_documents",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("MONGO_URI", "mongodb://localhost:27017")
			t.Setenv("MONGO_DATABASE_NAME", tc.envDatabase)
			t.Setenv("MONGO_COLLECTION_NAME", tc.envCollection)
			t.Setenv("CONSUL_ENABLED", "")

			cfg, err := Load()
			if err != nil {
				t.Fatalf("error inesperado: %v", err)
			}
			cfg.ApplyKV(tc.kv)

			if cfg.MongoDatabase != tc.wantDatabase {
				t.Errorf("MongoDatabase = %q, want %q", cfg.MongoDatabase, tc.wantDatabase)
			}
			if cfg.MongoCollection != tc.wantCollection {
				t.Errorf("MongoCollection = %q, want %q", cfg.MongoCollection, tc.wantCollection)
			}
		})
	}
}

func TestApplyKV_NeverOverridesMongoURI(t *testing.T) {
	const envURI = "mongodb://env:27017"
	t.Setenv("MONGO_URI", envURI)
	t.Setenv("MONGO_DATABASE_NAME", "")
	t.Setenv("MONGO_COLLECTION_NAME", "")
	t.Setenv("CONSUL_ENABLED", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}

	cfg.ApplyKV(map[string]string{
		"MONGO_URI":             "mongodb://kv:27017",
		"MONGO_DATABASE_NAME":   "kv_db",
		"MONGO_COLLECTION_NAME": "kv_docs",
	})

	if cfg.MongoURI != envURI {
		t.Errorf("MongoURI = %q, want %q (MONGO_URI solo se acepta por env)", cfg.MongoURI, envURI)
	}
}
