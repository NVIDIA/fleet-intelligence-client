// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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

func TestNewConfiguredClientUsesEnvFallback(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv(config.EnvAPIURL, "https://env-fleet.example.com")
	t.Setenv(config.EnvServiceKey, "env-test-key")

	client, err := newConfiguredClient()
	if err != nil {
		t.Fatalf("new configured client failed: %v", err)
	}
	if client.BaseURL() != "https://env-fleet.example.com" {
		t.Fatalf("unexpected base URL: %q", client.BaseURL())
	}
	if !client.ServiceKeyConfigured() {
		t.Fatal("expected service key to be configured")
	}
}

func TestNewConfiguredClientEnvOverridesConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv(config.EnvAPIURL, "https://env-fleet.example.com")
	t.Setenv(config.EnvServiceKey, "env-test-key")

	if err := config.Save(config.Config{APIURL: "https://file-fleet.example.com", ServiceKey: "file-test-key"}); err != nil {
		t.Fatalf("save config failed: %v", err)
	}

	client, err := newConfiguredClient()
	if err != nil {
		t.Fatalf("new configured client failed: %v", err)
	}
	if client.BaseURL() != "https://env-fleet.example.com" {
		t.Fatalf("unexpected base URL: %q", client.BaseURL())
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
