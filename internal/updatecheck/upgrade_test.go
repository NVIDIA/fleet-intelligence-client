// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package updatecheck

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNewUpgradePlan(t *testing.T) {
	plan, err := Checker{}.NewUpgradePlan("1.0.0", Release{Version: "v1.2.0"})
	if err != nil {
		t.Fatalf("NewUpgradePlan failed: %v", err)
	}

	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable failed: %v", err)
	}
	if resolved, err := filepath.EvalSymlinks(executable); err == nil {
		executable = resolved
	}
	if plan.ExecutablePath != executable {
		t.Fatalf("ExecutablePath = %q, want %q", plan.ExecutablePath, executable)
	}
	if plan.InstallDir != filepath.Dir(executable) {
		t.Fatalf("InstallDir = %q, want %q", plan.InstallDir, filepath.Dir(executable))
	}

	// The installer must be pinned to the resolved tag, not the moving `latest`
	// permalink, so the script and the archive come from one release.
	if !strings.Contains(plan.InstallerURL, "/download/v1.2.0/") {
		t.Fatalf("installer URL is not pinned to the tag: %q", plan.InstallerURL)
	}
	if !strings.HasSuffix(plan.InstallerURL, installerName()) {
		t.Fatalf("installer URL does not name this platform's installer: %q", plan.InstallerURL)
	}
}

func TestUpgradePlanSummary(t *testing.T) {
	plan, err := Checker{}.NewUpgradePlan("1.0.0", Release{Version: "v1.2.0"})
	if err != nil {
		t.Fatalf("NewUpgradePlan failed: %v", err)
	}

	summary := plan.Summary()
	for _, want := range []string{"nvfleetint v1.0.0 -> v1.2.0", plan.InstallDir, "checksum and code signature"} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary missing %q: %q", want, summary)
		}
	}
}

// Verifies the installer URL follows the checker's base, so a plan built from a
// test server does not reach github.com.
func TestNewUpgradePlanFollowsCheckerBase(t *testing.T) {
	plan, err := Checker{ReleasesURL: "https://example.test/releases"}.
		NewUpgradePlan("1.0.0", Release{Version: "v1.2.0"})
	if err != nil {
		t.Fatalf("NewUpgradePlan failed: %v", err)
	}
	if want := "https://example.test/releases/download/v1.2.0/" + installerName(); plan.InstallerURL != want {
		t.Fatalf("InstallerURL = %q, want %q", plan.InstallerURL, want)
	}
}

// Verifies a release that ships no installer is caught before the confirmation
// prompt, and says why. Releases published before the installers were added —
// v0.1.0 and v0.2.0 — are exactly this case: the tag resolves, but there is no
// install.sh to run.
func TestCheckInstallerAvailable(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		wantErr string
		wantIs  error
	}{
		{name: "published", status: http.StatusOK},
		{name: "no installer", status: http.StatusNotFound, wantErr: "predates the installer", wantIs: ErrInstallerUnavailable},
		{name: "unexpected status", status: http.StatusInternalServerError, wantErr: "unexpected status 500"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var gotMethod string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod = r.Method
				w.WriteHeader(test.status)
			}))
			defer server.Close()

			plan := UpgradePlan{
				Release:      Release{Version: "v0.2.0", URL: "https://example.test/tag/v0.2.0"},
				InstallerURL: server.URL + "/download/v0.2.0/" + installerName(),
			}

			err := plan.CheckInstallerAvailable(context.Background())
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("CheckInstallerAvailable failed: %v", err)
				}
				// The check must not download the script, only ask whether it exists.
				if gotMethod != http.MethodHead {
					t.Fatalf("expected a HEAD request, got %s", gotMethod)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("expected an error containing %q, got %v", test.wantErr, err)
			}
			if test.wantIs != nil {
				if !errors.Is(err, test.wantIs) {
					t.Fatalf("expected %v, got %v", test.wantIs, err)
				}
				// The user has to be left somewhere to go.
				if !strings.Contains(err.Error(), "https://example.test/tag/v0.2.0") {
					t.Fatalf("error does not point at the release page: %v", err)
				}
			}
		})
	}
}

func TestReleasePageURLFallsBackToReleasePage(t *testing.T) {
	plan := UpgradePlan{Release: Release{Version: "v0.2.0"}}
	want := releasesPage + tagSegment + "v0.2.0"
	if got := plan.releasePageURL(); got != want {
		t.Fatalf("releasePageURL() = %q, want %q", got, want)
	}
}

