// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdutil"
	"github.com/NVIDIA/fleet-intelligence-client/internal/config"
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

func TestReportInventorySignedWriteFailure(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	type request struct {
		method, path, accept string
		query                url.Values
	}
	requests := make(chan request, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- request{r.Method, r.URL.Path, r.Header.Get("Accept"), r.URL.Query()}
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write([]byte("PK\x03\x04 signed-zip-bytes"))
	}))
	defer server.Close()
	cmdtest.SaveConfig(t, server.URL, "test-key")

	target := filepath.Join(t.TempDir(), "missing", "inventory.zip")
	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"report", "inventory", "--data-source", "inband", "--format", "csv", "--signed", "--output", "json", "--output-path", target})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "write signed report") || !errors.Is(err, os.ErrNotExist) {
		t.Errorf("error = %v, want write signed report wrapping os.ErrNotExist", err)
	}
	if out.Len() != 0 {
		t.Errorf("unexpected output after write failure: %q", out.String())
	}
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("destination stat error = %v, want os.ErrNotExist", err)
	}
	select {
	case req := <-requests:
		if req.method != http.MethodGet || req.path != "/v1/reports/inventory" || req.accept != "application/zip" {
			t.Errorf("request: %s %s Accept=%q", req.method, req.path, req.accept)
		}
		for key, want := range map[string]string{"dataSource": "inband", "format": "csv", "signed": "true"} {
			if got := req.query.Get(key); got != want {
				t.Errorf("query %s = %q, want %q", key, got, want)
			}
		}
	default:
		t.Error("signed inventory request not received")
	}
}

func TestWriteSignedReportDefaultFilename(t *testing.T) {
	dir := t.TempDir()
	payload := []byte("PK\x03\x04 known signed bundle")
	path, err := writeSignedReport(dir, "", payload)
	if err != nil {
		t.Fatalf("write signed report: %v", err)
	}
	if want := filepath.Join(dir, "inventory-report.zip"); path != want {
		t.Errorf("path = %q, want %q", path, want)
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read signed report: %v", err)
	}
	if !bytes.Equal(written, payload) {
		t.Errorf("contents = %q, want %q", written, payload)
	}
}

func TestReportVerifyArtifactAndKeyErrors(t *testing.T) {
	cases := []struct {
		name string
	}{
		{name: "missing csv"},
		{name: "missing bundle"},
		{name: "csv is a directory"},
		{name: "invalid local key"},
		{name: "invalid fetched key"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			cmdtest.ClearCredentialEnv(t)
			dir := t.TempDir()
			csvPath, bundlePath, keyPath := writeSignedFixture(t, dir, []byte("customer,issued_at\nacme,2026-06-15T00:00:00Z\n"))
			var want []string
			var requests chan string
			switch tc.name {
			case "missing csv":
				csvPath = filepath.Join(dir, "missing.csv")
				want = []string{"--csv", csvPath, "does not exist"}
			case "missing bundle":
				bundlePath = filepath.Join(dir, "missing.sig.bundle")
				want = []string{"--bundle", bundlePath, "does not exist"}
			case "csv is a directory":
				csvPath = dir
				want = []string{"cannot read --csv", csvPath}
			case "invalid local key":
				if err := os.WriteFile(keyPath, []byte("invalid PEM"), 0o644); err != nil {
					t.Fatalf("write invalid key: %v", err)
				}
				want = []string{keyPath, "not a valid PEM public key"}
			case "invalid fetched key":
				requests = make(chan string, 1)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					select {
					case requests <- r.URL.Path:
					default:
					}
					_, _ = w.Write([]byte("invalid PEM"))
				}))
				defer server.Close()
				cmdtest.SaveConfig(t, server.URL, "test-key")
				keyPath = ""
				want = []string{"the signing key fetched from the API is not a valid PEM public key"}
			}

			args := []string{"report", "verify", "--csv", csvPath, "--bundle", bundlePath}
			if keyPath != "" {
				args = append(args, "--key", keyPath)
			}
			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetArgs(args)
			err := cmd.Execute()
			if err == nil {
				t.Fatal("command succeeded, want an error")
			}
			for _, part := range want {
				if !strings.Contains(err.Error(), part) {
					t.Errorf("error = %q, want it to contain %q", err, part)
				}
			}
			if tc.name == "csv is a directory" && strings.Contains(err.Error(), "does not exist") {
				t.Errorf("directory reported as missing: %v", err)
			}
			if out.Len() != 0 {
				t.Errorf("unexpected output after failure: %q", out.String())
			}
			if requests != nil {
				select {
				case path := <-requests:
					if path != "/.well-known/signing-key.pub" {
						t.Errorf("requested path = %q, want signing-key endpoint", path)
					}
				default:
					t.Error("signing-key endpoint was not requested")
				}
			}
		})
	}
}

