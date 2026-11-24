// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package alert

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
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
)

// Verifies table alert rows and derived has-more metadata
func TestAlertListTableAndHasMore(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/alerts" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("page"); got != "0" {
			t.Fatalf("unexpected page: %q", got)
		}
		if got := r.URL.Query().Get("component"); got != "gpu" {
			t.Fatalf("unexpected component: %q", got)
		}
		if got := r.URL.Query().Get("state"); got != "Triggered" {
			t.Fatalf("unexpected state: %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"alerts":[{"alertUUID":"alert-1","nodeUUID":"node-1","component":"gpu","severity":"Warning","state":"Detected","detectedAt":"2026-05-01T00:00:00Z"}],"page":0,"pageSize":1,"total":2}`))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"alert", "list", "--page", "1", "--component", "gpu", "--state", "Triggered"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{"UUID", "NODE UUID", "COMPONENT", "SEVERITY", "STATE", "FIRED-AT", "alert-1", "Warning", "Page: 1  Total Pages: 2  Page Size: 1  Total Entries: 2"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q: %q", want, got)
		}
	}
}

// Verifies `alert node` documents its own sort columns on its flags. The
// options endpoint advertises `alert summary`'s columns only, so --help is the
// sole discovery path for these two.
func TestAlertNodeHelpDocumentsSortColumns(t *testing.T) {
	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"alert", "node", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("alert node --help failed: %v", err)
	}
	for _, want := range []string{"startTime, lastUpdate, or dataSource", "default: lastUpdate", "asc or desc", "default: desc"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("alert node help missing %q:\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "--sort-by, and --order") {
		t.Fatalf("alert node help still points --sort-by at the options command:\n%s", out.String())
	}
}

func TestAlertCommandAPIErrorPropagation(t *testing.T) {
	const node = "node-1"
	const alertID = "alert-1"
	tests := []struct {
		name      string
		args      []string
		path      string
		firstPage string
		all       bool
	}{
		{name: "list", args: []string{"alert", "list"}, path: "/v1/alerts"},
		{name: "summary", args: []string{"alert", "summary"}, path: "/v1/alert_timeline/nodes"},
		{name: "node", args: []string{"alert", "node", node}, path: "/v1/alert_timeline/nodes/" + node + "/alerts"},
		{name: "options", args: []string{"alert", "options"}, path: "/v1/alert_timeline/filter_options"},
		{name: "describe", args: []string{"alert", "describe", alertID, "--node", node}, path: "/v1/alert_timeline/nodes/" + node + "/alerts/" + alertID},
		{name: "list all", args: []string{"alert", "list"}, path: "/v1/alerts", all: true, firstPage: `{"alerts":[{"alertUUID":"alert-1","nodeUUID":"node-1"}],"page":0,"pageSize":1,"total":2,"hasMore":true}`},
		{name: "summary all", args: []string{"alert", "summary"}, path: "/v1/alert_timeline/nodes", all: true, firstPage: `{"nodes":[{"nodeUuid":"node-1"}],"page":0,"pageSize":1,"total":2,"hasMore":true}`},
		{name: "node all", args: []string{"alert", "node", node}, path: "/v1/alert_timeline/nodes/" + node + "/alerts", all: true, firstPage: `{"nodeUuid":"node-1","alerts":[{"alertUuid":"alert-1"}],"page":0,"pageSize":1,"total":2,"hasMore":true}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			type request struct{ method, path, page, pageSize, authorization string }
			var mu sync.Mutex
			var requests []request
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				requests = append(requests, request{r.Method, r.URL.Path, r.URL.Query().Get("page"), r.URL.Query().Get("pageSize"), r.Header.Get("Authorization")})
				mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				if tc.all && r.URL.Query().Get("page") == "0" {
					_, _ = w.Write([]byte(tc.firstPage))
					return
				}
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"forbidden","details":"no alert access"}`))
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")

			args := append([]string(nil), tc.args...)
			if tc.all {
				args = append(args, "--all", "--page-size", "1", "--output", "json")
			}
			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs(args)
			err := cmd.Execute()
			var apiErr *nvfleetint.APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("expected APIError, got %T: %v", err, err)
			}
			if apiErr.StatusCode != http.StatusForbidden || apiErr.Message != "forbidden" || apiErr.Details != "no alert access" {
				t.Errorf("unexpected API error: %+v", apiErr)
			}
			if out.Len() != 0 {
				t.Errorf("unexpected command output: %q", out.String())
			}

			mu.Lock()
			observed := append([]request(nil), requests...)
			mu.Unlock()
			wantPages := []string{""}
			if tc.all {
				wantPages = []string{"0", "1"}
			}
			if len(observed) != len(wantPages) {
				t.Fatalf("got requests %+v, want %d requests", observed, len(wantPages))
			}
			var pages []string
			for _, got := range observed {
				if got.method != http.MethodGet || got.path != tc.path || got.authorization != "Bearer test-key" {
					t.Errorf("unexpected request: %+v; want GET %s with test key", got, tc.path)
				}
				if tc.all && got.pageSize != "1" {
					t.Errorf("pageSize = %q, want 1", got.pageSize)
				}
				pages = append(pages, got.page)
			}
			if tc.all && !reflect.DeepEqual(pages, wantPages) {
				t.Errorf("requested pages = %v, want %v", pages, wantPages)
			}
		})
	}
}
