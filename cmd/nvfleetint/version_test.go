// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/clihelpers"
	"github.com/NVIDIA/fleet-intelligence-client/internal/updatecheck"
)

func TestVersionCommand(t *testing.T) {
	server, _ := releaseServer(t, "v1.2.0")
	setUpdateChecker(t, server.URL)

	stdout, _ := runVersion(t, "version")

	if !strings.Contains(stdout, "nvfleetint ") {
		t.Fatalf("version output missing binary name: %q", stdout)
	}
}

func TestVersionCommandJSON(t *testing.T) {
	server, _ := releaseServer(t, "v1.2.0")
	setUpdateChecker(t, server.URL)

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

// Verifies a failed lookup never fails the command, but does tell the user the
// CLI could not check whether it is current.
func TestVersionCommandLookupFailureIsReported(t *testing.T) {
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

			_, stderr := runVersion(t, "version")

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

// Verifies --upgrade confirms first, then runs the upgrade against the release
// the check resolved.
func TestVersionCommandUpgrade(t *testing.T) {
	setVersion(t, "1.0.0")
	server, _ := releaseServer(t, "v1.2.0")
	setUpdateChecker(t, server.URL)
	recorder := captureUpgrades(t, nil)

	stdout, stderr := runVersionWithInput(t, "y\n", "version", "--upgrade")

	if len(recorder.plans) != 1 {
		t.Fatalf("expected exactly one upgrade, got %d", len(recorder.plans))
	}
	if recorder.plans[0].Release.Version != "v1.2.0" || recorder.plans[0].CurrentVersion != "1.0.0" {
		t.Fatalf("unexpected plan %#v", recorder.plans[0])
	}
	if !strings.Contains(stdout, "Upgraded 1.0.0 -> v1.2.0") {
		t.Fatalf("stdout missing the result: %q", stdout)
	}
	// The prompt has to say what is about to happen, on stderr.
	for _, want := range []string{"nvfleetint v1.0.0 -> v1.2.0", "Are you sure?"} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("stderr missing %q: %q", want, stderr)
		}
	}
}

// Verifies declining the prompt aborts without touching the binary.
func TestVersionCommandUpgradeDeclined(t *testing.T) {
	setVersion(t, "1.0.0")
	server, _ := releaseServer(t, "v1.2.0")
	setUpdateChecker(t, server.URL)
	recorder := captureUpgrades(t, nil)

	cmd := newRootCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetIn(strings.NewReader("n\n"))
	cmd.SetArgs([]string{"version", "--upgrade"})

	err := cmd.Execute()
	if !errors.Is(err, clihelpers.ErrAborted) {
		t.Fatalf("expected ErrAborted, got %v", err)
	}
	if len(recorder.plans) != 0 {
		t.Fatalf("expected no upgrade after declining, got %d", len(recorder.plans))
	}
}

// Verifies --yes skips the prompt, which is what a CI job needs.
func TestVersionCommandUpgradeYes(t *testing.T) {
	setVersion(t, "1.0.0")
	server, _ := releaseServer(t, "v1.2.0")
	setUpdateChecker(t, server.URL)
	recorder := captureUpgrades(t, nil)

	_, stderr := runVersion(t, "version", "--upgrade", "--yes")

	if len(recorder.plans) != 1 {
		t.Fatalf("expected exactly one upgrade, got %d", len(recorder.plans))
	}
	if strings.Contains(stderr, "Are you sure?") {
		t.Fatalf("--yes should not prompt: %q", stderr)
	}
}

// Verifies an up-to-date build reports so and upgrades nothing.
func TestVersionCommandUpgradeAlreadyCurrent(t *testing.T) {
	setVersion(t, "1.2.0")
	server, _ := releaseServer(t, "v1.2.0")
	setUpdateChecker(t, server.URL)
	recorder := captureUpgrades(t, nil)

	stdout, _ := runVersion(t, "version", "--upgrade")

	if len(recorder.plans) != 0 {
		t.Fatalf("expected no upgrade, got %d", len(recorder.plans))
	}
	if !strings.Contains(stdout, "already the newest release") {
		t.Fatalf("unexpected output: %q", stdout)
	}
}

