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

func TestReleaseLookupsRejectInvalidRequests(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	lookups := []struct {
		name string
		call func(context.Context, Checker) (Release, error)
	}{
		{"Latest", func(ctx context.Context, checker Checker) (Release, error) { return checker.Latest(ctx) }},
		{"Release", func(ctx context.Context, checker Checker) (Release, error) { return checker.Release(ctx, "1.2.3") }},
	}
	cases := []struct {
		name      string
		ctx       context.Context
		url       string
		cancelled bool
	}{
		{"malformed URL", context.Background(), "://bad", false},
		{"cancelled context", cancelled, server.URL, true},
	}
	for _, lookup := range lookups {
		for _, tc := range cases {
			t.Run(lookup.name+"/"+tc.name, func(t *testing.T) {
				got, err := lookup.call(tc.ctx, Checker{ReleasesURL: tc.url, HTTPClient: server.Client()})
				if err == nil {
					t.Error("expected an error")
				}
				if tc.cancelled && !errors.Is(err, context.Canceled) {
					t.Errorf("error = %v, want context.Canceled", err)
				}
				if got != (Release{}) {
					t.Errorf("release = %+v, want zero value", got)
				}
				if got := requests.Load(); got != 0 {
					t.Errorf("server received %d requests, want none", got)
				}
			})
		}
	}
}

func TestCheckRejectsUnparseablePublishedTag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "/releases/tag/v1.x.0")
		w.WriteHeader(http.StatusFound)
	}))
	defer server.Close()

	got, err := Check(context.Background(), Checker{ReleasesURL: server.URL + "/releases", HTTPClient: server.Client()}, "1.0.0")
	if err == nil {
		t.Error("Check succeeded with an unparseable published tag")
	}
	if got != (Result{}) {
		t.Errorf("result = %+v, want zero value", got)
	}
}

func TestLatestRejectsMalformedLocation(t *testing.T) {
	requests := make(chan struct{ method, path string }, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- struct{ method, path string }{r.Method, r.URL.Path}
		w.Header().Set("Location", "/releases/tag/%zz")
		w.WriteHeader(http.StatusMultipleChoices)
	}))
	defer server.Close()

	got, err := (Checker{ReleasesURL: server.URL + "/releases", HTTPClient: server.Client()}).Latest(context.Background())
	if err == nil || !strings.Contains(err.Error(), "release lookup failed") {
		t.Errorf("Latest error = %v, want release lookup failed", err)
	}
	if got != (Release{}) {
		t.Errorf("release = %+v, want zero value", got)
	}
	select {
	case request := <-requests:
		if request.method != http.MethodHead || request.path != "/releases/latest" {
			t.Errorf("request = %s %s, want HEAD /releases/latest", request.method, request.path)
		}
	default:
		t.Error("server did not receive the latest-release request")
	}
}

func TestNumericPrereleaseIdentifiersWithDifferentFormattingCompareEqual(t *testing.T) {
	for _, versions := range [][2]string{
		{"1.0.0-01", "1.0.0-1"},
		{"1.0.0-1", "1.0.0-01"},
	} {
		t.Run(versions[0]+"_and_"+versions[1], func(t *testing.T) {
			same, err := IsSame(versions[0], versions[1])
			if err != nil || !same {
				t.Errorf("IsSame(%q, %q) = %t, %v; want true, nil", versions[0], versions[1], same, err)
			}
			newer, err := IsNewer(versions[0], versions[1])
			if err != nil || newer {
				t.Errorf("IsNewer(%q, %q) = %t, %v; want false, nil", versions[0], versions[1], newer, err)
			}
		})
	}
}

func TestIsNewerVersionBoundaries(t *testing.T) {
	cases := []struct {
		name, candidate, current string
		want                     bool
		wantError                bool
	}{
		{"numeric before alphanumeric", "1.0.0-1", "1.0.0-alpha", false, false},
		{"alphanumeric after numeric", "1.0.0-alpha", "1.0.0-1", true, false},
		{"alpha before beta", "1.0.0-alpha", "1.0.0-beta", false, false},
		{"beta after alpha", "1.0.0-beta", "1.0.0-alpha", true, false},
		{"invalid candidate", "invalid", "1.0.0", false, true},
		{"invalid current", "1.0.0", "invalid", false, true},
		{"numeric component out of range", "99999999999999999999.0.0", "1.0.0", false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := IsNewer(tc.candidate, tc.current)
			if (err != nil) != tc.wantError {
				t.Errorf("error = %v, want error = %t", err, tc.wantError)
			}
			if got != tc.want {
				t.Errorf("IsNewer(%q, %q) = %t, want %t", tc.candidate, tc.current, got, tc.want)
			}
		})
	}
}

func TestDisplayVersionBoundaries(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{" \t\n", "unknown"},
		{" V1.2.3 ", "V1.2.3"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			if got := DisplayVersion(tc.input); got != tc.want {
				t.Errorf("DisplayVersion(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

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
