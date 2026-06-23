package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/config"
)

// Verifies config is converted to an SDK client
func TestNewConfiguredClientBuildsSDKClient(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if err := config.Save(config.Config{APIURL: "https://fleet.example.com", ServiceKey: "test-key"}); err != nil {
		t.Fatalf("save config failed: %v", err)
	}

	client, err := newConfiguredClient()
	if err != nil {
		t.Fatalf("new configured client failed: %v", err)
	}
	if client.BaseURL() != "https://fleet.example.com" {
		t.Fatalf("unexpected base URL: %q", client.BaseURL())
	}
	if !client.ServiceKeyConfigured() {
		t.Fatal("expected service key to be configured")
	}
}

// Verifies missing auth config fails clearly
func TestNewConfiguredClientRequiresServiceKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if err := config.Save(config.Config{APIURL: "https://fleet.example.com"}); err != nil {
		t.Fatalf("save config failed: %v", err)
	}

	_, err := newConfiguredClient()
	if err == nil {
		t.Fatal("expected service key error")
	}
	if !strings.Contains(err.Error(), "service key is not configured") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Verifies config load failures are returned
func TestNewConfiguredClientReturnsLoadError(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	path := filepath.Join(homeDir, ".config", "nvfleetctl", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(path, []byte("api_url\n"), 0o600); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	_, err := newConfiguredClient()
	if err == nil {
		t.Fatal("expected config load error")
	}
}
