// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package updatecheck

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

func TestInstallerRequestsRejectMalformedURL(t *testing.T) {
	const invalidURL = "://bad"
	plan := UpgradePlan{InstallerURL: invalidURL}
	for _, tc := range []struct {
		name string
		call func() error
	}{
		{"availability", func() error { return plan.CheckInstallerAvailable(context.Background()) }},
		{"download", func() error {
			_, err := downloadInstaller(context.Background(), invalidURL)
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			var parseErr *url.Error
			if !errors.As(err, &parseErr) {
				t.Fatalf("error = %v, want *url.Error from malformed URL", err)
			}
			if parseErr.Op != "parse" || parseErr.URL != invalidURL {
				t.Errorf("URL error = %+v, want parse failure for %q", parseErr, invalidURL)
			}
		})
	}
}

func TestInstallerRequestsHonorCancelledContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	const version = "v1.2.3"
	plan := UpgradePlan{InstallerURL: server.URL, Release: Release{Version: version}}
	if err := plan.CheckInstallerAvailable(ctx); !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), version) {
		t.Errorf("CheckInstallerAvailable error = %v, want cancellation identifying %s", err, version)
	}
	if _, err := downloadInstaller(ctx, server.URL); !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "could not download the installer") {
		t.Errorf("downloadInstaller error = %v, want cancellation identifying the download failure", err)
	}
}

func TestUpgradePlanRunTruncatedDownloadPreservesExecutable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "100")
		_, _ = io.WriteString(w, "short installer")
	}))
	defer server.Close()

	dir := t.TempDir()
	executable := filepath.Join(dir, "nvfleetint")
	original := []byte("original executable bytes")
	if err := os.WriteFile(executable, original, 0o700); err != nil {
		t.Fatal(err)
	}
	plan := UpgradePlan{InstallerURL: server.URL, ExecutablePath: executable, InstallDir: dir}
	var progress bytes.Buffer
	err := plan.Run(context.Background(), &progress)
	if !errors.Is(err, io.ErrUnexpectedEOF) || !strings.Contains(err.Error(), "could not download the installer") {
		t.Errorf("Run error = %v, want truncated download error", err)
	}
	got, err := os.ReadFile(executable)
	if err != nil {
		t.Fatalf("read executable after failed download: %v", err)
	}
	if !bytes.Equal(got, original) {
		t.Errorf("executable changed: got %q, want %q", got, original)
	}
	if progress.Len() != 0 {
		t.Errorf("installer progress = %q, want empty", progress.String())
	}
}

func TestUpgradePlanRunWindowsMoveFailure(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("requires Windows executable move behavior; not executed on this platform")
	}

	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = io.WriteString(w, "installer body")
	}))
	defer server.Close()

	dir := t.TempDir()
	executable := filepath.Join(dir, "missing.exe")
	plan := UpgradePlan{InstallerURL: server.URL, ExecutablePath: executable, InstallDir: dir}
	var progress bytes.Buffer
	err := plan.Run(context.Background(), &progress)
	if !errors.Is(err, os.ErrNotExist) || !strings.Contains(err.Error(), "could not move the running executable aside") {
		t.Errorf("Run error = %v, want missing executable move failure", err)
	}
	if got := requests.Load(); got != 1 {
		t.Errorf("installer requests = %d, want 1", got)
	}
	if progress.Len() != 0 {
		t.Errorf("progress = %q, want empty", progress.String())
	}
	for _, path := range []string{executable, executable + ".old"} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("stat %q error = %v, want not exist", path, err)
		}
	}
}

func TestRestoreWindowsBinaryMissingBackup(t *testing.T) {
	dir := t.TempDir()
	backup := filepath.Join(dir, "missing-backup")
	executable := filepath.Join(dir, "missing-executable")
	err := restoreWindowsBinary(backup, executable)
	if !errors.Is(err, os.ErrNotExist) || !strings.Contains(err.Error(), "could not restore the original executable") {
		t.Errorf("restoreWindowsBinary error = %v, want missing backup restoration error", err)
	}
	if _, err := os.Stat(executable); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("executable stat error = %v, want not exist", err)
	}
}

