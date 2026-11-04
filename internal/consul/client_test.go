package consul

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const testToken = "token-de-prueba"

func TestNewClient_NormalizesAddressAndSetsTimeout(t *testing.T) {
	client := NewClient("consul:8500/", testToken)

	if client.baseURL != "http://consul:8500" {
		t.Errorf("baseURL = %q, want %q", client.baseURL, "http://consul:8500")
	}
	if client.token != testToken {
		t.Errorf("token = %q, want %q", client.token, testToken)
	}
	if client.httpClient.Timeout != defaultRequestTimeout {
		t.Errorf("timeout = %v, want %v", client.httpClient.Timeout, defaultRequestTimeout)
	}

	withScheme := NewClient("https://consul.example:8501", "")
	if withScheme.baseURL != "https://consul.example:8501" {
		t.Errorf("baseURL con esquema = %q, want %q", withScheme.baseURL, "https://consul.example:8501")
	}
}

func TestClient_Register_SendsACLHeaderAndPayload(t *testing.T) {
	reg := ServiceRegistration{
		ID:      "persistence-host",
		Name:    "persistence",
		Address: "10.0.1.23",
		Port:    8002,
		Tags:    []string{"traefik.enable=true"},
		Meta:    map[string]string{"version": "1.0.0"},
		Check: HealthCheck{
			HTTP:                           "http://10.0.1.23:8002/health/ready",
			Interval:                       "10s",
			Timeout:                        "2s",
			DeregisterCriticalServiceAfter: "1m",
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("método = %s, want %s", r.Method, http.MethodPut)
		}
		if r.URL.Path != "/v1/agent/service/register" {
			t.Errorf("path = %q, want %q", r.URL.Path, "/v1/agent/service/register")
		}
		if got := r.Header.Get("X-Consul-Token"); got != testToken {
			t.Errorf("X-Consul-Token = %q, want %q", got, testToken)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want %q", got, "application/json")
		}

		var received ServiceRegistration
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("payload no es JSON válido: %v", err)
		}
		if received.ID != reg.ID || received.Name != reg.Name || received.Address != reg.Address {
			t.Errorf("payload inesperado: %+v", received)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL, testToken)
	if err := client.Register(context.Background(), reg); err != nil {
		t.Fatalf("Register() error inesperado: %v", err)
	}
}

func TestClient_Register_ErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "consul caído", http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(server.URL, testToken)
	if err := client.Register(context.Background(), ServiceRegistration{}); err == nil {
		t.Fatal("Register() esperaba error con status 500, obtuvo nil")
	}
}

func TestClient_Register_RespectsContextTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL, testToken)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	if err := client.Register(ctx, ServiceRegistration{}); err == nil {
		t.Fatal("Register() esperaba error por timeout del contexto, obtuvo nil")
	}
}

func TestClient_Deregister_SendsACLHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("método = %s, want %s", r.Method, http.MethodPut)
		}
		if r.URL.Path != "/v1/agent/service/deregister/persistence-host" {
			t.Errorf("path = %q, want %q", r.URL.Path, "/v1/agent/service/deregister/persistence-host")
		}
		if got := r.Header.Get("X-Consul-Token"); got != testToken {
			t.Errorf("X-Consul-Token = %q, want %q", got, testToken)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL, testToken)
	if err := client.Deregister(context.Background(), "persistence-host"); err != nil {
		t.Fatalf("Deregister() error inesperado: %v", err)
	}
}

func TestClient_Deregister_NotFoundIsIdempotent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, testToken)
	if err := client.Deregister(context.Background(), "inexistente"); err != nil {
		t.Errorf("Deregister() debe tolerar 404, obtuvo error: %v", err)
	}
}

func TestClient_Deregister_ErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "error interno", http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(server.URL, testToken)
	if err := client.Deregister(context.Background(), "persistence-host"); err == nil {
		t.Fatal("Deregister() esperaba error con status 500, obtuvo nil")
	}
}

func TestClient_HealthPassing(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{name: "con instancias passing", body: `[{"Service":{"ID":"persistence-host"}}]`, want: true},
		{name: "sin instancias passing", body: `[]`, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("método = %s, want %s", r.Method, http.MethodGet)
				}
				if r.URL.Path != "/v1/health/service/persistence" {
					t.Errorf("path = %q, want %q", r.URL.Path, "/v1/health/service/persistence")
				}
				if got := r.URL.Query().Get("passing"); got != "true" {
					t.Errorf("passing = %q, want %q", got, "true")
				}
				if got := r.Header.Get("X-Consul-Token"); got != testToken {
					t.Errorf("X-Consul-Token = %q, want %q", got, testToken)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()

			client := NewClient(server.URL, testToken)
			got, err := client.HealthPassing(context.Background(), "persistence")
			if err != nil {
				t.Fatalf("HealthPassing() error inesperado: %v", err)
			}
			if got != tc.want {
				t.Errorf("HealthPassing() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestClient_HealthPassing_ErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "error interno", http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(server.URL, testToken)
	if _, err := client.HealthPassing(context.Background(), "persistence"); err == nil {
		t.Fatal("HealthPassing() esperaba error con status 500, obtuvo nil")
	}
}

func TestClient_KVList_DecodesValues(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte("kv_db"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("método = %s, want %s", r.Method, http.MethodGet)
		}
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
		_, _ = fmt.Fprintf(w, `[{"Key":"pdf-extractext/config/persistence/MONGO_DATABASE_NAME","Value":%q}]`, encoded)
	}))
	defer server.Close()

	client := NewClient(server.URL, testToken)
	entries, err := client.KVList(context.Background(), PersistenceConfigPrefix)
	if err != nil {
		t.Fatalf("KVList() error inesperado: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}
	if entries[0].Key != "pdf-extractext/config/persistence/MONGO_DATABASE_NAME" {
		t.Errorf("Key = %q", entries[0].Key)
	}
	if entries[0].Value != "kv_db" {
		t.Errorf("Value = %q, want %q (debe decodificar base64)", entries[0].Value, "kv_db")
	}
}

func TestClient_KVList_NotFoundReturnsEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, testToken)
	entries, err := client.KVList(context.Background(), PersistenceConfigPrefix)
	if err != nil {
		t.Fatalf("KVList() debe tolerar 404, obtuvo error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("len(entries) = %d, want 0", len(entries))
	}
}

func TestClient_KVList_InvalidBase64(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"Key":"k","Value":"no-es-base64!"}]`))
	}))
	defer server.Close()

	client := NewClient(server.URL, testToken)
	if _, err := client.KVList(context.Background(), PersistenceConfigPrefix); err == nil {
		t.Fatal("KVList() esperaba error por base64 inválido, obtuvo nil")
	}
}

func TestClient_KVList_ErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "error interno", http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(server.URL, testToken)
	if _, err := client.KVList(context.Background(), PersistenceConfigPrefix); err == nil {
		t.Fatal("KVList() esperaba error con status 500, obtuvo nil")
	}
}
