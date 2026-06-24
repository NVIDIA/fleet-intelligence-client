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

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingConfigReturnsDefault(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if cfg.APIURL != DefaultAPIURL {
		t.Fatalf("unexpected default API URL: %q", cfg.APIURL)
	}
	if cfg.ServiceKey != "" {
		t.Fatalf("expected empty service key, got %q", cfg.ServiceKey)
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	want := Config{
		APIURL:     "https://fleet.example.com",
		ServiceKey: "test-key",
	}
	if err := Save(want); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if got != want {
		t.Fatalf("config mismatch: got %#v want %#v", got, want)
	}
}

func TestSaveUsesExpectedPathAndMode(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	if err := Save(Config{ServiceKey: "test-key"}); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	path, err := Path()
	if err != nil {
		t.Fatalf("path failed: %v", err)
	}

	wantPath := filepath.Join(homeDir, ".config", "nvfleetctl", "config.yaml")
	if path != wantPath {
		t.Fatalf("unexpected path: got %q want %q", path, wantPath)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}
	if gotMode := info.Mode().Perm(); gotMode != fileMode {
		t.Fatalf("unexpected config mode: got %v want %v", gotMode, os.FileMode(fileMode))
	}
}

func TestLoadMalformedYAMLFails(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	path, err := Path()
	if err != nil {
		t.Fatalf("path failed: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(path, []byte("api_url\n"), fileMode); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	if _, err := Load(); err == nil {
		t.Fatal("expected malformed config error")
	}
}
