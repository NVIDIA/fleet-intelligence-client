// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestEditConcurrentAddsPreserveProfiles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	clearEnv(t)

	const count = 20
	var wg sync.WaitGroup
	errs := make(chan error, count)
	for i := range count {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := fmt.Sprintf("p%d", i)
			_, err := Edit(func(cfg *Config) error {
				return cfg.AddProfile(name, Profile{APIKey: "key-" + name})
			})
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("edit failed: %v", err)
		}
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if len(cfg.Profiles) != count {
		t.Fatalf("expected %d profiles, got %d: %#v", count, len(cfg.Profiles), cfg)
	}
}

func TestEditRecoversStaleLockThroughRelativeSymlink(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
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
	wantMode := os.FileMode(fileMode)
	if runtime.GOOS == "windows" {
		// Windows exposes writable regular files as 0666; Chmod applies only
		// the owner write bit (see os.fileStat.mode in types_windows.go).
		wantMode = 0666
	}
	if info.Mode().Perm() != wantMode {
		t.Fatalf("target permissions: got %o, want %o", info.Mode().Perm(), wantMode)
	}
	if _, err := os.Lstat(lockPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lock remains or cannot be inspected: %v", err)
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
		file, err := acquireLock(lockPath, lockTimeout)
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

func TestConfigLockRejectsOverlongFilename(t *testing.T) {
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
