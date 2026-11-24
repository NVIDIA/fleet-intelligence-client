// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdutil"
	clioutput "github.com/NVIDIA/fleet-intelligence-client/internal/output"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
)

func TestReportAPIPermissionFailures(t *testing.T) {
	cases := []struct {
		name, report, format, output, accept string
		signed, all                          bool
	}{
		{name: "inventory table", report: "inventory", format: "json", output: "table", accept: "application/json"},
		{name: "inventory json", report: "inventory", format: "json", output: "json", accept: "application/json"},
		{name: "inventory csv", report: "inventory", format: "csv", output: "table", accept: "text/csv"},
		{name: "inventory signed csv", report: "inventory", format: "csv", output: "table", accept: "application/zip", signed: true},
		{name: "error table", report: "error", format: "json", output: "table", accept: "application/json"},
		{name: "error json", report: "error", format: "json", output: "json", accept: "application/json"},
		{name: "error csv", report: "error", format: "csv", output: "table", accept: "text/csv"},
		{name: "inventory all table", report: "inventory", format: "json", output: "table", accept: "application/json", all: true},
		{name: "inventory all json", report: "inventory", format: "json", output: "json", accept: "application/json", all: true},
		{name: "error all table", report: "error", format: "json", output: "table", accept: "application/json", all: true},
		{name: "error all json", report: "error", format: "json", output: "json", accept: "application/json", all: true},
	}
	type request struct {
		method, path, accept string
		query                url.Values
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			var mu sync.Mutex
			var requests []request
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				requests = append(requests, request{r.Method, r.URL.Path, r.Header.Get("Accept"), r.URL.Query()})
				requestNumber := len(requests)
				mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				if tc.all && requestNumber == 1 {
					if tc.report == "inventory" {
						_, _ = w.Write([]byte(`{"nodes":[{"nodeUUID":"first-node"}],"page":0,"pageSize":1,"total":2,"hasMore":true}`))
					} else {
						_, _ = w.Write([]byte(`{"errors":[{"name":"first-error","count":1}],"page":0,"pageSize":1,"total":2,"hasMore":true}`))
					}
					return
				}
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"Forbidden","details":"no report access"}`))
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")

			args := []string{"report", tc.report, "--data-source", "inband", "--format", tc.format}
			if tc.format != "csv" || tc.signed {
				args = append(args, "--output", tc.output)
			}
			if tc.report == "error" {
				args = append(args, "--view", "list", "--group-by", "error", "--window", "24h")
			}
			if tc.signed {
				args = append(args, "--signed")
			}
			if tc.all {
				args = append(args, "--all", "--page-size", "1")
			}
			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetArgs(args)
			err := cmd.Execute()
			var apiErr *nvfleetint.APIError
			if !errors.As(err, &apiErr) {
				t.Errorf("error = %v, want *nvfleetint.APIError", err)
			} else if apiErr.StatusCode != http.StatusForbidden || apiErr.Message != "Forbidden" || apiErr.Details != "no report access" {
				t.Errorf("API error = %+v, want 403, Forbidden, no report access", apiErr)
			}
			if tc.all && (err == nil || !strings.Contains(err.Error(), "page 1 after 1 completed pages")) {
				t.Errorf("pagination error = %v, want page 1 after 1 completed pages", err)
			}
			if out.Len() != 0 {
				t.Errorf("unexpected output after failure: %q", out.String())
			}

			mu.Lock()
			defer mu.Unlock()
			wantCount := 1
			if tc.all {
				wantCount = 2
			}
			if len(requests) != wantCount {
				t.Errorf("request count = %d, want %d", len(requests), wantCount)
			}
			for i, req := range requests {
				if req.method != http.MethodGet || req.path != "/v1/reports/"+tc.report || req.accept != tc.accept {
					t.Errorf("request %d: %s %s Accept=%q", i, req.method, req.path, req.accept)
				}
				want := map[string]string{"dataSource": "inband", "format": tc.format}
				if tc.report == "error" {
					want["view"], want["groupBy"], want["window"] = "list", "error", "24h"
				}
				if tc.signed {
					want["signed"] = "true"
				} else {
					want["signed"] = ""
				}
				if tc.all {
					want["page"], want["pageSize"] = strconv.Itoa(i), "1"
				}
				for key, value := range want {
					if got := req.query.Get(key); got != value {
						t.Errorf("request %d query %s = %q, want %q (query %v)", i, key, got, value, req.query)
					}
				}
			}
		})
	}
}

func TestReportOutputTableWriteFailureStopsBeforeFooter(t *testing.T) {
	common := cmdutil.Resolved{Output: clioutput.FormatTable}
	page := &clioutput.Pagination{Page: 0, PageSize: 1, Total: 1}
	sentinel := errors.New("table write failed")
	cases := []struct {
		name  string
		write func(*reportFailingWriter) error
	}{
		{name: "in-band inventory", write: func(w *reportFailingWriter) error {
			return writeReportInventoryOutput(w, common, reportInventoryOutput{Report: nvfleetint.InventoryReport{DataSource: nvfleetint.DataSourceInband, Nodes: []nvfleetint.InventoryNode{{NodeUUID: "node-inband"}}}, Page: page})
		}},
		{name: "OOB inventory", write: func(w *reportFailingWriter) error {
			return writeReportInventoryOutput(w, common, reportInventoryOutput{Report: nvfleetint.InventoryReport{DataSource: nvfleetint.DataSourceOOB, Nodes: []nvfleetint.InventoryNode{{NodeUUID: "node-oob"}}}, Page: page})
		}},
		{name: "error list", write: func(w *reportFailingWriter) error {
			return writeReportErrorOutput(w, common, reportErrorOutput{Report: nvfleetint.ErrorReport{View: nvfleetint.ErrorReportViewList, GroupBy: nvfleetint.ErrorReportGroupByError, Errors: []nvfleetint.ErrorReportError{{Name: "error-one"}}}, Page: page})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := &reportFailingWriter{err: sentinel}
			if err := tc.write(w); !errors.Is(err, sentinel) {
				t.Errorf("error = %v, want sentinel", err)
			}
			if w.calls != 1 {
				t.Errorf("write calls = %d, want 1 table attempt and no footer", w.calls)
			}
		})
	}
}