func TestReportValidationBoundariesRejectBeforeRequest(t *testing.T) {
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

	cases := []struct {
		name string
		args []string
		want []string
	}{
		{name: "inventory csv page", args: []string{"report", "inventory", "--data-source", "inband", "--format", "csv", "--page", "1"}, want: []string{"pagination flags cannot be used with --format csv"}},
		{name: "error csv page size", args: []string{"report", "error", "--view", "list", "--group-by", "error", "--window", "24h", "--format", "csv", "--page-size", "1"}, want: []string{"pagination flags cannot be used with --format csv"}},
		{name: "error overview page", args: []string{"report", "error", "--view", "overview", "--window", "24h", "--page", "1"}, want: []string{"pagination flags cannot be used with --view overview"}},
		{name: "error conflicting time range", args: []string{"report", "error", "--view", "overview", "--window", "24h", "--start", "2026-05-01T00:00:00Z", "--end", "2026-05-02T00:00:00Z"}, want: []string{"--window cannot be used with --start or --end"}},
		{name: "inventory invalid end", args: []string{"report", "inventory", "--start", "2026-05-01T00:00:00Z", "--end", "yesterday"}, want: []string{"--end must be RFC3339"}},
		{name: "error invalid end", args: []string{"report", "error", "--view", "overview", "--start", "2026-05-01T00:00:00Z", "--end", "yesterday"}, want: []string{"--end must be RFC3339"}},
		{name: "error empty severity", args: []string{"report", "error", "--view", "overview", "--window", "24h", "--severities", "Critical,,Fatal"}, want: []string{"invalid severity", "empty values are not allowed"}},
		{name: "inventory invalid page", args: []string{"report", "inventory", "--page", "0"}, want: []string{"--page must be greater than or equal to 1"}},
		{name: "error invalid page", args: []string{"report", "error", "--view", "list", "--group-by", "error", "--window", "24h", "--page", "0"}, want: []string{"--page must be greater than or equal to 1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			for _, part := range tc.want {
				if err == nil || !strings.Contains(err.Error(), part) {
					t.Errorf("error = %v, want it to contain %q", err, part)
				}
			}
			if out.Len() != 0 {
				t.Errorf("unexpected output after failure: %q", out.String())
			}
			mu.Lock()
			count := requests
			mu.Unlock()
			if count != 0 {
				t.Errorf("request count = %d, want 0", count)
			}
		})
	}
}

