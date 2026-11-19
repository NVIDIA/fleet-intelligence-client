// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nodegroup

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdutil"
	clioutput "github.com/NVIDIA/fleet-intelligence-client/internal/output"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
)

func TestNodeGroupCommandValidationAndMissingProfile(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want []string
	}{
		{"all with page", []string{"list", "--all", "--page", "1"}, []string{"--page cannot be used with --all"}},
		{"empty health value", []string{"list", "--health", "Healthy,,Degraded"}, []string{"invalid health", "empty values are not allowed"}},
		{"zero timeout", []string{"options", "--timeout", "0s"}, []string{"--timeout must be greater than 0"}},
		{"list missing profile", []string{"list", "--profile", "missing"}, []string{"profile not found", "missing"}},
		{"options missing profile", []string{"options", "--profile", "missing"}, []string{"profile not found", "missing"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			var mu sync.Mutex
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				mu.Lock()
				requests++
				mu.Unlock()
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")
			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs(append([]string{"nodegroup"}, tc.args...))
			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected command error")
			}
			for _, want := range tc.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
			if out.Len() != 0 {
				t.Errorf("unexpected output: %q", out.String())
			}
			mu.Lock()
			gotRequests := requests
			mu.Unlock()
			if gotRequests != 0 {
				t.Errorf("requests = %d, want 0", gotRequests)
			}
		})
	}
}

func TestNodeGroupCommandAPIFailures(t *testing.T) {
	type request struct{ method, path, page, pageSize string }
	cases := []struct {
		name string
		args []string
		want []request
	}{
		{"list", []string{"list"}, []request{{method: "GET", path: "/v1/nodegroups"}}},
		{"options", []string{"options"}, []request{{method: "GET", path: "/v1/nodegroups/options"}}},
		{"later page", []string{"list", "--all", "--page-size", "1"}, []request{
			{method: "GET", path: "/v1/nodegroups", page: "0", pageSize: "1"},
			{method: "GET", path: "/v1/nodegroups", page: "1", pageSize: "1"},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			var mu sync.Mutex
			var requests []request
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				requests = append(requests, request{r.Method, r.URL.Path, r.URL.Query().Get("page"), r.URL.Query().Get("pageSize")})
				mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				if tc.name == "later page" && r.URL.Query().Get("page") == "0" {
					_, _ = w.Write([]byte(`{"nodeGroups":[{"id":"ng-1","name":"Training"}],"hasMore":true,"page":0,"pageSize":1,"total":2}`))
					return
				}
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"forbidden","details":"no nodegroup access"}`))
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")
			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs(append([]string{"nodegroup"}, tc.args...))
			err := cmd.Execute()
			var apiErr *nvfleetint.APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("error = %v, want *nvfleetint.APIError", err)
			}
			if apiErr.StatusCode != http.StatusForbidden || apiErr.Message != "forbidden" || apiErr.Details != "no nodegroup access" {
				t.Errorf("unexpected API error: %+v", apiErr)
			}
			if out.Len() != 0 {
				t.Errorf("unexpected partial output: %q", out.String())
			}
			mu.Lock()
			got := append([]request(nil), requests...)
			mu.Unlock()
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("requests = %#v, want %#v", got, tc.want)
			}
		})
	}
}

type nodeGroupFailingWriter struct{ err error }

func (w nodeGroupFailingWriter) Write([]byte) (int, error) { return 0, w.err }

func TestNodeGroupListOutputBoundaries(t *testing.T) {
	common := cmdutil.Resolved{Output: clioutput.FormatTable}
	result := nodeGroupListOutput{
		View:       string(nvfleetint.NodeGroupViewBasic),
		NodeGroups: []nvfleetint.NodeGroup{{ID: "ng-1", Name: "Training"}},
	}
	sentinel := errors.New("table write failed")
	if err := writeNodeGroupListOutput(nodeGroupFailingWriter{sentinel}, common, result); !errors.Is(err, sentinel) {
		t.Errorf("error = %v, want sentinel", err)
	}
	var out bytes.Buffer
	if err := writeNodeGroupListOutput(&out, common, result); err != nil {
		t.Fatalf("write output: %v", err)
	}
	for _, want := range []string{"ng-1", "Training"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q: %q", want, out.String())
		}
	}
	if strings.Contains(out.String(), "Page:") || strings.Contains(out.String(), "Total Entries:") {
		t.Errorf("unexpected pagination footer: %q", out.String())
	}
}
