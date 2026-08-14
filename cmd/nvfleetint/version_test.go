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
	stdout, _ := runVersion(t, "version")

	if !strings.Contains(stdout, "nvfleetint ") {
		t.Fatalf("version output missing binary name: %q", stdout)
	}
}

func TestVersionCommandJSON(t *testing.T) {
	stdout, _ := runVersion(t, "version", "--output", "json")

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

	stdout, stderr := runVersion(t, "version")

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

	stdout, stderr := runVersion(t, "version", "--output", "json")

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

// Verifies a failed lookup never fails the command and never prints a notice —
// being offline is not an error for `nvfleetint version`.
func TestVersionCommandLookupFailureIsSilent(t *testing.T) {
	setVersion(t, "1.0.0")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	setUpdateChecker(t, server.URL)

	stdout, stderr := runVersion(t, "version", "--output", "json")

	var got versionOutput
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode version JSON failed: %v", err)
	}
	if got.UpdateCheck != nil {
		t.Fatalf("expected no updateCheck after a failed lookup: %#v", got.UpdateCheck)
	}
	if stderr != "" {
		t.Fatalf("expected no output after a failed lookup: %q", stderr)
	}
}

// Verifies every documented way of turning the check off keeps the CLI offline.
func TestVersionCommandSkipsLookup(t *testing.T) {
	tests := []struct {
		name    string
		version string
		args    []string
		env     string
	}{
		{name: "flag", version: "1.0.0", args: []string{"version", "--check-update=false"}},
		{name: "env", version: "1.0.0", args: []string{"version"}, env: "1"},
		{name: "dev build", version: "dev", args: []string{"version"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setVersion(t, test.version)
			t.Setenv(updatecheck.EnvDisable, test.env)
			server, requests := releaseServer(t, "v9.9.9")
			setUpdateChecker(t, server.URL)

			_, stderr := runVersion(t, test.args...)

			if requests.Load() != 0 {
				t.Fatalf("expected no release lookup, got %d", requests.Load())
			}
			if stderr != "" {
				t.Fatalf("expected no notice: %q", stderr)
			}
		})
	}
}

// Runs the version command and returns its stdout and stderr.
func runVersion(t *testing.T, args ...string) (string, string) {
	t.Helper()

	cmd := newRootCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("version command failed: %v", err)
	}
	return stdout.String(), stderr.String()
}

// Serves the latest-release permalink the way github.com does — a redirect to
// the release page — and counts the requests it received.
func releaseServer(t *testing.T, tag string) (*httptest.Server, *atomic.Int64) {
	t.Helper()

	var requests atomic.Int64
	var base string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Location", fmt.Sprintf("%s/releases/tag/%s", base, tag))
		w.WriteHeader(http.StatusFound)
	}))
	base = server.URL
	t.Cleanup(server.Close)
	return server, &requests
}

// Points the version command's release lookup at a test server.
func setUpdateChecker(t *testing.T, url string) {
	t.Helper()

	previous := updateChecker
	updateChecker = updatecheck.Checker{URL: url}
	t.Cleanup(func() { updateChecker = previous })
}

// Overrides the build version stamped in by ldflags.
func setVersion(t *testing.T, value string) {
	t.Helper()

	previous := version
	version = value
	t.Cleanup(func() { version = previous })
}
