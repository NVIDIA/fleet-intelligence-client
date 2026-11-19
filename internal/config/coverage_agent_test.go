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
	"time"
)

func TestEditRecoversStaleLockThroughRelativeSymlink(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	clearEnv(t)

	path, err := Path()
	if err != nil {
		t.Fatalf("path: %v", err)
	}
	target := filepath.Join(home, "dotfiles", "nvfleetint.yaml")
	if err := os.MkdirAll(filepath.Dir(target), dirMode); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	initial := []byte("current_profile: missing\nprofiles:\n  dev:\n    api_url: https://old.example.com\n    api_key: old-key\n")
	if err := os.WriteFile(target, initial, fileMode); err != nil {
		t.Fatalf("write target: %v", err)
	}
	relativeTarget, err := filepath.Rel(filepath.Dir(path), target)
	if err != nil {
		t.Fatalf("relative target: %v", err)
	}
	if err := os.Symlink(relativeTarget, path); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	lockPath := path + ".lock"
	if err := os.WriteFile(lockPath, []byte("stale"), lockMode); err != nil {
		t.Fatalf("write lock: %v", err)
	}
	stale := time.Now().Add(-lockStaleAfter - time.Minute)
	if err := os.Chtimes(lockPath, stale, stale); err != nil {
		t.Fatalf("age lock: %v", err)
	}

	want := Config{CurrentProfile: "dev", Profiles: map[string]Profile{
		"dev": {APIURL: "https://new.example.com", APIKey: "new-key"},
	}}
	got, err := Edit(func(cfg *Config) error {
		return cfg.UpdateProfile("dev", want.Profiles["dev"])
	})
	if err != nil {
		t.Fatalf("edit: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("edited config: got %#v, want %#v", got, want)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !reflect.DeepEqual(loaded, want) {
		t.Fatalf("reloaded config: got %#v, want %#v", loaded, want)
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("config path is not a symlink: info=%v err=%v", info, err)
	}
	targetConfig, err := loadFromPath(target)
	if err != nil || !reflect.DeepEqual(targetConfig, want) {
		t.Fatalf("target config: got %#v, want %#v, err=%v", targetConfig, want, err)
	}
	info, err = os.Stat(target)
	if err != nil {
		t.Fatalf("stat target: %v", err)
	}
	if info.Mode().Perm() != fileMode {
		t.Fatalf("target permissions: got %o, want %o", info.Mode().Perm(), fileMode)
	}
	if _, err := os.Lstat(lockPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lock remains or cannot be inspected: %v", err)
	}
}

func TestEditReadErrorDoesNotMutateAndRemovesLock(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
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

func TestEditAtomicRenameFailureCleansUpCredentials(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
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
			clearEnv(t)
			cfg := Config{CurrentProfile: "dev", Profiles: map[string]Profile{
				"dev": {APIURL: "https://fleet.example.com", APIKey: "distinct-test-key"},
			}}
			tt.run(t, home, cfg)
		})
	}
}

func assertBlockerUnchanged(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("blocking file changed: got %q, want %q, err=%v", got, want, err)
	}
}

func TestConfigFilesystemResolutionAndLockErrors(t *testing.T) {
	t.Run("atomic write path resolution", func(t *testing.T) {
		dir := t.TempDir()
		blocker := filepath.Join(dir, "blocker")
		original := []byte("distinctive existing data\n")
		if err := os.WriteFile(blocker, original, fileMode); err != nil {
			t.Fatalf("write blocker: %v", err)
		}
		path := filepath.Join(blocker, fileName)
		err := writeFileAtomic(path, []byte("api_key: temporary-test-key\n"))
		var pathErr *os.PathError
		if !errors.As(err, &pathErr) || pathErr.Op != "lstat" || pathErr.Path != path || errors.Is(err, os.ErrNotExist) {
			t.Errorf("writeFileAtomic error: got %v, want non-NotExist lstat error for %q", err, path)
		}
		assertBlockerUnchanged(t, blocker, original)
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read temporary directory: %v", err)
		}
		if len(entries) != 1 || entries[0].Name() != "blocker" {
			t.Errorf("unexpected files after atomic write: %v", entries)
		}
	})

	t.Run("lock directory absent", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "removed")
		lockPath := filepath.Join(dir, "config.lock")
		file, err := acquireLock(lockPath)
		if file != nil {
			_ = file.Close()
			t.Error("acquireLock returned a file for an absent directory")
		}
		if !errors.Is(err, os.ErrNotExist) {
			t.Errorf("acquireLock error: got %v, want os.ErrNotExist", err)
		}
		for _, path := range []string{dir, lockPath} {
			if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("path %q exists or cannot be inspected: %v", path, err)
			}
		}
	})
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

func TestEditRemoveProfilesWithNoCurrentSelection(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	clearEnv(t)

	survivor := Profile{APIURL: "https://survivor.example.com", APIKey: "survivor-key"}
	initial := Config{CurrentProfile: "first", Profiles: map[string]Profile{
		"first":    {APIURL: "https://first.example.com", APIKey: "first-key"},
		"second":   {APIURL: "https://second.example.com", APIKey: "second-key"},
		"survivor": survivor,
	}}
	if err := Save(initial); err != nil {
		t.Fatalf("save initial config: %v", err)
	}
	if _, err := Edit(func(cfg *Config) error { return cfg.RemoveProfile("first") }); err != nil {
		t.Fatalf("remove current profile: %v", err)
	}
	if _, err := Edit(func(cfg *Config) error {
		if cfg.CurrentProfile != "" {
			t.Fatalf("current profile before second removal: got %q, want empty", cfg.CurrentProfile)
		}
		return cfg.RemoveProfile("second")
	}); err != nil {
		t.Fatalf("remove second profile: %v", err)
	}

	want := Config{Profiles: map[string]Profile{"survivor": survivor}}
	loaded, err := Load()
	if err != nil || !reflect.DeepEqual(loaded, want) {
		t.Fatalf("config after removals: got %#v, want %#v, err=%v", loaded, want, err)
	}
	resolved, err := Resolve("")
	if err != nil {
		t.Fatalf("resolve without selection: %v", err)
	}
	if resolved.Profile != "" || resolved.APIKey != "" || resolved.APIURL != DefaultAPIURL ||
		resolved.APIURLSource != SourceDefault || resolved.APIKeySource != SourceDefault || !resolved.ProfilesConfigured {
		t.Fatalf("resolve without selection: unexpected result %#v", resolved)
	}

	added := Profile{APIURL: "https://new.example.com", APIKey: "new-key"}
	if _, err := Edit(func(cfg *Config) error { return cfg.AddProfile("new", added) }); err != nil {
		t.Fatalf("add profile: %v", err)
	}
	want = Config{CurrentProfile: "new", Profiles: map[string]Profile{
		"survivor": survivor,
		"new":      added,
	}}
	loaded, err = Load()
	if err != nil || !reflect.DeepEqual(loaded, want) {
		t.Fatalf("config after adding profile: got %#v, want %#v, err=%v", loaded, want, err)
	}
}
