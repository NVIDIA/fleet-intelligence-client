// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"syscall"
	"testing"
)

// The file holds every stored API key, so a failed write must not be able
// to leave a truncated or half-written config behind.
func TestSaveReplacesFileAtomically(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	var cfg Config
	if err := cfg.AddProfile("prod", Profile{APIURL: "https://prod.example.com", APIKey: "prod-key"}); err != nil {
		t.Fatalf("add prod failed: %v", err)
	}
	if err := Save(cfg); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	if err := cfg.AddProfile("dev", Profile{APIURL: "https://dev.example.com", APIKey: "dev-key"}); err != nil {
		t.Fatalf("add dev failed: %v", err)
	}
	if err := Save(cfg); err != nil {
		t.Fatalf("second save failed: %v", err)
	}

	// The rename must leave the directory holding exactly the config file: no
	// temp file with credentials in it survives a successful write.
	entries, err := os.ReadDir(filepath.Join(homeDir, ".config", "nvfleetint"))
	if err != nil {
		t.Fatalf("read dir failed: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != fileName {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Fatalf("unexpected directory contents: %v", names)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if len(got.Profiles) != 2 {
		t.Fatalf("expected both profiles to survive the rewrite, got %#v", got)
	}
}

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

func TestConfigWriteDirectoryFailuresPreserveData(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T, home string, cfg Config)
	}{
		{"lock directory", func(t *testing.T, home string, cfg Config) {
			blocker := filepath.Join(home, ".config")
			original := []byte("existing data")
			if err := os.WriteFile(blocker, original, fileMode); err != nil {
				t.Fatalf("write blocker: %v", err)
			}
			if err := Save(cfg); err == nil {
				t.Error("Save succeeded with a blocked config directory")
			}
			called := false
			if _, err := Edit(func(*Config) error { called = true; return nil }); err == nil {
				t.Error("Edit succeeded with a blocked config directory")
			}
			if called {
				t.Error("Edit invoked callback after lock directory failure")
			}
			assertBlockerUnchanged(t, blocker, original)
		}},
		{"resolved target directory", func(t *testing.T, home string, cfg Config) {
			path, err := Path()
			if err != nil {
				t.Fatalf("path: %v", err)
			}
			if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
				t.Fatalf("mkdir config: %v", err)
			}
			blocker := filepath.Join(home, "blocker")
			original := []byte("existing data")
			if err := os.WriteFile(blocker, original, fileMode); err != nil {
				t.Fatalf("write blocker: %v", err)
			}
			target := filepath.Join(blocker, "child", fileName)
			if err := os.Symlink(target, path); err != nil {
				t.Fatalf("symlink: %v", err)
			}
			// Compare the complete tree so neither a temporary file nor any other
			// new file containing the profile key can be left behind.
			tree := func() []string {
				var paths []string
				err := filepath.WalkDir(home, func(path string, _ os.DirEntry, err error) error {
					if err != nil {
						return err
					}
					paths = append(paths, path)
					return nil
				})
				if err != nil {
					t.Fatalf("walk home: %v", err)
				}
				return paths
			}
			before := tree()
			if err := Save(cfg); err == nil {
				t.Error("Save succeeded with a blocked symlink target directory")
			}
			info, err := os.Lstat(path)
			if err != nil || info.Mode()&os.ModeSymlink == 0 {
				t.Fatalf("config path is not a symlink: info=%v err=%v", info, err)
			}
			if got, err := os.Readlink(path); err != nil || got != target {
				t.Errorf("symlink target: got %q, want %q, err=%v", got, target, err)
			}
			assertBlockerUnchanged(t, blocker, original)
			if _, err := os.Lstat(path + ".lock"); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("lock remains or cannot be inspected: %v", err)
			}
			if after := tree(); !reflect.DeepEqual(after, before) {
				t.Errorf("home tree changed: got %v, want %v", after, before)
			}
		}},
		{"saveToPath directory", func(t *testing.T, home string, cfg Config) {
			blocker := filepath.Join(home, "blocker")
			original := []byte("existing data")
			if err := os.WriteFile(blocker, original, fileMode); err != nil {
				t.Fatalf("write blocker: %v", err)
			}
			if err := saveToPath(filepath.Join(blocker, "child", fileName), cfg); err == nil {
				t.Error("saveToPath succeeded with a blocked directory")
			}
			assertBlockerUnchanged(t, blocker, original)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			clearEnv(t)
			cfg := Config{CurrentProfile: "dev", Profiles: map[string]Profile{
				"dev": {APIURL: "https://fleet.example.com", APIKey: "distinct-test-key"},
			}}
			tt.run(t, home, cfg)
		})
	}
}

func TestWriteAndCloseFailuresPreserveCredentials(t *testing.T) {
	tests := []struct {
		name        string
		closeBefore bool
		operation   string
	}{
		{name: "chmod on closed file", closeBefore: true, operation: "chmod"},
		{name: "write on read-only file", operation: "write"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "credentials")
			original := []byte("api_key: original-test-credential\n")
			if err := os.WriteFile(path, original, fileMode); err != nil {
				t.Fatalf("write original credentials: %v", err)
			}
			file, err := os.Open(path)
			if err != nil {
				t.Fatalf("open credentials: %v", err)
			}
			t.Cleanup(func() { _ = file.Close() })
			if tt.closeBefore {
				if err := file.Close(); err != nil {
					t.Fatalf("close credentials: %v", err)
				}
			}

			err = writeAndClose(file, []byte("api_key: replacement-test-credential\n"))
			if err == nil {
				t.Fatal("writeAndClose succeeded, want an error")
			}
			if tt.closeBefore && !errors.Is(err, os.ErrClosed) {
				t.Errorf("error: got %v, want os.ErrClosed", err)
			}
			var pathErr *os.PathError
			if !errors.As(err, &pathErr) || pathErr.Op != tt.operation {
				t.Errorf("error: got %v, want %s operation", err, tt.operation)
			}
			if _, err := file.Stat(); !errors.Is(err, os.ErrClosed) {
				t.Errorf("file remains open or cannot be checked: %v", err)
			}
			got, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(got, original) {
				t.Errorf("credentials changed: got %q, want %q, err=%v", got, original, err)
			}
		})
	}
}

func TestWriteAndClosePipeSyncError(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("requires Linux pipe sync semantics; not executed on this platform")
	}

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create pipe: %v", err)
	}
	t.Cleanup(func() { _ = reader.Close() })
	t.Cleanup(func() { _ = writer.Close() })

	payload := []byte("api_key: distinctive-pipe-test-credential\n")
	err = writeAndClose(writer, payload)
	var pathErr *os.PathError
	if !errors.As(err, &pathErr) || pathErr.Op != "sync" || !errors.Is(err, syscall.EINVAL) {
		t.Errorf("writeAndClose error: got %v, want sync error wrapping EINVAL", err)
	}
	if _, err := writer.Stat(); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("pipe write end remains open or cannot be checked: %v", err)
	}
	got, err := io.ReadAll(reader)
	if err != nil || !bytes.Equal(got, payload) {
		t.Errorf("pipe payload: got %q, want %q, err=%v", got, payload, err)
	}
}

func TestAtomicConfigWritePreservesOriginalWhenTempPathTooLong(t *testing.T) {
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
