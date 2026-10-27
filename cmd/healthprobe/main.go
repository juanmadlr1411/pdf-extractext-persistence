// healthprobe verifica el readiness local sin shell/curl (imagen distroless): exit 0 si 200, 1 si no.
package main

import (
	"fmt"
	"net/http"
	"os"
	"time"
)

// probeURL arma la URL de readiness con APP_PORT, igual que el servicio (config.Load).
func probeURL() string {
	port := os.Getenv("APP_PORT")
	if port == "" {
		port = "8002"
	}
	return "http://127.0.0.1:" + port + "/health/ready"
}

func main() {
	httpClient := &http.Client{Timeout: 3 * time.Second}
	resp, err := httpClient.Get(probeURL())
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthprobe: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "healthprobe: status %d\n", resp.StatusCode)
		os.Exit(1)
	}
}
