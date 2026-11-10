package consul

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnvExampleDocumentsRequiredVariables(t *testing.T) {
	content := readRepoFile(t, ".env.example")

	required := []string{
		"APP_PORT=",
		"MONGO_URI=",
		"MONGO_DATABASE_NAME=",
		"MONGO_COLLECTION_NAME=",
		"CONSUL_ENABLED=",
		"CONSUL_HTTP_ADDR=",
		"CONSUL_HTTP_TOKEN=",
		"SERVICE_NAME=",
		"SERVICE_PORT=",
		"SERVICE_HEALTH_PATH=",
	}
	for _, key := range required {
		if !strings.Contains(content, key) {
			t.Errorf(".env.example debe documentar %q", key)
		}
	}
}

func TestLocalComposeDisablesConsul(t *testing.T) {
	content := readRepoFile(t, "docker-compose.yml")

	if !strings.Contains(content, "CONSUL_ENABLED=false") &&
		!strings.Contains(content, "CONSUL_ENABLED=${CONSUL_ENABLED:-false}") {
		t.Error("docker-compose.yml local debe declarar CONSUL_ENABLED=false o su default parametrizado")
	}
}

func TestMakefileTestRunsWithRaceDetector(t *testing.T) {
	content := readRepoFile(t, "Makefile")

	if !strings.Contains(content, "go test -race ./...") {
		t.Error("make test debe ejecutar go test -race ./...")
	}
}

func TestReadmeDocumentsConsulRegistration(t *testing.T) {
	content := readRepoFile(t, "README.md")

	required := []string{
		"Consul",
		"pdf-extractext/config/persistence",
		"persistence.pdf-extractext.localhost",
		"red `services`",
		"X-Consul-Token",
		"SIGTERM",
	}
	for _, fragment := range required {
		if !strings.Contains(content, fragment) {
			t.Errorf("README.md debe documentar %q", fragment)
		}
	}
}

func readRepoFile(t *testing.T, name string) string {
	t.Helper()

	content, err := os.ReadFile(filepath.Join("..", "..", name))
	if err != nil {
		t.Fatalf("leer %s: %v", name, err)
	}
	return string(content)
}
