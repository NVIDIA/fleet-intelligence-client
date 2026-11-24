// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package node

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
)

// Verifies node describe table output
func TestNodeDescribeTable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/nodes/node-1" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodeUUID":"node-1","hostname":"gpu-001","computeZone":"East","computeZoneId":"cz-1","nodeGroup":"Training","nodeGroupId":"ng-1","healthStatus":"Degraded","gpuType":"NVIDIA-H100","gpuCount":8,"agentStatus":"Online","resources":{"gpuInfo":{"product":"NVIDIA H100","gpus":[{"uuid":"GPU-1"}]}},"systemInfo":{"agentVersion":"1.2.3","cudaVersion":"12.4"},"tags":["prod","h100"]}`))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"node", "describe", "node-1", "--data-source", "inband"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{"FIELD", "VALUE", "UUID", "node-1", "COMPUTE ZONE", "East (cz-1)", "GPU DEVICES", "1", "CUDA", "12.4"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q: %q", want, got)
		}
	}
}

// Verifies node describe combines independent in-band and OOB views by default
func TestNodeDescribeCombinedViews(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("dataSource") {
		case "inband":
			_, _ = w.Write([]byte(`{
				"nodeUUID":"node-1",
				"hostname":"gpu-001",
				"dataSource":"inband",
				"healthStatus":"Healthy",
				"agentStatus":"Online"
			}`))
		case "oob":
			_, _ = w.Write([]byte(`{
				"nodeUUID":"node-1",
				"dataSource":"oob",
				"bmcMac":"bmc-001",
				"bmcIP":"192.0.2.10:443",
				"healthStatus":"Degraded",
				"oobInventory":{
					"schemaVersion":"inventory.v1alpha1",
					"source":{"sourceType":"redfish","vendor":"NVIDIA"}
				}
			}`))
		default:
			t.Errorf("unexpected dataSource: %q", r.URL.Query().Get("dataSource"))
			http.Error(w, "missing dataSource", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"node", "describe", "node-1"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("combined table command failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"In-band", "gpu-001", "Online",
		"Out-of-band", "bmc-001", "192.0.2.10:443",
		"INVENTORY SCHEMA VERSION", "inventory.v1alpha1",
		"SOURCE TYPE", "redfish", "SOURCE VENDOR", "NVIDIA",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("combined output missing %q: %q", want, got)
		}
	}

	out.Reset()
	cmd = newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"node", "describe", "node-1", "--output", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("combined JSON command failed: %v", err)
	}

	var combined struct {
		Inband map[string]any `json:"inband"`
		OOB    map[string]any `json:"oob"`
	}
	if err := json.Unmarshal(out.Bytes(), &combined); err != nil {
		t.Fatalf("decode combined JSON: %v", err)
	}
	if combined.Inband["hostname"] != "gpu-001" {
		t.Fatalf("unexpected in-band JSON: %#v", combined.Inband)
	}
	if combined.OOB["bmcMac"] != "bmc-001" {
		t.Fatalf("unexpected OOB JSON: %#v", combined.OOB)
	}
}

// Verifies a missing agent-specific view does not hide the available view
func TestNodeDescribeCombinedViewAllowsNotFound(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("dataSource") == "oob" {
			http.Error(w, `{"error":"node does not have an OOB agent"}`, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"nodeUUID":"node-1",
			"hostname":"gpu-001",
			"dataSource":"inband"
		}`))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"node", "describe", "node-1"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("combined command failed: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "In-band") || !strings.Contains(got, "gpu-001") {
		t.Fatalf("available in-band view is missing: %q", got)
	}
	if strings.Contains(got, "Out-of-band") {
		t.Fatalf("missing OOB view should not be rendered: %q", got)
	}
}

