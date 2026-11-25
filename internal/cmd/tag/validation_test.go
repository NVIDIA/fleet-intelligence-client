// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package tag

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
	"github.com/NVIDIA/fleet-intelligence-client/internal/config"
)

func TestTagCommandValidationBoundaries(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		want    string
		profile bool
	}{
		{"invalid output", []string{"tag", "list", "--output", "yaml"}, `invalid output "yaml": expected table or json`, false},
		{"zero timeout", []string{"tag", "set", "node-1", "--tags", "gpu-health", "--yes", "--timeout", "0s"}, "--timeout must be greater than 0", false},
		{"node and compute zone", []string{"tag", "list", "--node", "node-1", "--computezone", "zone-1"}, "at most one of --node, --nodegroup, or --computezone may be used", false},
		{"blank node", []string{"tag", "set", "  \t", "--clear", "--yes"}, "node UUID is required", false},
		{"list missing profile", []string{"tag", "list", "--profile", "missing"}, "profile not found", true},
		{"set missing profile", []string{"tag", "set", "node-1", "--clear", "--yes", "--profile", "missing"}, "profile not found", true},
		{"invalid dry run tag", []string{"tag", "set", "node-1", "--tags", "GPU-Health", "--dry-run"}, `invalid tag "GPU-Health": use lowercase letters`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			var rec tagRequestRecorder
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				rec.record(r)
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")

			var out, errOut bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetErr(&errOut)
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want containing %q", err, tc.want)
			}
			if tc.profile && (!errors.Is(err, config.ErrProfileNotFound) || !strings.Contains(err.Error(), "missing")) {
				t.Fatalf("error = %v, want missing profile error", err)
			}
			if out.Len() != 0 || errOut.Len() != 0 {
				t.Fatalf("unexpected output: stdout %q, stderr %q", out.String(), errOut.String())
			}
			if got := rec.snapshot(); len(got) != 0 {
				t.Fatalf("unexpected requests: %+v", got)
			}
		})
	}
}

type tagRequestRecorder struct {
	mu       sync.Mutex
	requests []tagRecordedRequest
}

type tagRecordedRequest struct {
	method string
	path   string
	query  url.Values
}

func (rec *tagRequestRecorder) record(r *http.Request) {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	rec.requests = append(rec.requests, tagRecordedRequest{r.Method, r.URL.Path, r.URL.Query()})
}

func (rec *tagRequestRecorder) snapshot() []tagRecordedRequest {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	return append([]tagRecordedRequest(nil), rec.requests...)
}
