// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"bytes"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdutil"
	clioutput "github.com/NVIDIA/fleet-intelligence-client/internal/output"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
)

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