func TestReportErrorOutputSuggestedActions(t *testing.T) {
	rows := []struct {
		name   string
		action *nvfleetint.SuggestedAction
		want   string
	}{
		{name: "nil-action", want: "-"},
		{name: "both-fields", action: &nvfleetint.SuggestedAction{Action: "  Drain node  ", Code: "  DRAIN  "}, want: "Drain node (DRAIN)"},
		{name: "action-only", action: &nvfleetint.SuggestedAction{Action: "Drain node"}, want: "Drain node"},
		{name: "code-only", action: &nvfleetint.SuggestedAction{Code: "DRAIN"}, want: "DRAIN"},
		{name: "blank-fields", action: &nvfleetint.SuggestedAction{Action: "  ", Code: "  "}, want: "-"},
	}
	report := nvfleetint.ErrorReport{View: nvfleetint.ErrorReportViewList, GroupBy: nvfleetint.ErrorReportGroupByError}
	for _, row := range rows {
		report.Errors = append(report.Errors, nvfleetint.ErrorReportError{Name: row.name, SuggestedAction: row.action})
	}
	var out bytes.Buffer
	if err := writeReportErrorOutput(&out, cmdutil.Resolved{Output: clioutput.FormatTable}, reportErrorOutput{Report: report}); err != nil {
		t.Fatalf("write error report: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != len(rows)+1 {
		t.Fatalf("got %d lines, want header and %d rows: %q", len(lines), len(rows), out.String())
	}
	for _, row := range rows {
		matches := 0
		for _, line := range lines[1:] {
			fields := strings.Fields(line)
			if len(fields) == 0 || fields[0] != row.name {
				continue
			}
			matches++
			// The final column is the action; compare all of it on this row.
			if len(fields) < 5 || strings.Join(fields[4:], " ") != row.want {
				t.Errorf("row %q = %q, want action %q", row.name, line, row.want)
			}
		}
		if matches != 1 {
			t.Errorf("row %q appeared %d times, want once", row.name, matches)
		}
	}
}

func TestReportErrorOutputEmptyViewsAndInvalidView(t *testing.T) {
	common := cmdutil.Resolved{Output: clioutput.FormatTable}
	for _, tc := range []struct {
		name    string
		view    nvfleetint.ErrorReportView
		headers []string
	}{
		{name: "nil graph", view: nvfleetint.ErrorReportViewGraph, headers: []string{"ERROR", "VALUES", "START", "END"}},
		{name: "nil overview", view: nvfleetint.ErrorReportViewOverview, headers: []string{"FIELD", "VALUE"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			err := writeReportErrorOutput(&out, common, reportErrorOutput{Report: nvfleetint.ErrorReport{View: tc.view}})
			if err != nil {
				t.Fatalf("write error report: %v", err)
			}
			lines := strings.Split(strings.TrimSpace(out.String()), "\n")
			if len(lines) != 1 || strings.Join(strings.Fields(lines[0]), "|") != strings.Join(tc.headers, "|") {
				t.Errorf("output = %q, want only headers %v", out.String(), tc.headers)
			}
		})
	}
	t.Run("invalid view", func(t *testing.T) {
		var out bytes.Buffer
		err := writeReportErrorOutput(&out, common, reportErrorOutput{Report: nvfleetint.ErrorReport{View: "unknown-view"}})
		if err == nil || !strings.Contains(err.Error(), "unknown-view") {
			t.Errorf("error = %v, want invalid view identified", err)
		}
		if out.Len() != 0 {
			t.Errorf("unexpected output for invalid view: %q", out.String())
		}
	})
}

type reportFailingWriter struct {
	err   error
	calls int
}

func (w *reportFailingWriter) Write(_ []byte) (int, error) {
	w.calls++
	return 0, w.err
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

func TestReportCommandsRequireCredentials(t *testing.T) {
	for _, name := range []string{"inventory", "error", "verify"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			cmdtest.ClearCredentialEnv(t)
			var args []string
			switch name {
			case "inventory":
				args = []string{"report", "inventory", "--data-source", "inband"}
			case "error":
				args = []string{"report", "error", "--window", "24h"}
			case "verify":
				csv, bundle, _ := writeSignedFixture(t, t.TempDir(), []byte("customer\nacme\n"))
				args = []string{"report", "verify", "--csv", csv, "--bundle", bundle}
			}
			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetArgs(args)
			err := cmd.Execute()
			if !errors.Is(err, config.ErrNoProfile) {
				t.Errorf("error = %v, want ErrNoProfile", err)
			}
			if err == nil || !strings.Contains(err.Error(), "nvfleetint auth add <name>") || !strings.Contains(err.Error(), config.EnvAPIKey) {
				t.Errorf("error = %v, want credential setup hint", err)
			}
			if out.Len() != 0 {
				t.Errorf("unexpected output after failure: %q", out.String())
			}
		})
	}
}

func TestReportVerifyTimeoutPrecedesArtifactAccess(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	cmdtest.ClearCredentialEnv(t)
	dir := t.TempDir()
	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"report", "verify", "--timeout", "0", "--csv", filepath.Join(dir, "missing.csv"), "--bundle", filepath.Join(dir, "missing.bundle")})
	err := cmd.Execute()
	if err == nil || err.Error() != "--timeout must be greater than 0" {
		t.Errorf("error = %v, want timeout validation error", err)
	}
	if out.Len() != 0 {
		t.Errorf("unexpected output after failure: %q", out.String())
	}
}

func TestReportVerifyErrorPreservesUnexpectedError(t *testing.T) {
	sentinel := errors.New("unexpected verification error")
	wrapped := fmt.Errorf("verification context: %w", sentinel)
	got := reportVerifyError(reportVerifyFlags{}, wrapped)
	if got != wrapped || !errors.Is(got, sentinel) {
		t.Errorf("error = %v, want unchanged wrapped sentinel %v", got, wrapped)
	}
}
