// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
)

func TestRound2ConfigLockNameTooLong(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("requires Linux or macOS filename limits")
	}

	dir := t.TempDir()
	name := strings.Repeat("a", 255)
	path := filepath.Join(dir, name)
	initial := []byte("distinctive original config contents\n")
	if err := os.WriteFile(path, initial, 0600); err != nil {
		if errors.Is(err, syscall.ENAMETOOLONG) {
			t.Skipf("filesystem does not support 255-byte filenames: %v", err)
		}
		t.Fatalf("write original config: %v", err)
	}

	called := false
	err := withConfigLock(path, func() error {
		called = true
		return nil
	})
	var pathErr *os.PathError
	if !errors.As(err, &pathErr) {
		t.Fatalf("lock error = %v, want *os.PathError", err)
	}
	if pathErr.Op != "open" || pathErr.Path != path+".lock" || !errors.Is(pathErr.Err, syscall.ENAMETOOLONG) {
		t.Errorf("lock error = %#v, want open of %q with ENAMETOOLONG", pathErr, path+".lock")
	}
	if called {
		t.Error("callback ran after lock acquisition failed")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read original config: %v", err)
	}
	if !bytes.Equal(got, initial) {
		t.Errorf("config contents = %q, want %q", got, initial)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read config directory: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != name || !entries[0].Type().IsRegular() {
		t.Errorf("directory entries = %v, want only original regular file %q", entries, name)
	}
}

func TestRound2ConfigAtomicTempPathTooLong(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("requires Linux PATH_MAX behavior")
	}

	const dirLength = 4088
	dir := t.TempDir()
	if !filepath.IsAbs(dir) || len(dir) > dirLength-2 {
		t.Fatalf("cannot construct %d-byte directory from temporary directory %q", dirLength, dir)
	}
	for len(dir) < dirLength {
		remaining := dirLength - len(dir)
		componentLength := remaining - 1 // Account for the path separator.
		if componentLength > 200 {
			componentLength = 200
			if remaining-componentLength-1 == 1 {
				componentLength--
			}
		}
		if componentLength < 1 {
			t.Fatalf("cannot finish %d-byte directory path from %q", dirLength, dir)
		}
		dir = filepath.Join(dir, strings.Repeat("d", componentLength))
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatalf("create fixture directory of length %d: %v", len(dir), err)
		}
	}
	if len(dir) != dirLength {
		t.Fatalf("fixture directory length = %d, want %d", len(dir), dirLength)
	}

	target := filepath.Join(dir, "c")
	original := []byte("original credentials: secret-original\n")
	if err := os.WriteFile(target, original, 0600); err != nil {
		t.Fatalf("create original config: %v", err)
	}

	err := writeFileAtomic(target, []byte("replacement credentials: secret-new\n"))
	var pathErr *os.PathError
	if !errors.As(err, &pathErr) {
		t.Fatalf("atomic write error = %v, want *os.PathError", err)
	}
	if pathErr.Op != "open" || filepath.Dir(pathErr.Path) != dir ||
		!strings.HasPrefix(filepath.Base(pathErr.Path), ".config-") ||
		!strings.HasSuffix(pathErr.Path, ".yaml") || !errors.Is(pathErr, syscall.ENAMETOOLONG) {
		t.Errorf("atomic write error = %#v, want temporary-file open with ENAMETOOLONG in %q", pathErr, dir)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read original config: %v", err)
	}
	if !bytes.Equal(got, original) {
		t.Errorf("config contents = %q, want %q", got, original)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read config directory: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "c" || !entries[0].Type().IsRegular() {
		t.Errorf("directory entries = %v, want only original regular file %q", entries, "c")
	}
}
