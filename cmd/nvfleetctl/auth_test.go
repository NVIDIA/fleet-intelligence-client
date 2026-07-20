// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/config"
)

const (
	serviceKey = "test-key"
	apiURL     = "https://fleet.example.com"
)

// newAuthStatusServer starts a test server that mimics GET /v1/auth/status,
// asserting the bearer token and replying with the given status and body.
func newAuthStatusServer(t *testing.T, statusCode int, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/auth/status" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+serviceKey {
			t.Errorf("unexpected auth header: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		_, _ = w.Write([]byte(body))
	}))
}

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
	cmd.SetArgs([]string{"auth", "login", "--key", serviceKey, "--api-url", apiURL})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("login failed: %v", err)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if cfg.APIURL != apiURL {
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

	if err := config.Save(config.Config{APIURL: apiURL, ServiceKey: serviceKey}); err != nil {
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
	if cfg.APIURL != apiURL {
		t.Fatalf("unexpected API URL: %q", cfg.APIURL)
	}
	if cfg.ServiceKey != "" {
		t.Fatalf("expected cleared service key, got %q", cfg.ServiceKey)
	}
}

func TestAuthStatusChecksConnectionAndDoesNotPrintSecret(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := newAuthStatusServer(t, http.StatusOK, `{"authenticated":true}`)
	defer server.Close()

	if err := config.Save(config.Config{APIURL: server.URL, ServiceKey: serviceKey}); err != nil {
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
	if !strings.Contains(got, "API URL: "+server.URL) {
		t.Fatalf("status missing API URL: %q", got)
	}
	if !strings.Contains(got, "Service key: configured") {
		t.Fatalf("status missing service key state: %q", got)
	}
	if !strings.Contains(got, "Connection: ok") {
		t.Fatalf("status missing connection state: %q", got)
	}
	if strings.Contains(got, serviceKey) {
		t.Fatalf("status printed secret: %q", got)
	}
}

func TestAuthStatusReportsUnauthorizedOnRejectedKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := newAuthStatusServer(t, http.StatusUnauthorized, `{"error":"unauthorized"}`)
	defer server.Close()

	if err := config.Save(config.Config{APIURL: server.URL, ServiceKey: serviceKey}); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"auth", "status"})

	// A rejected key is a reportable status, not a command failure.
	if err := cmd.Execute(); err != nil {
		t.Fatalf("status failed: %v", err)
	}

	if got := out.String(); !strings.Contains(got, "Connection: unauthorized") {
		t.Fatalf("status missing unauthorized state: %q", got)
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

func TestAuthStatusJSONUsesEnvFallback(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := newAuthStatusServer(t, http.StatusOK, `{"authenticated":true}`)
	defer server.Close()

	t.Setenv(config.EnvAPIURL, server.URL)
	t.Setenv(config.EnvServiceKey, serviceKey)

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"auth", "status", "--output", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("status failed: %v", err)
	}

	var got authStatusOutput
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode status JSON failed: %v", err)
	}
	if got.APIURL != server.URL || !got.ServiceKeyConfigured || got.Connection != "ok" {
		t.Fatalf("unexpected status JSON: %#v", got)
	}
	if strings.Contains(out.String(), serviceKey) {
		t.Fatalf("status printed secret: %q", out.String())
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
