// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package release

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

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
