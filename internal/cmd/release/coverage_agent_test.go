// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package release

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/spf13/cobra"
)

type failingVersionWriter struct {
	err error
}

func (w failingVersionWriter) Write([]byte) (int, error) {
	return 0, w.err
}

func TestVersionCommandInvalidOutputSkipsLookup(t *testing.T) {
	setVersion(t, "1.0.0")
	server, requests := releaseServer(t, "v1.2.0")
	setUpdateChecker(t, server.URL)

	cmd := newRootCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"version", "--output", "yaml"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), `invalid output "yaml": expected table or json`) {
		t.Fatalf("expected invalid output error with supported formats, got %v", err)
	}
	if got := stdout.String(); got != "" {
		t.Errorf("expected empty stdout, got %q", got)
	}
	if got := requests.Load(); got != 0 {
		t.Errorf("expected no release lookup, got %d requests", got)
	}
}

func TestVersionCommandJSONWriteFailurePrecedesLookupWarning(t *testing.T) {
	setVersion(t, "1.0.0")
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	setUpdateChecker(t, server.URL)

	writeErr := errors.New("version output write failed")
	cmd := newRootCmd()
	var stderr bytes.Buffer
	cmd.SetOut(failingVersionWriter{err: writeErr})
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"version", "--output", "json"})

	if err := cmd.Execute(); !errors.Is(err, writeErr) {
		t.Fatalf("expected write error %v, got %v", writeErr, err)
	}
	if got := requests.Load(); got != 1 {
		t.Errorf("expected one release lookup, got %d requests", got)
	}
	if got := stderr.String(); got != "" {
		t.Errorf("expected no lookup warning on stderr, got %q", got)
	}
}

func TestLatestReleaseCheckNilContext(t *testing.T) {
	server, requests := releaseServer(t, "v1.2.0")
	setUpdateChecker(t, server.URL)

	result, err := latestReleaseCheck(nil, BuildInfo{Version: "1.0.0"}) //nolint:staticcheck // SA1012: deliberately exercise the helper's nil-context fallback.
	if err != nil {
		t.Fatalf("latestReleaseCheck failed: %v", err)
	}
	if got := requests.Load(); got != 1 {
		t.Errorf("expected one release lookup, got %d requests", got)
	}
	if result == nil || result.Version != "v1.2.0" || !result.Available {
		t.Errorf("expected available release v1.2.0, got %#v", result)
	}
}

func TestUpgradeCommandInstallerFailures(t *testing.T) {
	tests := []struct {
		name       string
		headStatus int
		wantError  string
		wantGets   int64
	}{
		{
			name:       "installer availability fails",
			headStatus: http.StatusServiceUnavailable,
			wantError:  "could not reach the installer for v1.2.0: unexpected status 503",
			wantGets:   0,
		},
		{
			name:       "installer download fails",
			headStatus: http.StatusOK,
			wantError:  "could not download the installer: unexpected status 503",
			wantGets:   1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setVersion(t, "1.0.0")
			var tagHeads, installerHeads, installerGets, unexpected atomic.Int64
			installer := "install.sh"
			if runtime.GOOS == "windows" {
				installer = "install.ps1"
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodHead && r.URL.Path == "/tag/v1.2.0":
					tagHeads.Add(1)
					w.WriteHeader(http.StatusOK)
				case r.URL.Path == "/download/v1.2.0/"+installer && r.Method == http.MethodHead:
					installerHeads.Add(1)
					w.WriteHeader(test.headStatus)
				case r.URL.Path == "/download/v1.2.0/"+installer && r.Method == http.MethodGet:
					installerGets.Add(1)
					w.WriteHeader(http.StatusServiceUnavailable)
				default:
					unexpected.Add(1)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			t.Cleanup(server.Close)
			setUpdateChecker(t, server.URL)

			cmd := newRootCmd()
			var stdout, stderr bytes.Buffer
			cmd.SetOut(&stdout)
			cmd.SetErr(&stderr)
			cmd.SetArgs([]string{"upgrade", "--version", "v1.2.0", "--yes", "-o", "json"})

			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("expected error containing %q, got %v", test.wantError, err)
			}
			if got := stdout.String(); got != "" {
				t.Errorf("expected no success JSON, got stdout %q", got)
			}
			if got := tagHeads.Load(); got != 1 {
				t.Errorf("expected one release HEAD, got %d", got)
			}
			if got := installerHeads.Load(); got != 1 {
				t.Errorf("expected one installer HEAD, got %d", got)
			}
			if got := installerGets.Load(); got != test.wantGets {
				t.Errorf("expected %d installer GETs, got %d", test.wantGets, got)
			}
			if got := unexpected.Load(); got != 0 {
				t.Errorf("received %d unexpected requests", got)
			}
		})
	}
}

func TestRunUpgradeNilContextAlreadyInstalled(t *testing.T) {
	tests := []struct {
		name  string
		flags upgradeFlags
		want  string
	}{
		{name: "latest release", want: "nvfleetint v1.2.0 is already the newest release.\n"},
		{name: "named release", flags: upgradeFlags{version: "v1.2.0", versionSet: true}, want: "nvfleetint v1.2.0 is already installed.\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server, requests := releaseServer(t, "v1.2.0")
			setUpdateChecker(t, server.URL)
			recorder := captureUpgrades(t, nil)

			cmd := &cobra.Command{}
			var stdout, stderr bytes.Buffer
			cmd.SetOut(&stdout)
			cmd.SetErr(&stderr)
			if cmd.Context() != nil {
				t.Fatal("expected a fresh command to have a nil context")
			}
			if err := runUpgrade(cmd, BuildInfo{Version: "1.2.0"}, "table", test.flags); err != nil {
				t.Fatalf("runUpgrade failed: %v", err)
			}
			if got := stdout.String(); got != test.want {
				t.Errorf("stdout = %q, want %q", got, test.want)
			}
			if len(recorder.plans) != 0 {
				t.Errorf("expected no upgrade plans, got %d", len(recorder.plans))
			}
			if got := requests.Load(); got != 1 {
				t.Errorf("expected one release lookup and no installer requests, got %d requests", got)
			}
		})
	}
}
