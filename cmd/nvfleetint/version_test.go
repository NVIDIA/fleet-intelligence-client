// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/updatecheck"
)

func TestVersionCommand(t *testing.T) {
	server, _ := releaseServer(t, "v1.2.0")
	setUpdateChecker(t, server.URL)

	stdout, _ := runCLI(t, "version")

	if !strings.Contains(stdout, "nvfleetint ") {
		t.Fatalf("version output missing binary name: %q", stdout)
	}
}

func TestVersionCommandJSON(t *testing.T) {
	server, _ := releaseServer(t, "v1.2.0")
	setUpdateChecker(t, server.URL)

	stdout, _ := runCLI(t, "version", "--output", "json")

	var got versionOutput
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode version JSON failed: %v", err)
	}
	if got.Name != "nvfleetint" || got.Version == "" {
		t.Fatalf("unexpected version JSON: %#v", got)
	}
}

// Verifies a newer published release produces an upgrade notice on stderr, so
// stdout stays exactly what it was before the check existed.
func TestVersionCommandUpdateAvailable(t *testing.T) {
	setVersion(t, "1.0.0")
	server, requests := releaseServer(t, "v1.2.0")
	setUpdateChecker(t, server.URL)

	stdout, stderr := runCLI(t, "version")

	if requests.Load() != 1 {
		t.Fatalf("expected exactly one release lookup, got %d", requests.Load())
	}
	if strings.Contains(stdout, "Update available") {
		t.Fatalf("update notice leaked into stdout: %q", stdout)
	}
	for _, want := range []string{"Update available: v1.2.0 (current v1.0.0)", "Release notes:", "Upgrade:"} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("stderr missing %q: %q", want, stderr)
		}
	}
}

// Verifies a current build reports the check in JSON without claiming an update.
func TestVersionCommandUpToDateJSON(t *testing.T) {
	setVersion(t, "1.2.0")
	server, _ := releaseServer(t, "v1.2.0")
	setUpdateChecker(t, server.URL)

	stdout, stderr := runCLI(t, "version", "--output", "json")

	var got versionOutput
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode version JSON failed: %v", err)
	}
	if got.UpdateCheck == nil {
		t.Fatal("expected updateCheck in JSON output")
	}
	if got.UpdateCheck.Available {
		t.Fatalf("expected no update for a current build: %#v", got.UpdateCheck)
	}
	if got.UpdateCheck.Version != "v1.2.0" {
		t.Fatalf("unexpected latest version: %#v", got.UpdateCheck)
	}
	if stderr != "" {
		t.Fatalf("expected no notice for a current build: %q", stderr)
	}
}

// Verifies a failed lookup never fails the command, but does tell the user the
// CLI could not check whether it is current.
func TestVersionCommandLookupFailureIsReported(t *testing.T) {
	setVersion(t, "1.0.0")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	setUpdateChecker(t, server.URL)

	stdout, stderr := runCLI(t, "version", "--output", "json")

	var got versionOutput
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode version JSON failed: %v", err)
	}
	if got.UpdateCheck != nil {
		t.Fatalf("expected no updateCheck after a failed lookup: %#v", got.UpdateCheck)
	}
	if !strings.Contains(stderr, "Can't check if nvfleetint is up to date. Check your internet connection and try again.") {
		t.Fatalf("expected a non-fatal lookup warning, got %q", stderr)
	}
}

// Verifies the version command performs the release lookup for release builds,
// but skips locally built binaries whose versions cannot be compared.
func TestVersionCommandAlwaysLooksUpLatestRelease(t *testing.T) {
	tests := []struct {
		name       string
		version    string
		wantChecks int64
		wantStderr string
	}{
		{name: "release build", version: "1.0.0", wantChecks: 1, wantStderr: "Update available"},
		{name: "dev build skips", version: "dev"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setVersion(t, test.version)
			server, requests := releaseServer(t, "v9.9.9")
			setUpdateChecker(t, server.URL)

			_, stderr := runCLI(t, "version")

			if requests.Load() != test.wantChecks {
				t.Fatalf("expected %d release lookups, got %d", test.wantChecks, requests.Load())
			}
			if test.wantStderr != "" && !strings.Contains(stderr, test.wantStderr) {
				t.Fatalf("stderr missing %q: %q", test.wantStderr, stderr)
			}
			if test.wantStderr == "" && stderr != "" {
				t.Fatalf("expected no stderr, got %q", stderr)
			}
		})
	}
}

// Runs a command with stdin attached, and returns stdout and stderr.
func runCLIWithInput(t *testing.T, input string, args ...string) (string, string) {
	t.Helper()

	cmd := newRootCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetIn(strings.NewReader(input))
	cmd.SetArgs(args)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("%v failed: %v", args, err)
	}
	return stdout.String(), stderr.String()
}

// Runs a command and returns its stdout and stderr.
func runCLI(t *testing.T, args ...string) (string, string) {
	t.Helper()

	cmd := newRootCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("%v failed: %v", args, err)
	}
	return stdout.String(), stderr.String()
}

// Serves the release pages the CLI reads the way github.com does: the
// latest-release permalink as a redirect to latest's own page, a page per
// published release, and that release's installer asset — 404 for anything
// else, which is how an unpublished version is refused. It counts the requests
// it received.
//
// A tag listed in withoutInstaller has a release page but no installer asset,
// which is what the releases published before install.sh existed look like.
func releaseServer(t *testing.T, latest string, alsoPublished ...string) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	return releaseServerWithout(t, nil, latest, alsoPublished...)
}

func releaseServerWithout(t *testing.T, withoutInstaller []string, latest string, alsoPublished ...string) (*httptest.Server, *atomic.Int64) {
	t.Helper()

	published := map[string]bool{latest: true}
	for _, tag := range alsoPublished {
		published[tag] = true
	}
	installerMissing := map[string]bool{}
	for _, tag := range withoutInstaller {
		published[tag] = true
		installerMissing[tag] = true
	}

	var requests atomic.Int64
	var base string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		switch {
		case strings.Contains(r.URL.Path, "/download/"):
			// /download/<tag>/<installer>
			_, rest, _ := strings.Cut(r.URL.Path, "/download/")
			tag, _, _ := strings.Cut(rest, "/")
			if !published[tag] || installerMissing[tag] {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = w.Write([]byte("#!/usr/bin/env bash\n"))
		case strings.Contains(r.URL.Path, "/tag/"):
			_, tag, _ := strings.Cut(r.URL.Path, "/tag/")
			if !published[tag] {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = w.Write([]byte("release page"))
		default:
			w.Header().Set("Location", fmt.Sprintf("%s/releases/tag/%s", base, latest))
			w.WriteHeader(http.StatusFound)
		}
	}))
	base = server.URL
	t.Cleanup(server.Close)
	return server, &requests
}

// Points the release lookup at a test server.
func setUpdateChecker(t *testing.T, url string) {
	t.Helper()

	previous := updateChecker
	updateChecker = updatecheck.Checker{ReleasesURL: url}
	t.Cleanup(func() { updateChecker = previous })
}

// Overrides the build version stamped in by ldflags.
func setVersion(t *testing.T, value string) {
	t.Helper()

	previous := version
	version = value
	t.Cleanup(func() { version = previous })
}
