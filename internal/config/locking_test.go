// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A live competing writer must time out without invoking the edit callback or
// deleting the other process's lock/configuration. Only this test's temp files
// are involved; a short acquisition deadline keeps this test fast.
func TestConfigLockTimeoutPreservesCompetingWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	original := []byte("original configuration\n")
	owner := []byte("another live writer owns this lock\n")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".lock", owner, 0600); err != nil {
		t.Fatal(err)
	}
	called := false
	err := withConfigLockTimeout(path, 2*lockPollInterval, func() error {
		called = true
		return os.WriteFile(path, []byte("unexpected edit"), 0600)
	})
	if err == nil || !strings.Contains(err.Error(), "config is locked by another nvfleetint process") || !strings.Contains(err.Error(), path+".lock") {
		t.Fatalf("lock error = %v, want competing-writer error naming the lock", err)
	}
	if called {
		t.Error("edit callback ran while another writer owned the lock")
	}
	for name, want := range map[string][]byte{path: original, path + ".lock": owner} {
		got, readErr := os.ReadFile(name)
		if readErr != nil || !bytes.Equal(got, want) {
			t.Errorf("file %q changed: contents=%q error=%v", name, got, readErr)
		}
	}
}

func TestEditReadErrorDoesNotMutateAndRemovesLock(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	clearEnv(t)
	path, err := Path()
	if err != nil {
		t.Fatalf("path: %v", err)
	}
	if err := os.MkdirAll(path, dirMode); err != nil {
		t.Fatalf("mkdir config path: %v", err)
	}
	called := false
	_, err = Edit(func(*Config) error {
		called = true
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "read config") || !strings.Contains(err.Error(), path) {
		t.Fatalf("expected read-config error identifying %q, got %v", path, err)
	}
	if called {
		t.Fatal("mutation callback called after read error")
	}
	if _, err := os.Lstat(path + ".lock"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lock remains or cannot be inspected: %v", err)
	}
}