// Verifies a failed upgrade is reported. The passive check swallows failures;
// an explicit --upgrade must not, or the user believes they were upgraded.
func TestVersionCommandUpgradeFailureIsReported(t *testing.T) {
	setVersion(t, "1.0.0")
	server, _ := releaseServer(t, "v1.2.0")
	setUpdateChecker(t, server.URL)
	captureUpgrades(t, errors.New("the installer failed: checksum mismatch"))

	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"version", "--upgrade", "--yes"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected the installer failure to surface, got %v", err)
	}
}

// Verifies a lookup failure fails --upgrade rather than being swallowed.
func TestVersionCommandUpgradeLookupFailure(t *testing.T) {
	setVersion(t, "1.0.0")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	setUpdateChecker(t, server.URL)
	recorder := captureUpgrades(t, nil)

	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"version", "--upgrade", "--yes"})

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected a failed lookup to fail --upgrade")
	}
	if len(recorder.plans) != 0 {
		t.Fatalf("expected no upgrade after a failed lookup, got %d", len(recorder.plans))
	}
}

// Verifies the flag combinations that cannot mean anything are rejected.
func TestVersionCommandUpgradeRejectsFlags(t *testing.T) {
	tests := []struct {
		name    string
		version string
		args    []string
		wantErr string
	}{
		{
			name:    "dev build",
			version: "dev",
			args:    []string{"version", "--upgrade", "--yes"},
			wantErr: "cannot upgrade a locally built binary",
		},
		{
			name:    "yes without upgrade",
			version: "1.0.0",
			args:    []string{"version", "--yes"},
			wantErr: "--yes has no effect without --upgrade",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setVersion(t, test.version)
			server, requests := releaseServer(t, "v9.9.9")
			setUpdateChecker(t, server.URL)
			recorder := captureUpgrades(t, nil)

			cmd := newRootCmd()
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetArgs(test.args)

			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("expected error containing %q, got %v", test.wantErr, err)
			}
			if len(recorder.plans) != 0 {
				t.Fatalf("expected no upgrade, got %d", len(recorder.plans))
			}
			if test.name == "dev build" && requests.Load() != 0 {
				t.Fatalf("a dev build must not even look up a release, got %d", requests.Load())
			}
		})
	}
}

// Verifies --upgrade keeps stdout parseable under -o json.
func TestVersionCommandUpgradeJSON(t *testing.T) {
	setVersion(t, "1.0.0")
	server, _ := releaseServer(t, "v1.2.0")
	setUpdateChecker(t, server.URL)
	captureUpgrades(t, nil)

	stdout, _ := runVersion(t, "version", "--upgrade", "--yes", "-o", "json")

	var got upgradeOutput
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode upgrade JSON failed: %v -- %q", err, stdout)
	}
	if !got.Upgraded || got.From != "1.0.0" || got.To != "v1.2.0" {
		t.Fatalf("unexpected upgrade JSON: %#v", got)
	}
}

// upgradeRecorder collects the upgrades a command asked for.
type upgradeRecorder struct {
	plans []updatecheck.UpgradePlan
}

// Replaces the upgrade runner, recording the plans it is given and returning
// err instead of downloading and executing a real installer.
func captureUpgrades(t *testing.T, err error) *upgradeRecorder {
	t.Helper()

	recorder := &upgradeRecorder{}
	previous := upgradeRunner
	upgradeRunner = func(_ context.Context, plan updatecheck.UpgradePlan, _ io.Writer) error {
		recorder.plans = append(recorder.plans, plan)
		return err
	}
	t.Cleanup(func() { upgradeRunner = previous })
	return recorder
}

// Runs the version command with stdin attached, and returns stdout and stderr.
func runVersionWithInput(t *testing.T, input string, args ...string) (string, string) {
	t.Helper()

	cmd := newRootCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetIn(strings.NewReader(input))
	cmd.SetArgs(args)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("version command failed: %v", err)
	}
	return stdout.String(), stderr.String()
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