// Verifies a read-only install directory is refused before anything downloads.
func TestCheckWritable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory permissions are not enforced the same way on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}

	writable := UpgradePlan{InstallDir: t.TempDir()}
	if err := writable.CheckWritable(); err != nil {
		t.Fatalf("expected a writable temp dir to pass: %v", err)
	}

	readOnly := t.TempDir()
	if err := os.Chmod(readOnly, 0o500); err != nil {
		t.Fatalf("chmod failed: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(readOnly, 0o700) })

	err := UpgradePlan{InstallDir: readOnly}.CheckWritable()
	if err == nil {
		t.Fatal("expected a read-only install dir to be refused")
	}
	// The error has to leave the user somewhere to go.
	if !strings.Contains(err.Error(), "not writable") || !strings.Contains(err.Error(), "upgrade manually") {
		t.Fatalf("unhelpful error: %v", err)
	}
}

// Verifies CheckWritable leaves nothing behind in the install directory.
func TestCheckWritableLeavesNoFiles(t *testing.T) {
	dir := t.TempDir()
	if err := (UpgradePlan{InstallDir: dir}).CheckWritable(); err != nil {
		t.Fatalf("CheckWritable failed: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("CheckWritable left %d file(s) behind", len(entries))
	}
}

// Verifies the installer is downloaded and run with the release and install
// directory pinned, using a stand-in script that records its own arguments.
func TestUpgradePlanRun(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in installer is a shell script")
	}

	var requestedPath string
	installDir := t.TempDir()
	marker := filepath.Join(installDir, "installer-ran")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		_, _ = w.Write([]byte("#!/usr/bin/env bash\necho \"installer args: $*\"\necho ran > " + marker + "\n"))
	}))
	defer server.Close()

	plan := UpgradePlan{
		CurrentVersion: "1.0.0",
		Release:        Release{Version: "v1.2.0"},
		ExecutablePath: filepath.Join(installDir, "nvfleetint"),
		InstallDir:     installDir,
		InstallerURL:   server.URL + "/download/v1.2.0/install.sh",
	}

	var progress strings.Builder
	if err := plan.Run(context.Background(), &progress); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if requestedPath != "/download/v1.2.0/install.sh" {
		t.Fatalf("unexpected installer path %q", requestedPath)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("the installer did not run: %v", err)
	}
	// The installer must be told exactly which release to fetch and where to put
	// it; letting it resolve `latest` itself would reintroduce the race the
	// pinned URL exists to close.
	for _, want := range []string{"--version v1.2.0", "--install-dir " + installDir} {
		if !strings.Contains(progress.String(), want) {
			t.Fatalf("installer not invoked with %q: %q", want, progress.String())
		}
	}
}

// Verifies a failing installer surfaces as an error rather than a false success.
func TestUpgradePlanRunInstallerFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in installer is a shell script")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("#!/usr/bin/env bash\necho 'checksum verification failed' >&2\nexit 1\n"))
	}))
	defer server.Close()

	plan := UpgradePlan{
		Release:        Release{Version: "v1.2.0"},
		ExecutablePath: filepath.Join(t.TempDir(), "nvfleetint"),
		InstallDir:     t.TempDir(),
		InstallerURL:   server.URL,
	}

	var progress strings.Builder
	err := plan.Run(context.Background(), &progress)
	if err == nil || !strings.Contains(err.Error(), "installer failed") {
		t.Fatalf("expected an installer failure, got %v", err)
	}
	// The installer's own diagnosis has to reach the user.
	if !strings.Contains(progress.String(), "checksum verification failed") {
		t.Fatalf("installer output was swallowed: %q", progress.String())
	}
}

func TestDownloadInstallerErrors(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{name: "not found", status: http.StatusNotFound, body: "nope", wantErr: "unexpected status 404"},
		{name: "empty", status: http.StatusOK, wantErr: "it is empty"},
		{name: "oversized", status: http.StatusOK, body: strings.Repeat("x", maxInstallerBytes+1), wantErr: "larger than"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()

			if _, err := downloadInstaller(context.Background(), server.URL); err == nil ||
				!strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("expected error containing %q, got %v", test.wantErr, err)
			}
		})
	}
}

func TestRestoreWindowsBinaryReplacesFailedExecutable(t *testing.T) {
	dir := t.TempDir()
	backup := filepath.Join(dir, "nvfleetint.exe.old")
	executable := filepath.Join(dir, "nvfleetint.exe")

	if err := os.WriteFile(backup, []byte("old binary"), 0o600); err != nil {
		t.Fatalf("write backup failed: %v", err)
	}
	if err := os.WriteFile(executable, []byte("failed replacement"), 0o600); err != nil {
		t.Fatalf("write executable failed: %v", err)
	}

	if err := restoreWindowsBinary(backup, executable); err != nil {
		t.Fatalf("restoreWindowsBinary failed: %v", err)
	}

	got, err := os.ReadFile(executable)
	if err != nil {
		t.Fatalf("read restored executable failed: %v", err)
	}
	if string(got) != "old binary" {
		t.Fatalf("restored executable = %q, want old binary", got)
	}
	if _, err := os.Stat(backup); !os.IsNotExist(err) {
		t.Fatalf("backup still exists after restore: %v", err)
	}
}

func TestRestoreWindowsBinaryReportsRemovalFailure(t *testing.T) {
	dir := t.TempDir()
	backup := filepath.Join(dir, "nvfleetint.exe.old")
	executable := filepath.Join(dir, "nvfleetint.exe")

	if err := os.WriteFile(backup, []byte("old binary"), 0o600); err != nil {
		t.Fatalf("write backup failed: %v", err)
	}
	if err := os.Mkdir(executable, 0o700); err != nil {
		t.Fatalf("mkdir executable failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(executable, "child"), []byte("replacement"), 0o600); err != nil {
		t.Fatalf("write child failed: %v", err)
	}

	err := restoreWindowsBinary(backup, executable)
	if err == nil || !strings.Contains(err.Error(), "failed replacement executable") {
		t.Fatalf("expected removal failure, got %v", err)
	}
	if _, statErr := os.Stat(backup); statErr != nil {
		t.Fatalf("backup should remain after failed restore: %v", statErr)
	}
}

func TestManualUpgradeCommand(t *testing.T) {
	tests := []struct {
		name string
		goos string
		want string
	}{
		{
			name: "windows",
			goos: "windows",
			want: "irm " + releasesPage + "/latest/download/install.ps1 | iex",
		},
		{
			name: "unix",
			goos: "linux",
			want: "curl -fsSL " + releasesPage + "/latest/download/install.sh | bash",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := manualUpgradeCommand(tt.goos); got != tt.want {
				t.Fatalf("unexpected manual upgrade command: got %q, want %q", got, tt.want)
			}
		})
	}

	if got := ManualUpgradeCommand(); got != manualUpgradeCommand(runtime.GOOS) {
		t.Fatalf("manual command does not match runtime platform: %q", got)
	}
}