func TestNodeDescribeSummaryWriteBoundaries(t *testing.T) {
	inband := nvfleetint.NodeDetails{Node: nvfleetint.Node{UUID: "inband-node", DataSource: "inband"}}
	oob := nvfleetint.NodeDetails{Node: nvfleetint.Node{UUID: "oob-node", DataSource: "oob"}}
	result := nodeDescribeResult{Inband: &inband, OOB: &oob}
	options := nodeDescribeRenderOptions{showSummary: true}
	control := &nodeRenderWriter{failAt: -1}
	if err := writeNodeDescribeResultTable(control, result, "", options); err != nil {
		t.Fatalf("render combined control: %v", err)
	}
	full := control.output.String()
	positions := []int{strings.Index(full, "In-band\n"), strings.Index(full, "inband-node"), strings.Index(full, "\nOut-of-band\n"), strings.Index(full, "oob-node")}
	if positions[0] != 0 || positions[1] <= positions[0] || positions[2] <= positions[1] || positions[3] <= positions[2] {
		t.Fatalf("unexpected combined section order: %q", full)
	}
	for _, tc := range []struct {
		name   string
		offset int
		write  string
	}{
		{"in-band heading", 0, "In-band\n"},
		{"separator", positions[2], "\n"},
		{"out-of-band heading", positions[2] + 1, "Out-of-band\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			index, offset := -1, 0
			for i, part := range control.writes {
				if offset == tc.offset && part == tc.write {
					index = i
					break
				}
				offset += len(part)
			}
			if index < 0 {
				t.Fatalf("boundary %q at %d not found in writes %q", tc.write, tc.offset, control.writes)
			}
			sentinel := errors.New("describe section write failed")
			w := &nodeRenderWriter{failAt: index, err: sentinel}
			if err := writeNodeDescribeResultTable(w, result, "", options); !errors.Is(err, sentinel) {
				t.Errorf("error = %v, want sentinel", err)
			}
			if w.after != 0 || len(w.writes) != index+1 || w.output.String() != full[:tc.offset] {
				t.Errorf("unexpected writes after failure: accepted=%q writes=%q", w.output.String(), w.writes)
			}
		})
	}

	t.Run("explicit inband summary table", func(t *testing.T) {
		var expected bytes.Buffer
		if err := writeNodeDescribeResultTable(&expected, result, nvfleetint.DataSourceInband, options); err != nil {
			t.Fatalf("render explicit control: %v", err)
		}
		if !strings.Contains(expected.String(), "inband-node") || strings.Contains(expected.String(), "oob-node") {
			t.Fatalf("unexpected explicit output: %q", expected.String())
		}
		sentinel := errors.New("summary table write failed")
		w := &nodeRenderWriter{failAt: 0, err: sentinel}
		if err := writeNodeDescribeResultTable(w, result, nvfleetint.DataSourceInband, options); !errors.Is(err, sentinel) {
			t.Errorf("error = %v, want sentinel", err)
		}
		if w.after != 0 || len(w.writes) != 1 || w.output.Len() != 0 {
			t.Errorf("unexpected writes after summary failure: %q", w.writes)
		}
	})
}

func TestNodeDescribeInbandResourceSummary(t *testing.T) {
	resourceRows := map[string]string{
		"CPU TYPE":            "Xeon-Example",
		"CPU MANUFACTURER":    "ExampleCorp",
		"CPU ARCHITECTURE":    "arm64",
		"CPU LOGICAL CORES":   "96",
		"MEMORY TOTAL BYTES":  "1099511627776",
		"CONTAINER ROOT DISK": "/dev/example0",
		"DISK DEVICES":        "2",
		"NIC INTERFACES":      "3",
	}
	cases := []struct {
		name      string
		response  string
		populated bool
	}{
		{
			name:      "populated resources",
			response:  `{"nodeUUID":"node-1","dataSource":"inband","resources":{"cpuInfo":{"type":"Xeon-Example","manufacturer":"ExampleCorp","architecture":"arm64","logicalCores":"96"},"memoryInfo":{"totalBytes":"1099511627776"},"diskInfo":{"containerRootDisk":"/dev/example0","blockDevices":[{"name":"disk0"},{"name":"disk1"}]},"nicInfo":{"privateIPInterfaces":[{"interface":"eth0"},{"interface":"eth1"},{"interface":"eth2"}]}}}`,
			populated: true,
		},
		{name: "absent resources", response: `{"nodeUUID":"node-1","dataSource":"inband"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			var mu sync.Mutex
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				requests++
				mu.Unlock()
				if r.Method != http.MethodGet || r.URL.Path != "/v1/nodes/node-1" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				if sources := r.URL.Query()["dataSource"]; len(sources) != 1 || sources[0] != "inband" {
					t.Errorf("dataSource = %q, want [inband]", sources)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.response))
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")

			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs([]string{"node", "describe", "node-1", "--data-source", "inband", "--output", "table"})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("command failed: %v", err)
			}
			mu.Lock()
			gotRequests := requests
			mu.Unlock()
			if gotRequests != 1 {
				t.Errorf("requests = %d, want 1", gotRequests)
			}

			rowValue := func(label string) (string, bool) {
				for _, line := range strings.Split(out.String(), "\n") {
					if strings.HasPrefix(line, label+"  ") {
						return strings.TrimSpace(strings.TrimPrefix(line, label)), true
					}
				}
				return "", false
			}
			if got, ok := rowValue("UUID"); !ok || got != "node-1" {
				t.Errorf("UUID row = %q (present %t), want node-1; output: %s", got, ok, out.String())
			}
			for label, want := range resourceRows {
				got, ok := rowValue(label)
				if tc.populated && (!ok || got != want) {
					t.Errorf("%s row = %q (present %t), want %q; output: %s", label, got, ok, want, out.String())
				} else if !tc.populated && ok {
					t.Errorf("unexpected %s row = %q", label, got)
				}
			}
		})
	}
}
