// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"bytes"
	"errors"
	"os"
	"reflect"
	"testing"
)

func TestEditInvalidProfileNamesPreserveFile(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config) error
	}{
		{"add", func(cfg *Config) error { return cfg.AddProfile("bad name", Profile{APIKey: "replacement"}) }},
		{"update", func(cfg *Config) error { return cfg.UpdateProfile("bad name", Profile{APIKey: "replacement"}) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			clearEnv(t)
			want := Config{CurrentProfile: "dev", Profiles: map[string]Profile{
				"dev": {APIURL: "https://fleet.example.com", APIKey: "original-key"},
			}}
			if err := Save(want); err != nil {
				t.Fatalf("save: %v", err)
			}
			path, err := Path()
			if err != nil {
				t.Fatalf("path: %v", err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read original: %v", err)
			}
			wantErr := ValidateProfileName("bad name")
			_, err = Edit(tt.mutate)
			if err == nil || wantErr == nil || err.Error() != wantErr.Error() {
				t.Fatalf("edit error: got %v, want %v", err, wantErr)
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read after edit: %v", err)
			}
			if !bytes.Equal(after, before) {
				t.Fatalf("config bytes changed: got %q, want %q", after, before)
			}
			loaded, err := Load()
			if err != nil || !reflect.DeepEqual(loaded, want) {
				t.Fatalf("config after edit: got %#v, want %#v, err=%v", loaded, want, err)
			}
			if _, err := os.Lstat(path + ".lock"); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("lock remains or cannot be inspected: %v", err)
			}
		})
	}
}
