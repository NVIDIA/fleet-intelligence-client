// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package release

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdutil"
	"github.com/NVIDIA/fleet-intelligence-client/internal/updatecheck"
)

// Verifies the command confirms first, then upgrades to the release the check
// resolved.
func TestUpgradeCommand(t *testing.T) {
	setVersion(t, "1.0.0")
	server, _ := releaseServer(t, "v1.2.0")
	setUpdateChecker(t, server.URL)
	recorder := captureUpgrades(t, nil)

	stdout, stderr := runCLIWithInput(t, "y\n", "upgrade")

	if len(recorder.plans) != 1 {
		t.Fatalf("expected exactly one upgrade, got %d", len(recorder.plans))
	}
	if recorder.plans[0].Release.Version != "v1.2.0" || recorder.plans[0].CurrentVersion != "1.0.0" {
		t.Fatalf("unexpected plan %#v", recorder.plans[0])
	}
	if !strings.Contains(stdout, "Upgraded v1.0.0 -> v1.2.0") {
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
func TestUpgradeCommandDeclined(t *testing.T) {
	setVersion(t, "1.0.0")
	server, _ := releaseServer(t, "v1.2.0")
	setUpdateChecker(t, server.URL)
	recorder := captureUpgrades(t, nil)

	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetIn(strings.NewReader("n\n"))
	cmd.SetArgs([]string{"upgrade"})

	err := cmd.Execute()
	if !errors.Is(err, cmdutil.ErrAborted) {
		t.Fatalf("expected ErrAborted, got %v", err)
	}
	if len(recorder.plans) != 0 {
		t.Fatalf("expected no upgrade after declining, got %d", len(recorder.plans))
	}
}

// Verifies --yes skips the prompt, which is what a CI job needs.
func TestUpgradeCommandYes(t *testing.T) {
	setVersion(t, "1.0.0")
	server, _ := releaseServer(t, "v1.2.0")
	setUpdateChecker(t, server.URL)
	recorder := captureUpgrades(t, nil)

	_, stderr := runCLI(t, "upgrade", "--yes")

	if len(recorder.plans) != 1 {
		t.Fatalf("expected exactly one upgrade, got %d", len(recorder.plans))
	}
	if strings.Contains(stderr, "Are you sure?") {
		t.Fatalf("--yes should not prompt: %q", stderr)
	}
}

// Verifies an up-to-date build reports so and upgrades nothing.
func TestUpgradeCommandAlreadyCurrent(t *testing.T) {
	setVersion(t, "1.2.0")
	server, _ := releaseServer(t, "v1.2.0")
	setUpdateChecker(t, server.URL)
	recorder := captureUpgrades(t, nil)

	stdout, _ := runCLI(t, "upgrade")

	if len(recorder.plans) != 0 {
		t.Fatalf("expected no upgrade, got %d", len(recorder.plans))
	}
	if !strings.Contains(stdout, "already the newest release") {
		t.Fatalf("unexpected output: %q", stdout)
	}
}

// Verifies --version installs exactly the release named, including an older one:
// pinning a known-good build is the reason to name a version.
func TestUpgradeCommandVersion(t *testing.T) {
	tests := []struct {
		name      string
		requested string
	}{
		{name: "older release", requested: "v1.0.0"},
		// The "v" is optional, since `version` prints the bare number.
		{name: "without the v prefix", requested: "1.0.0"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setVersion(t, "1.1.0")
			server, _ := releaseServer(t, "v1.2.0", "v1.1.0", "v1.0.0")
			setUpdateChecker(t, server.URL)
			recorder := captureUpgrades(t, nil)

			stdout, _ := runCLI(t, "upgrade", "--version", test.requested, "--yes")

			if len(recorder.plans) != 1 {
				t.Fatalf("expected exactly one upgrade, got %d", len(recorder.plans))
			}
			// The newest release must not be substituted for the one asked for.
			if got := recorder.plans[0].Release.Version; got != "v1.0.0" {
				t.Fatalf("upgraded to %q, want v1.0.0", got)
			}
			if !strings.Contains(stdout, "Upgraded v1.1.0 -> v1.0.0") {
				t.Fatalf("stdout missing the result: %q", stdout)
			}
		})
	}
}

// Verifies a version that was never published fails with something the user can
// act on, before anything is downloaded.
func TestUpgradeCommandVersionNotFound(t *testing.T) {
	setVersion(t, "1.0.0")
	server, _ := releaseServer(t, "v1.2.0")
	setUpdateChecker(t, server.URL)
	recorder := captureUpgrades(t, nil)

	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"upgrade", "--version", "v9.9.9", "--yes"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected an unpublished version to fail")
	}
	if !errors.Is(err, updatecheck.ErrReleaseNotFound) {
		t.Fatalf("expected ErrReleaseNotFound, got %v", err)
	}
	for _, want := range []string{"v9.9.9", "is not published", "releases"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error missing %q: %v", want, err)
		}
	}
	if len(recorder.plans) != 0 {
		t.Fatalf("expected no upgrade for a missing release, got %d", len(recorder.plans))
	}
}

