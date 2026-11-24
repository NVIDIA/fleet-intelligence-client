// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdutil"
	clioutput "github.com/NVIDIA/fleet-intelligence-client/internal/output"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
)

// Verifies overview error report table output and absolute time filters
func TestReportErrorOverviewTable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	start := "2026-05-01T00:00:00Z"
	end := "2026-05-02T00:00:00Z"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/reports/error" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		query := r.URL.Query()
		if got := query.Get("view"); got != "overview" {
			t.Fatalf("unexpected view: %q", got)
		}
		if got := query.Get("timeMode"); got != "absolute" {
			t.Fatalf("unexpected timeMode: %q", got)
		}
		if got := query.Get("startTime"); got != start {
			t.Fatalf("unexpected startTime: %q", got)
		}
		if got := query.Get("endTime"); got != end {
			t.Fatalf("unexpected endTime: %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"totalErrors":1250,"totalErrorNodes":42,"totalErrorTypes":15}`))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"report", "error", "--view", "overview", "--start", start, "--end", end})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{"FIELD", "VALUE", "TOTAL ERRORS", "1250", "TOTAL ERROR NODES", "42", "TOTAL ERROR TYPES", "15"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q: %q", want, got)
		}
	}
}

// Verifies the error report defaults to the overview view when --view is omitted
func TestReportErrorDefaultsToOverview(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/reports/error" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		query := r.URL.Query()
		if got := query.Get("view"); got != "overview" {
			t.Fatalf("unexpected view: %q", got)
		}
		if got := query.Get("groupBy"); got != "" {
			t.Fatalf("unexpected groupBy: %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"totalErrors":1250,"totalErrorNodes":42,"totalErrorTypes":15}`))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"report", "error", "--window", "24h"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	if !strings.Contains(out.String(), "TOTAL ERRORS") {
		t.Fatalf("expected overview table output: %q", out.String())
	}
}

// Verifies seven-day hour windows pass through unchanged
func TestReportErrorGraphSevenDayHourWindow(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/reports/error" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		query := r.URL.Query()
		if got := query.Get("view"); got != "graph" {
			t.Fatalf("unexpected view: %q", got)
		}
		if got := query.Get("groupBy"); got != "error" {
			t.Fatalf("unexpected groupBy: %q", got)
		}
		if got := query.Get("timeMode"); got != "relative" {
			t.Fatalf("unexpected timeMode: %q", got)
		}
		if got := query.Get("window"); got != "168h" {
			t.Fatalf("unexpected window: %q", got)
		}
		if got := query.Get("step"); got != "5m" {
			t.Fatalf("unexpected step: %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":[{"label":{"error":"xid_154"},"values":[[1716153600,5]]}],"timeRange":{"start":"2026-05-01T00:00:00Z","end":"2026-05-08T00:00:00Z"}}`))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"report", "error", "--view", "graph", "--window", "168h", "--step", "5m"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{"ERROR", "VALUES", "START", "END", "xid_154", "[[1716153600,5]]", "2026-05-01T00:00:00Z", "2026-05-08T00:00:00Z"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q: %q", want, got)
		}
	}
}

// Verifies backend-style hour windows pass through unchanged
func TestReportErrorGraphThreeHourWindow(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/reports/error" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		query := r.URL.Query()
		if got := query.Get("view"); got != "graph" {
			t.Fatalf("unexpected view: %q", got)
		}
		if got := query.Get("groupBy"); got != "error" {
			t.Fatalf("unexpected groupBy: %q", got)
		}
		if got := query.Get("timeMode"); got != "relative" {
			t.Fatalf("unexpected timeMode: %q", got)
		}
		if got := query.Get("window"); got != "3h" {
			t.Fatalf("unexpected window: %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":[],"timeRange":{"start":"2026-06-03T17:00:00Z","end":"2026-06-03T20:00:00Z"}}`))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"report", "error", "--view", "graph", "--window", "3h"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}
	if !strings.Contains(out.String(), "ERROR") {
		t.Fatalf("expected graph table header: %q", out.String())
	}
}

func TestErrorReportSeverityColumn(t *testing.T) {
	var out bytes.Buffer
	report := nvfleetint.ErrorReport{View: nvfleetint.ErrorReportViewList, GroupBy: nvfleetint.ErrorReportGroupByError, Errors: []nvfleetint.ErrorReportError{{Name: "XID 79", Severities: []nvfleetint.ErrorSeverity{nvfleetint.ErrorSeverityFatal, nvfleetint.ErrorSeverityWarning}}, {Name: "No severity"}}}
	if err := writeReportErrorTable(&out, report); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"SEVERITIES", "XID 79", "Fatal", "Warning", "No severity"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %s: %s", want, &out)
		}
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
