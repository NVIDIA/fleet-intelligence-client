// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestEditAtomicRenameFailureCleansUpCredentials(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	clearEnv(t)
	initial := Config{CurrentProfile: "dev", Profiles: map[string]Profile{
		"dev": {APIURL: "https://fleet.example.com", APIKey: "original-key"},
	}}
	if err := Save(initial); err != nil {
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
	backup := path + ".backup"
	edited, err := Edit(func(cfg *Config) error {
		if err := cfg.UpdateProfile("dev", Profile{APIURL: "https://new.example.com", APIKey: "replacement-key"}); err != nil {
			t.Fatalf("update profile: %v", err)
		}
		if err := os.Rename(path, backup); err != nil {
			t.Fatalf("back up config: %v", err)
		}
		if err := os.Mkdir(path, dirMode); err != nil {
			t.Fatalf("create destination directory: %v", err)
		}
		return nil
	})
	var renameErr *os.LinkError
	if !errors.As(err, &renameErr) || renameErr.Op != "rename" || renameErr.New != path {
		t.Fatalf("Edit error: got %v, want rename failure at %q", err, path)
	}
	if !reflect.DeepEqual(edited, Config{}) {
		t.Errorf("Edit config: got %#v, want zero Config", edited)
	}
	after, err := os.ReadFile(backup)
	if err != nil || !bytes.Equal(after, before) {
		t.Errorf("backup changed: got %q, want %q, err=%v", after, before, err)
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		t.Errorf("destination is not a directory: info=%v err=%v", info, err)
	}
	if _, err := os.Lstat(path + ".lock"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("lock remains or cannot be inspected: %v", err)
	}
	temps, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".config-*.yaml"))
	if err != nil || len(temps) != 0 {
		t.Errorf("temporary config files remain or cannot be inspected: %v, err=%v", temps, err)
	}
}
