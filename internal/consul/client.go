// Package consul implementa un cliente HTTP mínimo para la API de Consul.
package consul

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultRequestTimeout = 5 * time.Second

	// PersistenceConfigPrefix es la raíz de configuración del servicio en Consul KV.
	PersistenceConfigPrefix = "pdf-extractext/config/persistence/"
)

// ServiceRegistration representa el payload de registro de un servicio.
type ServiceRegistration struct {
	ID      string            `json:"ID"`
	Name    string            `json:"Name"`
	Address string            `json:"Address"`
	Port    int               `json:"Port"`
	Tags    []string          `json:"Tags,omitempty"`
	Meta    map[string]string `json:"Meta,omitempty"`
	Check   HealthCheck       `json:"Check"`
}

// HealthCheck describe el chequeo HTTP que Consul ejecuta contra el servicio.
type HealthCheck struct {
	HTTP                           string `json:"HTTP"`
	Interval                       string `json:"Interval"`
	Timeout                        string `json:"Timeout"`
	DeregisterCriticalServiceAfter string `json:"DeregisterCriticalServiceAfter"`
}

// KVEntry representa una clave del KV de Consul con su valor ya decodificado.
type KVEntry struct {
	Key   string `json:"Key"`
	Value string `json:"Value"`
}

// Client es un cliente HTTP sin SDK para la API de Consul.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewClient construye un cliente apuntando a Consul y aplica timeout a cada request.
func NewClient(addr, token string) *Client {
	return &Client{
		baseURL:    normalizeBaseURL(addr),
		token:      token,
		httpClient: &http.Client{Timeout: defaultRequestTimeout},
	}
}

// Register registra el servicio en el agente local de Consul.
func (c *Client) Register(ctx context.Context, reg ServiceRegistration) error {
	payload, err := json.Marshal(reg)
	if err != nil {
		return fmt.Errorf("serializar registro de Consul: %w", err)
	}

	resp, err := c.do(ctx, http.MethodPut, "/v1/agent/service/register", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	return ensureOK(resp, "register")
}

// Deregister elimina el servicio del catálogo; un 404 se considera éxito.
func (c *Client) Deregister(ctx context.Context, serviceID string) error {
	resp, err := c.do(ctx, http.MethodPut, "/v1/agent/service/deregister/"+url.PathEscape(serviceID), nil)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusNotFound {
		_ = resp.Body.Close()
		return nil
	}
	return ensureOK(resp, "deregister")
}

// HealthPassing indica si Consul reporta al menos una instancia passing.
func (c *Client) HealthPassing(ctx context.Context, serviceName string) (bool, error) {
	resp, err := c.do(ctx, http.MethodGet, "/v1/health/service/"+url.PathEscape(serviceName)+"?passing=true", nil)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if err := statusError(resp, "health"); err != nil {
		return false, err
	}

	var instances []json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&instances); err != nil {
		return false, fmt.Errorf("decodificar health de Consul: %w", err)
	}
	return len(instances) > 0, nil
}

// KVList lista un prefijo recursivo y decodifica los valores base64 de Consul.
func (c *Client) KVList(ctx context.Context, prefix string) ([]KVEntry, error) {
	path := "/v1/kv/" + strings.TrimPrefix(prefix, "/")
	resp, err := c.do(ctx, http.MethodGet, path+"?recurse", nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		_ = resp.Body.Close()
		return nil, nil
	}
	defer resp.Body.Close()

	if err := statusError(resp, "kv"); err != nil {
		return nil, err
	}

	var entries []KVEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, fmt.Errorf("decodificar KV de Consul: %w", err)
	}
	for i := range entries {
		if entries[i].Value == "" {
			continue
		}
		decoded, err := base64.StdEncoding.DecodeString(entries[i].Value)
		if err != nil {
			return nil, fmt.Errorf("decodificar valor de %q: %w", entries[i].Key, err)
		}
		entries[i].Value = string(decoded)
	}
	return entries, nil
}

func (c *Client) do(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("construir request a Consul: %w", err)
	}
	if c.token != "" {
		req.Header.Set("X-Consul-Token", c.token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("llamada a Consul %s %s: %w", method, path, err)
	}
	return resp, nil
}

func ensureOK(resp *http.Response, action string) error {
	defer resp.Body.Close()
	if err := statusError(resp, action); err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

func statusError(resp *http.Response, action string) error {
	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		return nil
	}
	detail, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return fmt.Errorf("Consul %s: status %d: %s", action, resp.StatusCode, strings.TrimSpace(string(detail)))
}

func normalizeBaseURL(addr string) string {
	addr = strings.TrimSpace(addr)
	if !strings.Contains(addr, "://") {
		addr = "http://" + addr
	}
	return strings.TrimRight(addr, "/")
}

func hostPort(addr string) string {
	addr = strings.TrimSpace(addr)
	if parsed, err := url.Parse(addr); err == nil && parsed.Host != "" {
		return parsed.Host
	}
	addr = strings.TrimPrefix(addr, "http://")
	addr = strings.TrimPrefix(addr, "https://")
	return strings.TrimRight(addr, "/")
}
