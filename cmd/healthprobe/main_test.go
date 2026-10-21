package main

import (
	"testing"
)

func TestProbeURL_DefaultPort(t *testing.T) {
	t.Setenv("APP_PORT", "")
	if got := probeURL(); got != "http://127.0.0.1:8002/health/ready" {
		t.Errorf("default: %s", got)
	}
}

func TestProbeURL_CustomPort(t *testing.T) {
	t.Setenv("APP_PORT", "9000")
	if got := probeURL(); got != "http://127.0.0.1:9000/health/ready" {
		t.Errorf("APP_PORT personalizado: %s", got)
	}
}