// Verifies a malformed --version is rejected without a release lookup.
func TestUpgradeCommandVersionInvalid(t *testing.T) {
	for _, requested := range []string{"latest", "1.x.0", ""} {
		setVersion(t, "1.0.0")
		server, requests := releaseServer(t, "v1.2.0")
		setUpdateChecker(t, server.URL)
		recorder := captureUpgrades(t, nil)

		cmd := newRootCmd()
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"upgrade", "--version", requested, "--yes"})

		if err := cmd.Execute(); err == nil {
			t.Fatalf("expected --version %q to fail", requested)
		}
		// An empty --version means "no version was named", so it falls back to
		// the latest release rather than erroring; every other value is refused
		// before the network is touched.
		if requested != "" && requests.Load() != 0 {
			t.Fatalf("--version %q reached the network: %d requests", requested, requests.Load())
		}
		if len(recorder.plans) != 0 {
			t.Fatalf("expected no upgrade for --version %q, got %d", requested, len(recorder.plans))
		}
	}
}

// Verifies naming the version already running upgrades nothing, and says so
// differently from being on the newest release — the user named a version, and
// telling them about a newer one they did not ask for would be a non sequitur.
func TestUpgradeCommandVersionAlreadyInstalled(t *testing.T) {
	setVersion(t, "1.1.0")
	server, _ := releaseServer(t, "v1.2.0", "v1.1.0")
	setUpdateChecker(t, server.URL)
	recorder := captureUpgrades(t, nil)

	stdout, _ := runCLI(t, "upgrade", "--version", "1.1.0", "--yes")

	if len(recorder.plans) != 0 {
		t.Fatalf("expected no upgrade, got %d", len(recorder.plans))
	}
	if !strings.Contains(stdout, "nvfleetint v1.1.0 is already installed") {
		t.Fatalf("unexpected output: %q", stdout)
	}
}

// Verifies a release that exists but ships no installer fails *before* the
// confirmation prompt. Releases published before install.sh existed are exactly
// this case, and asking the user to confirm an upgrade that cannot run is worse
// than refusing it outright.
func TestUpgradeCommandVersionWithoutInstaller(t *testing.T) {
	setVersion(t, "1.0.0")
	server, _ := releaseServerWithout(t, []string{"v0.2.0"}, "v1.2.0")
	setUpdateChecker(t, server.URL)
	recorder := captureUpgrades(t, nil)

	cmd := newRootCmd()
	var stderr bytes.Buffer
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&stderr)
	// Stdin would answer "y" if it were ever read; it must not be.
	cmd.SetIn(strings.NewReader("y\n"))
	cmd.SetArgs([]string{"upgrade", "--version", "v0.2.0"})

	err := cmd.Execute()
	if !errors.Is(err, updatecheck.ErrInstallerUnavailable) {
		t.Fatalf("expected ErrInstallerUnavailable, got %v", err)
	}
	if !strings.Contains(err.Error(), "v0.2.0") {
		t.Fatalf("error does not name the release: %v", err)
	}
	if strings.Contains(stderr.String(), "Are you sure?") {
		t.Fatalf("the user was asked to confirm an impossible upgrade: %q", stderr.String())
	}
	if len(recorder.plans) != 0 {
		t.Fatalf("expected no upgrade, got %d", len(recorder.plans))
	}
}

