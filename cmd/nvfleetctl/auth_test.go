package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/config"
)

const (
	serviceKey = "test-key"
	apiUrl     = "https://fleet.example.com"
)

func TestAuthLoginSavesKeyAndDefaultAPIURL(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"auth", "login", "--key", serviceKey})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("login failed: %v", err)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if cfg.APIURL != config.DefaultAPIURL {
		t.Fatalf("unexpected API URL: %q", cfg.APIURL)
	}
	if cfg.ServiceKey != serviceKey {
		t.Fatalf("unexpected service key: %q", cfg.ServiceKey)
	}
}

func TestAuthLoginSavesExplicitAPIURL(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"auth", "login", "--key", serviceKey, "--api-url", apiUrl})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("login failed: %v", err)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if cfg.APIURL != apiUrl {
		t.Fatalf("unexpected API URL: %q", cfg.APIURL)
	}
	if cfg.ServiceKey != serviceKey {
		t.Fatalf("unexpected service key: %q", cfg.ServiceKey)
	}
}

func TestAuthLoginRequiresKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"auth", "login"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected missing key error")
	}
	if !strings.Contains(err.Error(), "service key is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuthLoginRejectsInvalidAPIURL(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"auth", "login", "--key", serviceKey, "--api-url", "example.com"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected invalid URL error")
	}
	if !strings.Contains(err.Error(), "absolute http or https URL is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuthLogoutClearsKeyAndPreservesAPIURL(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if err := config.Save(config.Config{APIURL: apiUrl, ServiceKey: serviceKey}); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"auth", "logout"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("logout failed: %v", err)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if cfg.APIURL != apiUrl {
		t.Fatalf("unexpected API URL: %q", cfg.APIURL)
	}
	if cfg.ServiceKey != "" {
		t.Fatalf("expected cleared service key, got %q", cfg.ServiceKey)
	}
}

func TestAuthStatusReportsConfiguredKeyAndDoesNotPrintSecret(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if err := config.Save(config.Config{APIURL: apiUrl, ServiceKey: serviceKey}); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"auth", "status"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("status failed: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "API URL: "+apiUrl) {
		t.Fatalf("status missing API URL: %q", got)
	}
	if !strings.Contains(got, "Service key: configured") {
		t.Fatalf("status missing service key state: %q", got)
	}
	if !strings.Contains(got, "Connection: not checked") {
		t.Fatalf("status missing connection state: %q", got)
	}
	if strings.Contains(got, serviceKey) {
		t.Fatalf("status printed secret: %q", got)
	}
}

func TestAuthStatusWithoutConfigExitsZero(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"auth", "status"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("status failed: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "API URL: "+config.DefaultAPIURL) {
		t.Fatalf("status missing default API URL: %q", got)
	}
	if !strings.Contains(got, "Service key: not configured") {
		t.Fatalf("status missing not configured state: %q", got)
	}
}

func TestAuthLogoutWithoutConfigCreatesDefaultConfig(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"auth", "logout"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("logout failed: %v", err)
	}

	path := filepath.Join(homeDir, ".config", "nvfleetctl", "config.yaml")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected config file after logout: %v", err)
	}
}
