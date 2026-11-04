package consul

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestFetchKVConfig_ReadsDirectChildren(t *testing.T) {
	entries := []map[string]string{
		{
			"Key":   PersistenceConfigPrefix + "MONGO_DATABASE_NAME",
			"Value": base64.StdEncoding.EncodeToString([]byte("kv_db")),
		},
		{
			"Key":   PersistenceConfigPrefix + "MONGO_COLLECTION_NAME",
			"Value": base64.StdEncoding.EncodeToString([]byte("kv_docs")),
		},
		{
			"Key":   PersistenceConfigPrefix + "nested/IGNORED_KEY",
			"Value": base64.StdEncoding.EncodeToString([]byte("ignorado")),
		},
		{
			"Key":   "pdf-extractext/config/api/MAX_FILE_SIZE_MB",
			"Value": base64.StdEncoding.EncodeToString([]byte("16")),
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/kv/pdf-extractext/config/persistence/" {
			t.Errorf("path = %q, want %q", r.URL.Path, "/v1/kv/pdf-extractext/config/persistence/")
		}
		if !r.URL.Query().Has("recurse") {
			t.Error("la petición KV debe incluir ?recurse")
		}
		if got := r.Header.Get("X-Consul-Token"); got != testToken {
			t.Errorf("X-Consul-Token = %q, want %q", got, testToken)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(entries)
	}))
	defer server.Close()

	values, err := FetchKVConfig(context.Background(), NewClient(server.URL, testToken), PersistenceConfigPrefix)
	if err != nil {
		t.Fatalf("FetchKVConfig() error inesperado: %v", err)
	}

	want := map[string]string{
		"MONGO_DATABASE_NAME":   "kv_db",
		"MONGO_COLLECTION_NAME": "kv_docs",
	}
	if !reflect.DeepEqual(values, want) {
		t.Errorf("valores = %v, want %v", values, want)
	}
}

func TestFetchKVConfig_PropagatesError(t *testing.T) {
	reader := fakeKVReader{err: errors.New("KV no disponible")}

	if _, err := FetchKVConfig(context.Background(), reader, PersistenceConfigPrefix); err == nil {
		t.Fatal("FetchKVConfig() esperaba error, obtuvo nil")
	}
}

func TestFetchKVConfig_EmptyResult(t *testing.T) {
	values, err := FetchKVConfig(context.Background(), fakeKVReader{}, PersistenceConfigPrefix)
	if err != nil {
		t.Fatalf("FetchKVConfig() error inesperado: %v", err)
	}
	if len(values) != 0 {
		t.Errorf("valores = %v, want mapa vacío", values)
	}
}

type fakeKVReader struct {
	entries []KVEntry
	err     error
}

func (f fakeKVReader) KVList(context.Context, string) ([]KVEntry, error) {
	return f.entries, f.err
}