// Verifies a failed upgrade is reported. The passive check swallows failures;
// an explicit upgrade must not, or the user believes they were upgraded.
func TestUpgradeCommandFailureIsReported(t *testing.T) {
	setVersion(t, "1.0.0")
	server, _ := releaseServer(t, "v1.2.0")
	setUpdateChecker(t, server.URL)
	captureUpgrades(t, errors.New("the installer failed: checksum mismatch"))

	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"upgrade", "--yes"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected the installer failure to surface, got %v", err)
	}
}

// Verifies a lookup failure fails the command rather than being swallowed.
func TestUpgradeCommandLookupFailure(t *testing.T) {
	setVersion(t, "1.0.0")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	setUpdateChecker(t, server.URL)
	recorder := captureUpgrades(t, nil)

	for _, args := range [][]string{{"upgrade", "--yes"}, {"upgrade", "--version", "v1.2.0", "--yes"}} {
		cmd := newRootCmd()
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs(args)

		if err := cmd.Execute(); err == nil {
			t.Fatalf("expected a failed lookup to fail %v", args)
		}
	}
	if len(recorder.plans) != 0 {
		t.Fatalf("expected no upgrade after a failed lookup, got %d", len(recorder.plans))
	}
}

// Verifies a locally built binary is refused without even a lookup: it has no
// release version to upgrade from.
func TestUpgradeCommandRejectsDevBuild(t *testing.T) {
	for _, args := range [][]string{{"upgrade", "--yes"}, {"upgrade", "--version", "v1.2.0", "--yes"}} {
		setVersion(t, "dev")
		server, requests := releaseServer(t, "v9.9.9")
		setUpdateChecker(t, server.URL)
		recorder := captureUpgrades(t, nil)

		cmd := newRootCmd()
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs(args)

		err := cmd.Execute()
		if err == nil || !strings.Contains(err.Error(), "cannot upgrade a locally built binary") {
			t.Fatalf("expected %v to be refused, got %v", args, err)
		}
		if requests.Load() != 0 {
			t.Fatalf("a dev build must not even look up a release, got %d", requests.Load())
		}
		if len(recorder.plans) != 0 {
			t.Fatalf("expected no upgrade, got %d", len(recorder.plans))
		}
	}
}

// Verifies the upgrade keeps stdout parseable under -o json.
func TestUpgradeCommandJSON(t *testing.T) {
	setVersion(t, "1.0.0")
	server, _ := releaseServer(t, "v1.2.0")
	setUpdateChecker(t, server.URL)
	captureUpgrades(t, nil)

	stdout, _ := runCLI(t, "upgrade", "--yes", "-o", "json")

	var got upgradeOutput
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode upgrade JSON failed: %v -- %q", err, stdout)
	}
	if !got.Upgraded || got.From != "1.0.0" || got.To != "v1.2.0" {
		t.Fatalf("unexpected upgrade JSON: %#v", got)
	}
}

// Verifies an invalid output format is rejected, like every other command.
func TestUpgradeCommandRejectsInvalidOutput(t *testing.T) {
	setVersion(t, "1.0.0")
	recorder := captureUpgrades(t, nil)

	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"upgrade", "-o", "yaml", "--yes"})

	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "invalid output") {
		t.Fatalf("expected an invalid output error, got %v", err)
	}
	if len(recorder.plans) != 0 {
		t.Fatalf("expected no upgrade, got %d", len(recorder.plans))
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
