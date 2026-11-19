// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package computezone

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdutil"
	"github.com/NVIDIA/fleet-intelligence-client/internal/config"
	clioutput "github.com/NVIDIA/fleet-intelligence-client/internal/output"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
)

func TestComputeZoneCommandFailurePropagation(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		firstPage   bool
		updateRead  bool
		badProfile  bool
		wantMethods []string
	}{
		{name: "list forbidden", args: []string{"computezone", "list"}, wantMethods: []string{"GET"}},
		{name: "list second page forbidden", args: []string{"computezone", "list", "--all", "--page-size", "1"}, firstPage: true, wantMethods: []string{"GET", "GET"}},
		{name: "update dry run read forbidden", args: []string{"computezone", "update", "cz-1", "--type", "datacenter", "--dry-run"}, updateRead: true, wantMethods: []string{"GET"}},
		{name: "update write forbidden", args: []string{"computezone", "update", "cz-1", "--type", "datacenter", "--yes"}, updateRead: true, wantMethods: []string{"GET", "PUT"}},
		{name: "list missing profile", args: []string{"computezone", "list", "--profile", "nonexistent"}, badProfile: true},
		{name: "update missing profile", args: []string{"computezone", "update", "cz-1", "--type", "datacenter", "--yes", "--profile", "nonexistent"}, badProfile: true},
	}

	type request struct {
		method string
		path   string
		query  url.Values
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			var mu sync.Mutex
			var requests []request
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				requests = append(requests, request{method: r.Method, path: r.URL.Path, query: r.URL.Query()})
				mu.Unlock()

				w.Header().Set("Content-Type", "application/json")
				if tt.firstPage && r.Method == http.MethodGet && r.URL.Query().Get("page") == "0" {
					_, _ = w.Write([]byte(`{"computezones":[{"id":"cz-1","name":"East"}],"hasMore":true,"page":0,"pageSize":1,"total":2}`))
					return
				}
				if tt.updateRead && len(tt.wantMethods) == 2 && r.Method == http.MethodGet {
					_, _ = w.Write([]byte(`{"computezones":[{"id":"cz-1","name":"East","type":"datacenter"}],"hasMore":false,"page":0,"pageSize":20,"total":1}`))
					return
				}
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"forbidden","details":"no compute zone access"}`))
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")

			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs(tt.args)
			err := cmd.Execute()
			if tt.badProfile {
				if !errors.Is(err, config.ErrProfileNotFound) {
					t.Fatalf("expected profile not found, got %v", err)
				}
			} else {
				var apiErr *nvfleetint.APIError
				if !errors.As(err, &apiErr) {
					t.Fatalf("expected API error, got %v", err)
				}
				if apiErr.StatusCode != http.StatusForbidden || apiErr.Message != "forbidden" || apiErr.Details != "no compute zone access" {
					t.Fatalf("unexpected API error: %#v", apiErr)
				}
				if tt.firstPage && !strings.Contains(err.Error(), "fetch computezones API page 1 after 1 completed pages") {
					t.Errorf("missing pagination context: %v", err)
				}
			}
			if out.Len() != 0 {
				t.Errorf("unexpected result or preview: %q", out.String())
			}

			mu.Lock()
			got := append([]request(nil), requests...)
			mu.Unlock()
			var methods []string
			for _, req := range got {
				methods = append(methods, req.method)
				if req.path != "/v1/computezones" {
					t.Errorf("unexpected request path: %s", req.path)
				}
				if tt.updateRead && req.method == http.MethodGet {
					if req.query.Get("view") != "basic" || !slices.Equal(req.query["computeZoneIds"], []string{"cz-1"}) {
						t.Errorf("unexpected update read query: %v", req.query)
					}
				}
			}
			if !slices.Equal(methods, tt.wantMethods) {
				t.Errorf("request methods = %v, want %v", methods, tt.wantMethods)
			}
			if tt.firstPage && len(got) == 2 {
				for i, page := range []string{"0", "1"} {
					if got[i].query.Get("page") != page || got[i].query.Get("pageSize") != "1" {
						t.Errorf("request %d has unexpected pagination: %v", i, got[i].query)
					}
				}
			}
		})
	}
}

type computeZoneErrorWriter struct {
	err error
}

func (w computeZoneErrorWriter) Write(_ []byte) (int, error) {
	return 0, w.err
}

func TestComputeZoneListOutputWithoutPageAndWriteError(t *testing.T) {
	common := cmdutil.Resolved{Output: clioutput.FormatTable}
	result := computeZoneListOutput{
		ComputeZones: []nvfleetint.ComputeZone{{ID: "cz-1", Name: "East"}},
		View:         string(nvfleetint.ComputeZoneViewBasic),
	}
	var out bytes.Buffer
	if err := writeComputeZoneListOutput(&out, common, result); err != nil {
		t.Fatalf("write table: %v", err)
	}
	for _, want := range []string{"cz-1", "East"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("table missing %q: %q", want, out.String())
		}
	}
	if strings.Contains(out.String(), "Page:") {
		t.Errorf("unexpected pagination footer: %q", out.String())
	}

	sentinel := errors.New("table write failed")
	if err := writeComputeZoneListOutput(computeZoneErrorWriter{err: sentinel}, common, result); !errors.Is(err, sentinel) {
		t.Errorf("write error = %v, want %v", err, sentinel)
	}
}

func TestComputeZoneUpdateEarlyValidation(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "invalid timeout", args: []string{"computezone", "update", "cz-1", "--type", "datacenter", "--yes", "--timeout", "0s"}, want: "--timeout must be greater than 0"},
		{name: "whitespace ID", args: []string{"computezone", "update", "   ", "--type", "datacenter", "--yes"}, want: "compute zone ID is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			var requests atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")

			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs(tt.args)
			err := cmd.Execute()
			if err == nil || err.Error() != tt.want {
				t.Errorf("error = %v, want %q", err, tt.want)
			}
			if out.Len() != 0 {
				t.Errorf("unexpected result output: %q", out.String())
			}
			if got := requests.Load(); got != 0 {
				t.Errorf("HTTP requests = %d, want 0", got)
			}
		})
	}
}
