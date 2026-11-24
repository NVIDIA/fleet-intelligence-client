// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package xidburst

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
)

// Verifies xidburst describe renders the field/value table
func TestXIDBurstDescribeTable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/xid/bursts/burst-1" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"burstId":"burst-1","nodeUuid":"node-1","hostname":"gpu-01","nodeGroup":"ng","nodeGroupId":"ng-1","computeZone":"cz","computeZoneId":"cz-1","startTime":"2026-05-01T00:00:00Z","endTime":"2026-05-01T00:05:00Z","burstDurationSeconds":300,"xidCount":2,"xidNumbers":[{"xidNumber":48,"mnemonic":"DBE","description":"Double Bit ECC"}],"deviceIds":{"0000:0f:00.0":[48,94]},"jobDisruption":true,"category":"GPU","subcategory":"Memory","stickyXidsSuppressed":1,"suggestedActions":[{"action":"Drain the node","code":"DRAIN","persona":"tenant","type":"immediate"}]}`))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"xidburst", "describe", "burst-1"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"FIELD", "VALUE", "BURST ID", "burst-1", "NODE UUID", "HOSTNAME", "gpu-01",
		"NODE GROUP", "COMPUTE ZONE", "DURATION (S)", "300", "XIDS", "48",
		"JOB DISRUPTION", "true", "CATEGORY", "GPU", "SUBCATEGORY", "Memory",
		"XID 48", "DBE: Double Bit ECC", "DEVICE 0000:0f:00.0", "48, 94",
		"TENANT IMMEDIATE", "DRAIN: Drain the node", "END TIME", "2026-05-01T00:05:00Z",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
	// A field the backend omitted for this persona renders as "-", not "false".
	if !strings.Contains(got, "PLATFORM JOB DISRUPTION") || !strings.Contains(got, "PLATFORM JOB DISRUPTION  -") {
		t.Fatalf("expected omitted platform disruption to render as '-':\n%s", got)
	}
}

// Verifies xidburst describe JSON output is the raw backend payload
func TestXIDBurstDescribeJSON(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	body := `{"burstId":"burst-1","hostname":"gpu-01","jobDisruption":true}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"xidburst", "describe", "burst-1", "--output", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}
	if strings.TrimSpace(out.String()) != body {
		t.Fatalf("unexpected JSON:\ngot  %s\nwant %s", strings.TrimSpace(out.String()), body)
	}
}

func TestXIDBurstDescribeSparseDetails(t *testing.T) {
	number := 48
	burst := nvfleetint.XIDBurstDetails{XIDBurst: nvfleetint.XIDBurst{
		XIDNumbers:       []nvfleetint.XIDBurstXID{{}, {XIDNumber: &number, Description: "Memory error"}},
		DeviceIDs:        map[string][]int{"z-device": {94}, "a-device": {48}},
		SuggestedActions: []nvfleetint.SuggestedAction{{Code: "DRAIN", Action: "Drain node"}},
	}}
	rows := xidBurstDescribeRows(burst)
	if got := rows[13]; !reflect.DeepEqual(got, []string{"XIDS", "48"}) {
		t.Errorf("XIDS row = %v, want [XIDS 48]", got)
	}
	wantDetails := [][]string{
		{"XID 48", "Memory error"},
		{"DEVICE a-device", "48"},
		{"DEVICE z-device", "94"},
		{"SUGGESTED ACTION", "DRAIN: Drain node"},
	}
	if got := rows[19:]; !reflect.DeepEqual(got, wantDetails) {
		t.Errorf("detail rows = %v, want %v", got, wantDetails)
	}
	if got := xidBurstDeviceRows(map[string][]int{}); len(got) != 0 {
		t.Errorf("empty device map produced rows: %v", got)
	}
	var out bytes.Buffer
	if err := writeXIDBurstDescribeTable(&out, burst); err != nil {
		t.Fatalf("render describe table: %v", err)
	}
	for _, want := range []string{"XID 48", "Memory error", "DEVICE a-device", "DEVICE z-device", "SUGGESTED ACTION", "DRAIN: Drain node"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("describe output missing %q: %s", want, out.String())
		}
	}
}
