// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package node

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
)

func TestNodeSourceFlags(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		q := r.URL.Query()
		if q.Get("dataSource") != "oob" || q.Has("agentType") {
			t.Errorf("unexpected source query: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		const node = `{"nodeUUID":"n1","dataSource":"oob","nodeName":"machine","bmcMac":"bmc","gpuType":"H100","gpuCount":8}`
		switch r.URL.Path {
		case "/v1/nodes/n1":
			fmt.Fprint(w, node)
		case "/v1/nodes/options":
			fmt.Fprint(w, `{"filters":{"fields":[]},"sorting":{"fields":[]}}`)
		default:
			fmt.Fprintf(w, `{"nodes":[%s],"page":0,"pageSize":20,"total":1}`, node)
		}
	}))
	defer server.Close()
	cmdtest.SaveConfig(t, server.URL, "test-key")
	for _, command := range [][]string{{"list"}, {"describe", "n1"}, {"options"}} {
		var out, stderr bytes.Buffer
		cmd := newRootCmd()
		cmd.SetOut(&out)
		cmd.SetErr(&stderr)
		args := append([]string{"node"}, command...)
		args = append(args, "--data-source", "oob")
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if command[0] != "options" && (!strings.Contains(out.String(), "H100") || !strings.Contains(out.String(), "GPU COUNT")) {
			t.Fatalf("OOB GPU fields missing: %s", &out)
		}
		before := requests
		cmd = newRootCmd()
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		args = append([]string{"node"}, command...)
		cmd.SetArgs(append(args, "--agent-type", "oob"))
		if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "unknown flag: --agent-type") {
			t.Fatalf("removed flag error: %v", err)
		}
		if requests != before {
			t.Fatal("removed flag made an HTTP request")
		}
	}
}

func TestNodeBasicCombinedGPUQueries(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("view") != "basic" || q.Has("dataSource") || q.Has("agentType") {
			t.Errorf("unexpected combined query: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/nodes/options" {
			fmt.Fprint(w, `{"filters":{"fields":[{"name":"gpuTypes","options":["H100"]}]},"sorting":{"fields":["gpuType","gpuCount","nodeGroup","computeZone"]}}`)
			return
		}
		if q.Get("gpuTypes") != "H100" || q.Get("gpuCounts") != "8" || q.Get("sortBy") != "gpuCount" {
			t.Errorf("lost GPU filters/sort: %s", r.URL.RawQuery)
		}
		fmt.Fprint(w, `{"nodes":[{"nodeUUID":"n1","nodeName":"machine"}],"total":1}`)
	}))
	defer server.Close()
	cmdtest.SaveConfig(t, server.URL, "test-key")
	for _, args := range [][]string{
		{"node", "list", "--view", "basic", "--gpu-type", "H100", "--gpu-count", "8", "--sort-by", "gpuCount"},
		{"node", "options", "--view", "basic"},
	} {
		cmd := newRootCmd()
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetArgs(append(args, "--output", "json"))
		if err := cmd.Execute(); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
}

func TestNodeDetailJSONRequiresSourceBeforeHTTP(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer server.Close()
	cmdtest.SaveConfig(t, server.URL, "test-key")
	for _, command := range []string{"list", "options"} {
		for _, explicitDetail := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/detail=%v", command, explicitDetail), func(t *testing.T) {
				variants := [][]string{nil}
				if command == "list" {
					variants = append(variants, []string{"--all"})
				}
				for _, extra := range variants {
					var out bytes.Buffer
					cmd := newRootCmd()
					cmd.SetOut(&out)
					cmd.SetErr(&bytes.Buffer{})
					args := append([]string{"node", command, "--output", "json"}, extra...)
					if explicitDetail {
						args = append(args, "--view", "detail")
					}
					cmd.SetArgs(args)
					if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "--data-source is required for detail JSON output") {
						t.Fatalf("unexpected validation: %v", err)
					}
					if out.Len() != 0 {
						t.Fatalf("unexpected output: %s", &out)
					}
				}
			})
		}
	}
	if requests != 0 {
		t.Fatalf("invalid commands made %d requests", requests)
	}
}

func TestNodeDetailJSONKeepsSingleSourceShape(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if source := r.URL.Query().Get("dataSource"); source != "inband" && source != "oob" {
			t.Errorf("missing source: %s", r.URL)
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/nodes/options" {
			fmt.Fprint(w, `{"filters":{"fields":[]},"sorting":{"fields":[]},"futureField":true}`)
		} else {
			fmt.Fprint(w, `{"nodes":[],"page":0,"pageSize":20,"total":0,"futureField":true}`)
		}
	}))
	defer server.Close()
	cmdtest.SaveConfig(t, server.URL, "test-key")
	for _, command := range []string{"list", "options"} {
		for _, source := range []string{"inband", "oob"} {
			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs([]string{"node", command, "--output", "json", "--data-source", source})
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			var result map[string]json.RawMessage
			if err := json.Unmarshal(out.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			key := "nodes"
			if command == "options" {
				key = "filters"
			}
			if result[key] == nil || result["futureField"] == nil || result["inband"] != nil || result["oob"] != nil {
				t.Fatalf("unexpected JSON shape: %s", &out)
			}
		}
	}
	if requests != 4 {
		t.Fatalf("requests = %d, want 4", requests)
	}
}
