// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package node

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
)

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

func TestNodeDescribeCombinedSelectedInventorySections(t *testing.T) {
	for _, tc := range []struct {
		name         string
		inbandStatus int
	}{
		{name: "inband present", inbandStatus: http.StatusOK},
		{name: "inband missing", inbandStatus: http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			var mu sync.Mutex
			counts := map[string]int{"inband": 0, "oob": 0}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/nodes/node-1" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
					http.Error(w, "unexpected request", http.StatusBadRequest)
					return
				}
				sources := r.URL.Query()["dataSource"]
				if len(sources) != 1 || (sources[0] != "inband" && sources[0] != "oob") {
					t.Errorf("unexpected dataSource query: %q", sources)
					http.Error(w, "unexpected source", http.StatusBadRequest)
					return
				}
				source := sources[0]
				mu.Lock()
				counts[source]++
				mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				if source == "oob" {
					_, _ = w.Write([]byte(completeOOBNodeJSON))
				} else if tc.inbandStatus == http.StatusNotFound {
					w.WriteHeader(http.StatusNotFound)
					_, _ = w.Write([]byte(`{"error":"source not found"}`))
				} else {
					_, _ = w.Write([]byte(`{"nodeUUID":"node-1","dataSource":"inband","hostname":"distinctive-inband-host"}`))
				}
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")

			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs([]string{"node", "describe", "node-1", "--section", "systems,chassis"})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("command failed: %v", err)
			}
			mu.Lock()
			for _, source := range []string{"inband", "oob"} {
				if counts[source] != 1 {
					t.Errorf("%s requests = %d, want 1", source, counts[source])
				}
			}
			mu.Unlock()
			got := out.String()
			for _, want := range []string{"Systems", "system-1", "CPUs", "cpu-1", "GPUs", "gpu-1", "Chassis", "chassis-1"} {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q: %q", want, got)
				}
			}
			for _, unwanted := range []string{"distinctive-inband-host", "In-band", "Out-of-band"} {
				if strings.Contains(got, unwanted) {
					t.Errorf("output contains %q: %q", unwanted, got)
				}
			}
			for _, line := range strings.Split(got, "\n") {
				fields := strings.Fields(line)
				if len(fields) > 0 && fields[0] == "FIELD" {
					t.Errorf("output contains summary header: %q", line)
				}
				for _, heading := range []string{"Managers", "Firmware", "Fabrics"} {
					if strings.TrimSpace(line) == heading {
						t.Errorf("output contains unselected heading %q", heading)
					}
				}
			}
		})
	}
}

// These cases protect combined versus source-specific detail behavior for
// Node describe source selection, including optional OOB data (docs/cli.md).
func TestNodeDescribeSourceFailuresAndJSON(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		statuses   map[string]int
		counts     map[string]int
		wantErrors []string
		jsonMode   string
	}{
		{name: "combined inband forbidden", statuses: map[string]int{"inband": 403, "oob": 200}, counts: map[string]int{"inband": 1, "oob": 1}, wantErrors: []string{"fetch in-band node view", "source access denied"}},
		{name: "combined oob forbidden", statuses: map[string]int{"inband": 200, "oob": 403}, counts: map[string]int{"inband": 1, "oob": 1}, wantErrors: []string{"fetch OOB node view", "source access denied"}},
		{name: "both missing", statuses: map[string]int{"inband": 404, "oob": 404}, counts: map[string]int{"inband": 1, "oob": 1}, wantErrors: []string{`node "node-1" was not found in either the in-band or OOB view`}},
		{name: "explicit inband forbidden", args: []string{"--data-source", "inband"}, statuses: map[string]int{"inband": 403}, counts: map[string]int{"inband": 1, "oob": 0}, wantErrors: []string{"source access denied"}},
		{name: "oob section without inventory", args: []string{"--data-source", "oob", "--section", "systems"}, statuses: map[string]int{"oob": 200}, counts: map[string]int{"inband": 0, "oob": 1}, wantErrors: []string{"does not have an OOB view for the requested section"}},
		{name: "combined json with only oob", args: []string{"--output", "json"}, statuses: map[string]int{"inband": 404, "oob": 200}, counts: map[string]int{"inband": 1, "oob": 1}, jsonMode: "combined"},
		{name: "explicit inband json", args: []string{"--data-source", "inband", "--output", "json"}, statuses: map[string]int{"inband": 200}, counts: map[string]int{"inband": 1, "oob": 0}, jsonMode: "inband"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			var mu sync.Mutex
			counts := map[string]int{"inband": 0, "oob": 0}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/nodes/node-1" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
					http.Error(w, "unexpected request", http.StatusBadRequest)
					return
				}
				sources := r.URL.Query()["dataSource"]
				if len(sources) != 1 || (sources[0] != "inband" && sources[0] != "oob") {
					t.Errorf("unexpected dataSource query: %q", sources)
					http.Error(w, "unexpected source", http.StatusBadRequest)
					return
				}
				source := sources[0]
				mu.Lock()
				counts[source]++
				mu.Unlock()
				status, ok := tc.statuses[source]
				if !ok {
					t.Errorf("unexpected source request: %s", source)
					http.Error(w, "unexpected source", http.StatusBadRequest)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				switch status {
				case http.StatusForbidden:
					_, _ = w.Write([]byte(`{"error":"source access denied"}`))
				case http.StatusNotFound:
					_, _ = w.Write([]byte(`{"error":"source not found"}`))
				default:
					if source == "inband" {
						_, _ = w.Write([]byte(`{"nodeUUID":"node-1","dataSource":"inband","hostname":"gpu-001","futureField":{"preserved":true}}`))
					} else {
						_, _ = w.Write([]byte(`{"nodeUUID":"node-1","dataSource":"oob"}`))
					}
				}
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")

			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs(append([]string{"node", "describe", "node-1"}, tc.args...))
			err := cmd.Execute()
			if len(tc.wantErrors) > 0 {
				if err == nil {
					t.Fatal("expected command error")
				}
				for _, want := range tc.wantErrors {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error %q does not contain %q", err, want)
					}
				}
				if out.Len() != 0 {
					t.Errorf("expected empty stdout, got %q", out.String())
				}
			} else if err != nil {
				t.Errorf("command failed: %v", err)
			} else {
				var got map[string]any
				if err := json.Unmarshal(out.Bytes(), &got); err != nil {
					t.Errorf("decode output: %v", err)
				} else if tc.jsonMode == "combined" {
					if _, exists := got["inband"]; exists {
						t.Errorf("unexpected inband object: %#v", got)
					}
					oob, ok := got["oob"].(map[string]any)
					if !ok || oob["nodeUUID"] != "node-1" || oob["dataSource"] != "oob" {
						t.Errorf("unexpected OOB object: %#v", got["oob"])
					}
				} else {
					future, ok := got["futureField"].(map[string]any)
					if got["nodeUUID"] != "node-1" || got["hostname"] != "gpu-001" || !ok || future["preserved"] != true {
						t.Errorf("unexpected inband JSON: %#v", got)
					}
					for _, key := range []string{"inband", "oob"} {
						if _, exists := got[key]; exists {
							t.Errorf("unexpected %s wrapper: %#v", key, got)
						}
					}
				}
			}
			mu.Lock()
			defer mu.Unlock()
			for _, source := range []string{"inband", "oob"} {
				if counts[source] != tc.counts[source] {
					t.Errorf("%s requests = %d, want %d", source, counts[source], tc.counts[source])
				}
			}
		})
	}
}
