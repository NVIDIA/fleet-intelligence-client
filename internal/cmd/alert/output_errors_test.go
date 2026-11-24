// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package alert

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdutil"
	clioutput "github.com/NVIDIA/fleet-intelligence-client/internal/output"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
)

func TestAlertTableOutputWriterErrors(t *testing.T) {
	sentinel := errors.New("table output failed")
	common := cmdutil.Resolved{Output: clioutput.FormatTable}
	for _, tc := range []struct {
		name  string
		write func() error
	}{
		{"list", func() error { return writeAlertListOutput(alertErrorWriter{sentinel}, common, alertListOutput{}) }},
		{"timeline nodes", func() error {
			return writeAlertTimelineOutput(alertErrorWriter{sentinel}, common, alertTimelineOutput{Mode: alertTimelineModeNodes})
		}},
		{"timeline alerts", func() error {
			return writeAlertTimelineOutput(alertErrorWriter{sentinel}, common, alertTimelineOutput{Mode: alertTimelineModeAlerts})
		}},
		{"options", func() error {
			return writeAlertTimelineOptionsTable(alertErrorWriter{sentinel}, nvfleetint.AlertTimelineFilterOptions{})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.write(); !errors.Is(err, sentinel) {
				t.Errorf("error = %v, want sentinel", err)
			}
		})
	}
}

func TestAlertTablesWithoutPagination(t *testing.T) {
	common := cmdutil.Resolved{Output: clioutput.FormatTable}
	var out bytes.Buffer
	if err := writeAlertListOutput(&out, common, alertListOutput{Alerts: []nvfleetint.Alert{{UUID: "alert-1", NodeUUID: "node-1", Component: "gpu", Severity: "critical"}}}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"UUID", "NODE UUID", "COMPONENT", "SEVERITY", "alert-1", "node-1", "gpu", "critical"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("list output %q missing %q", out.String(), want)
		}
	}
	if strings.Contains(out.String(), "Page:") {
		t.Errorf("unexpected pagination footer: %q", out.String())
	}

	out.Reset()
	node := nvfleetint.AlertTimelineNode{NodeUUID: "node-2", Hostname: "host-2", AgentStatus: &nvfleetint.AlertAgentStatuses{Inband: &nvfleetint.AlertAgentSourceStatus{Status: "online"}}, CriticalCount: 3}
	if err := writeAlertTimelineOutput(&out, common, alertTimelineOutput{Mode: alertTimelineModeNodes, Nodes: []nvfleetint.AlertTimelineNode{node}}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"NODE UUID", "HOSTNAME", "INBAND STATUS", "OOB STATUS", "CRITICAL", "node-2", "host-2"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("node output %q missing %q", out.String(), want)
		}
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("node output has %d lines: %q", len(lines), out.String())
	}
	fields := strings.Fields(lines[1])
	if len(fields) != 13 || fields[4] != "online" || fields[5] != "-" || fields[6] != "3" {
		t.Errorf("node row = %v, want inband online, missing OOB -, and critical count 3", fields)
	}
	if strings.Contains(out.String(), "not_enrolled") || strings.Contains(out.String(), "Page:") {
		t.Errorf("unexpected status or pagination footer: %q", out.String())
	}

	out.Reset()
	if err := writeAlertTimelineOutput(&out, common, alertTimelineOutput{Mode: alertTimelineModeAlerts, Alerts: []nvfleetint.AlertTimelineNodeAlert{{AlertUUID: "alert-2", Component: "memory", AlertStatus: "Detected"}}}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"ALERT UUID", "COMPONENT", "STATUS", "alert-2", "memory", "Detected"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("timeline output %q missing %q", out.String(), want)
		}
	}
	if strings.Contains(out.String(), "Page:") {
		t.Errorf("unexpected pagination footer: %q", out.String())
	}
}
