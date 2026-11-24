// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestLoadInvalidYAMLFails(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	path := filepath.Join(homeDir, ".config", "nvfleetint", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(path, []byte("profiles: [oops\n"), fileMode); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	if _, err := Load(); err == nil {
		t.Fatal("expected invalid YAML to fail")
	} else if !strings.Contains(err.Error(), path) {
		t.Fatalf("expected error to include path %q, got %v", path, err)
	}
}

func TestConfigInvalidHomeErrorPropagation(t *testing.T) {
	homeVar := "HOME"
	switch runtime.GOOS {
	case "windows":
		homeVar = "USERPROFILE"
	case "plan9":
		homeVar = "home"
	}

	tests := []struct {
		name string
		home string
	}{
		{name: "whitespace", home: " \t\n"},
	}
	if runtime.GOOS != "android" && runtime.GOOS != "ios" {
		tests = append(tests, struct {
			name string
			home string
		}{name: "empty", home: ""})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(homeVar, tt.home)
			clearEnv(t)

			path, pathErr := Path()
			if path != "" || pathErr == nil {
				t.Fatalf("Path: got %q, %v; want empty path and error", path, pathErr)
			}
			if tt.name == "whitespace" && pathErr.Error() != "home directory is required" {
				t.Fatalf("Path error: got %q, want home directory is required", pathErr)
			}
			checkError := func(operation string, err error) {
				t.Helper()
				if err == nil || err.Error() != pathErr.Error() {
					t.Errorf("%s error: got %v, want %v", operation, err, pathErr)
				}
			}

			loaded, err := Load()
			checkError("Load", err)
			if !reflect.DeepEqual(loaded, Config{}) {
				t.Errorf("Load config: got %#v, want zero Config", loaded)
			}
			checkError("Save", Save(Config{CurrentProfile: "named"}))
			called := false
			edited, err := Edit(func(*Config) error {
				called = true
				return nil
			})
			checkError("Edit", err)
			if !reflect.DeepEqual(edited, Config{}) || called {
				t.Errorf("Edit: got config %#v and callback called=%t; want zero Config and no callback", edited, called)
			}

			t.Setenv(EnvAPIKey, "test-environment-key")
			resolved, err := Resolve("")
			if err != nil {
				t.Fatalf("Resolve from environment: %v", err)
			}
			if resolved.APIKey != "test-environment-key" || resolved.APIKeySource != SourceEnvironment ||
				resolved.APIURL != DefaultAPIURL || resolved.APIURLSource != SourceDefault {
				t.Errorf("Resolve from environment: unexpected credentials or sources: %#v", resolved)
			}
			checkError("Resolve ConfigError", resolved.ConfigError)
			named, err := Resolve("named")
			checkError("Resolve named", err)
			if !reflect.DeepEqual(named, Resolved{}) {
				t.Errorf("Resolve named: got %#v, want zero Resolved", named)
			}
		})
	}
}
