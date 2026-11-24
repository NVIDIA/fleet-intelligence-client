// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"slices"
	"strconv"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
)

// Verifies all-page error list reports merge raw JSON items
func TestReportErrorListAllJSONMergesItems(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/reports/error" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		query := r.URL.Query()
		if got := query.Get("view"); got != "list" {
			t.Fatalf("unexpected view: %q", got)
		}
		if got := query.Get("groupBy"); got != "node" {
			t.Fatalf("unexpected groupBy: %q", got)
		}
		if got := query.Get("timeMode"); got != "relative" {
			t.Fatalf("unexpected timeMode: %q", got)
		}
		if got := query.Get("window"); got != "24h" {
			t.Fatalf("unexpected window: %q", got)
		}
		if got := query.Get("pageSize"); got != "1" {
			t.Fatalf("unexpected pageSize: %q", got)
		}
		if got := query["computeZoneIds"]; !slices.Equal(got, []string{"cz-1"}) {
			t.Fatalf("unexpected computeZoneIds: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query["nodeGroupIds"]; !slices.Equal(got, []string{"ng-1"}) {
			t.Fatalf("unexpected nodeGroupIds: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query["tags"]; !slices.Equal(got, []string{"prod"}) {
			t.Fatalf("unexpected tags: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query["errors"]; !slices.Equal(got, []string{"xid_154"}) {
			t.Fatalf("unexpected errors: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query["severities"]; !slices.Equal(got, []string{"Critical", "Fatal"}) {
			t.Fatalf("unexpected severities: %#v raw query %q", got, r.URL.RawQuery)
		}

		w.Header().Set("Content-Type", "application/json")
		switch query.Get("page") {
		case "0":
			requests++
			_, _ = w.Write([]byte(`{"nodes":[{"nodeUUID":"node-1","hostname":"gpu-001","errors":["xid_154"],"extra":"kept"}],"hasMore":true,"page":0,"pageSize":1,"total":2}`))
		case "1":
			requests++
			_, _ = w.Write([]byte(`{"nodes":[{"nodeUUID":"node-2","hostname":"gpu-002","errors":["NVSwitch Fatal Error"]}],"hasMore":false,"page":1,"pageSize":1,"total":2}`))
		default:
			t.Fatalf("unexpected page: %q", query.Get("page"))
		}
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"report", "error", "--view", "list", "--group-by", "node", "--window", "24h", "--compute-zone-ids", "cz-1", "--nodegroup-ids", "ng-1", "--tags", "prod", "--errors", "xid_154", "--severities", "Critical,Fatal", "--all", "--output", "json", "--page-size", "1"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	var got struct {
		Items      []map[string]any `json:"items"`
		Pagination struct {
			Page         int  `json:"page"`
			PageSize     int  `json:"pageSize"`
			Total        int  `json:"total"`
			HasMore      bool `json:"hasMore"`
			PagesFetched int  `json:"pagesFetched"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, out.String())
	}
	if requests != 2 {
		t.Fatalf("unexpected request count: %d", requests)
	}
	if len(got.Items) != 2 || got.Items[0]["nodeUUID"] != "node-1" || got.Items[0]["extra"] != "kept" {
		t.Fatalf("unexpected merged items: %#v", got.Items)
	}
	if got.Pagination.Page != 1 || got.Pagination.PageSize != 1 || got.Pagination.Total != 2 || got.Pagination.HasMore || got.Pagination.PagesFetched != 2 {
		t.Fatalf("unexpected pagination: %#v", got.Pagination)
	}
}

// Explicit --page is one-based; verify the zero-based backend request and
// one-based JSON pagination while preserving other response fields.
func TestErrorReportExplicitPagePreservesPayload(t *testing.T) {
	for _, cliPage := range []int{1, 3} {
		t.Run(fmt.Sprintf("page_%d", cliPage), func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			apiPage := cliPage - 1
			const pageSize = 7
			body := []byte(fmt.Sprintf(`{"errors":[{"name":"pagination-error","count":3,"nodeCount":1}],"page":%d,"pageSize":7,"total":%d,"hasMore":false,"opaquePaginationContract":"preserved"}`, apiPage, apiPage*pageSize+1))
			type request struct{ method, path, page, size string }
			requests := make(chan request, 4)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query()
				select {
				case requests <- request{r.Method, r.URL.Path, q.Get("page"), q.Get("pageSize")}:
				default:
					t.Error("unexpected extra API requests")
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(body)
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")
			cmd := newRootCmd()
			var out bytes.Buffer
			cmd.SetOut(&out)
			args := []string{"report", "error", "--view", "list", "--group-by", "error", "--window", "24h"}
			args = append(args, "--page", strconv.Itoa(cliPage), "--page-size", strconv.Itoa(pageSize), "--output", "json")
			cmd.SetArgs(args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("command failed: %v", err)
			}
			if got := len(requests); got != 1 {
				t.Fatalf("API request count = %d, want 1", got)
			}
			gotRequest := <-requests
			wantRequest := request{http.MethodGet, "/v1/reports/error", strconv.Itoa(apiPage), strconv.Itoa(pageSize)}
			if gotRequest != wantRequest {
				t.Fatalf("request = %+v, want %+v", gotRequest, wantRequest)
			}
			var got, want map[string]any
			if err := json.Unmarshal(out.Bytes(), &got); err != nil {
				t.Fatalf("invalid command JSON %q: %v", out.String(), err)
			}
			if err := json.Unmarshal(body, &want); err != nil {
				t.Fatal(err)
			}
			// The documented CLI contract normalizes API page 0 to CLI page 1.
			// See output.OneBasedPage and the existing command JSON tests.
			want["page"] = float64(cliPage)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("JSON result = %#v, want normalized CLI result %#v", got, want)
			}
		})
	}
}
