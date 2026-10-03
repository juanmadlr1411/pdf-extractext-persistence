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
