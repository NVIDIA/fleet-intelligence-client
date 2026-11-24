// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package alert

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
)

// Verifies the CLI exposes alert options in JSON and table formats.
func TestAlertTimelineOptionsOutput(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/alert_timeline/filter_options" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		requests++
		wantActive := requests == 1
		if got := r.URL.Query().Get("active"); got != map[bool]string{true: "true", false: "false"}[wantActive] {
			t.Fatalf("unexpected active value on request %d: %q", requests, got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"filters":{"fields":[{"name":"gpuTypes","options":["H100"]},{"name":"nodeGroups","options":[{"id":"ng-1","value":"Training"}]}]},"sorting":{"fields":["alert","hostname"],"orders":["asc","desc"],"defaults":{"field":"alert","order":"desc"}}}`))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")
	var jsonOut bytes.Buffer
	jsonCmd := newRootCmd()
	jsonCmd.SetOut(&jsonOut)
	jsonCmd.SetArgs([]string{"alert", "options", "--output", "json"})
	if err := jsonCmd.Execute(); err != nil {
		t.Fatalf("alert options JSON command failed: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(jsonOut.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, jsonOut.String())
	}
	if got["filters"] == nil || got["sorting"] == nil {
		t.Fatalf("unexpected options JSON: %#v", got)
	}

	var tableOut bytes.Buffer
	tableCmd := newRootCmd()
	tableCmd.SetOut(&tableOut)
	tableCmd.SetArgs([]string{"alert", "options", "--view", "historical"})
	if err := tableCmd.Execute(); err != nil {
		t.Fatalf("alert options table command failed: %v", err)
	}
	for _, want := range []string{
		"--gpu-type", "H100",
		"--nodegroup-ids", "ng-1", "Training",
		"--sort-by", "hostname", "--order", "(default: alert)", "(default: desc)",
	} {
		if !strings.Contains(tableOut.String(), want) {
			t.Fatalf("table output missing %q:\n%s", want, tableOut.String())
		}
	}

	// The endpoint advertises the summary's columns only, so that is the one
	// sorting section rendered; `alert node`'s columns are documented on its
	// own flags rather than invented here.
	if !strings.Contains(tableOut.String(), "Sorting for 'alert summary'") {
		t.Fatalf("missing 'alert summary' sorting section:\n%s", tableOut.String())
	}
	if strings.Contains(tableOut.String(), "Sorting for 'alert node'") {
		t.Fatalf("'alert node' sorting is not advertised by the endpoint:\n%s", tableOut.String())
	}
	if strings.Contains(tableOut.String(), "startTime") {
		t.Fatalf("options table invents a sort column the endpoint never returned:\n%s", tableOut.String())
	}
}

// Verifies filter fields with no matching flag, empty option lists, and absent
// sorting defaults still render rather than being dropped.
func TestAlertTimelineOptionsTableFallbacks(t *testing.T) {
	var out bytes.Buffer
	err := writeAlertTimelineOptionsTable(&out, nvfleetint.AlertTimelineFilterOptions{
		Filters: nvfleetint.AlertTimelineFilters{Fields: []nvfleetint.AlertTimelineFilterField{
			{Name: "newBackendFilter", Options: []nvfleetint.AlertTimelineFilterOption{{Value: "somevalue"}}},
			{Name: "alertStates"},
		}},
		Sorting: nvfleetint.AlertTimelineSortingOptions{Fields: []string{"alert"}, Orders: []string{"asc"}},
	})
	if err != nil {
		t.Fatalf("render options table failed: %v", err)
	}
	for _, want := range []string{"newBackendFilter", "no flag on", "somevalue", "--alert-state", "(none)"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "default:") {
		t.Fatalf("absent sorting defaults should not render:\n%s", out.String())
	}
}