func TestUpgradeStagingAndWindowsMoveSafety(t *testing.T) {
	dir := t.TempDir()
	executable := filepath.Join(dir, "nvfleetint.exe")
	original := []byte("original executable bytes")
	plan := UpgradePlan{ExecutablePath: executable, InstallDir: dir, Release: Release{Version: "v1.2.3"}}

	if runtime.GOOS != "windows" {
		if err := os.WriteFile(executable, original, 0o700); err != nil {
			t.Fatal(err)
		}
		invalidTemp := filepath.Join(dir, "not-a-directory")
		if err := os.WriteFile(invalidTemp, []byte("file"), 0o600); err != nil {
			t.Fatal(err)
		}
		var requests atomic.Int64
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			requests.Add(1)
			_, _ = io.WriteString(w, "installer body")
		}))
		defer server.Close()
		plan.InstallerURL = server.URL
		t.Setenv("TMPDIR", invalidTemp)
		var progress bytes.Buffer
		err := plan.Run(context.Background(), &progress)
		if err == nil || !strings.Contains(err.Error(), "could not create a temporary directory") {
			t.Errorf("Run error = %v, want temporary directory creation failure", err)
		}
		if got := requests.Load(); got != 1 {
			t.Errorf("installer requests = %d, want 1", got)
		}
		got, err := os.ReadFile(executable)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, original) {
			t.Errorf("executable = %q, want %q", got, original)
		}
		if progress.Len() != 0 {
			t.Errorf("progress = %q, want empty", progress.String())
		}
		return
	}

	if _, err := plan.moveRunningBinaryAside(); err == nil || !strings.Contains(err.Error(), "could not move the running executable aside") {
		t.Errorf("move missing executable error = %v, want move failure", err)
	}
	backup := executable + ".old"
	if err := os.WriteFile(executable, original, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backup, []byte("stale backup"), 0o600); err != nil {
		t.Fatal(err)
	}
	moved, err := plan.moveRunningBinaryAside()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(executable); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("moved executable stat error = %v, want not exist", err)
	}
	got, err := os.ReadFile(backup)
	if err != nil || !bytes.Equal(got, original) {
		t.Errorf("backup = %q, error = %v, want %q", got, err, original)
	}
	if err := moved.restore(); err != nil {
		t.Fatal(err)
	}
	got, err = os.ReadFile(executable)
	if err != nil || !bytes.Equal(got, original) {
		t.Errorf("restored executable = %q, error = %v, want %q", got, err, original)
	}
	if err := os.WriteFile(executable, []byte("fresh executable"), 0o700); err != nil {
		t.Fatal(err)
	}
	moved, err = plan.moveRunningBinaryAside()
	if err != nil {
		t.Fatal(err)
	}
	moved.discard()
	if _, err := os.Stat(backup); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("discarded backup stat error = %v, want not exist", err)
	}

	script := filepath.Join(dir, "installer script.ps1")
	args := plan.installerCommand(context.Background(), script).Args
	want := []string{"powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", script, "-Version", plan.Release.Version, "-InstallDir", dir, "-NoModifyPath"}
	if len(args) != len(want) {
		t.Errorf("installer arguments = %q, want %q", args, want)
	} else {
		for i := range want {
			if args[i] != want[i] {
				t.Errorf("installer argument %d = %q, want %q", i, args[i], want[i])
			}
		}
	}
	if got := installerName(); got != "install.ps1" {
		t.Errorf("installerName() = %q, want install.ps1", got)
	}
}

func TestUpgradeRunWindowsRestoreFailure(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("requires Windows executable move and restore behavior")
	}

	dir := t.TempDir()
	executable := filepath.Join(dir, "nvfleetint.exe")
	original := []byte("original executable bytes")
	if err := os.WriteFile(executable, original, 0o700); err != nil {
		t.Fatalf("write original executable: %v", err)
	}

	// PowerShell single-quoted strings escape an apostrophe by doubling it.
	quotedPath := strings.ReplaceAll(executable, "'", "''")
	script := "$ErrorActionPreference = 'Stop'\n" +
		"$target = '" + quotedPath + "'\n" +
		"[void][IO.Directory]::CreateDirectory($target)\n" +
		"[IO.File]::WriteAllText([IO.Path]::Combine($target, 'child'), 'replacement')\n" +
		"exit 1\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, script)
	}))
	defer server.Close()

	plan := UpgradePlan{
		Release:        Release{Version: "v1.2.3"},
		ExecutablePath: executable,
		InstallDir:     dir,
		InstallerURL:   server.URL,
	}
	var progress bytes.Buffer
	err := plan.Run(context.Background(), &progress)
	if err == nil {
		t.Fatal("Run succeeded, want installer and restore failures")
	}
	for _, want := range []string{
		"the installer failed",
		"additionally failed to restore the original executable",
		"could not remove the failed replacement executable",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Run error = %v, want %q", err, want)
		}
	}

	backup, err := os.ReadFile(executable + ".old")
	if err != nil {
		t.Fatalf("read original backup: %v", err)
	}
	if !bytes.Equal(backup, original) {
		t.Errorf("backup = %q, want %q", backup, original)
	}
	info, err := os.Stat(executable)
	if err != nil {
		t.Fatalf("stat replacement directory: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("replacement at %q is not a directory", executable)
	}
	child, err := os.ReadFile(filepath.Join(executable, "child"))
	if err != nil {
		t.Fatalf("read replacement child: %v", err)
	}
	if string(child) != "replacement" {
		t.Errorf("replacement child = %q, want %q", child, "replacement")
	}
}
