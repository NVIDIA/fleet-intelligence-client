// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package node

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
)

func TestNodeOptionsBothSources(t *testing.T) {
	for _, explicitDetail := range []bool{false, true} {
		t.Run(fmt.Sprintf("detail=%v", explicitDetail), func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			var mu sync.Mutex
			requests := map[string]int{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query()
				source := q.Get("dataSource")
				mu.Lock()
				requests[source]++
				mu.Unlock()
				if r.URL.Path != "/v1/nodes/options" || q.Get("view") != "detail" || q.Has("agentType") {
					t.Errorf("unexpected request: %s", r.URL)
				}
				sortField := "hostname"
				switch source {
				case "inband":
				case "oob":
					sortField = "bmcHostname"
				default:
					t.Errorf("unexpected source: %q", source)
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(w, `{"filters":{"fields":[{"name":"gpuTypes","options":[%q]}]},"sorting":{"fields":[%q]},"futureField":%q}`, source+"-gpu", sortField, source)
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")
			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			args := []string{"node", "options", "--output", "table"}
			if explicitDetail {
				args = append(args, "--view", "detail")
			}
			cmd.SetArgs(args)
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			mu.Lock()
			defer mu.Unlock()
			if len(requests) != 2 || requests["inband"] != 1 || requests["oob"] != 1 {
				t.Fatalf("expected one request per source: %v", requests)
			}
			sections := strings.Split(out.String(), "Out-of-band\n")
			if len(sections) != 2 || !strings.HasPrefix(sections[0], "In-band\n") {
				t.Fatalf("missing source headings: %s", &out)
			}
			if !strings.Contains(sections[0], "inband-gpu") || strings.Contains(sections[0], "oob-gpu") || !strings.Contains(sections[1], "oob-gpu") || strings.Contains(sections[1], "inband-gpu") {
				t.Fatalf("options mixed across sources: %s", &out)
			}
		})
	}
}

func TestNodeOptionsBothSourcesFailureDoesNotWritePartialOutput(t *testing.T) {
	for _, failingSource := range []string{"inband", "oob"} {
		t.Run(failingSource, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Query().Get("dataSource") == failingSource {
					w.WriteHeader(http.StatusForbidden)
					fmt.Fprint(w, `{"error":"Forbidden"}`)
					return
				}
				fmt.Fprint(w, `{"filters":{"fields":[]},"sorting":{"fields":[]}}`)
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")
			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs([]string{"node", "options", "--output", "table"})
			err := cmd.Execute()
			label := "In-band"
			if failingSource == "oob" {
				label = "Out-of-band"
			}
			if err == nil || !strings.Contains(err.Error(), "fetch "+label+" node options") {
				t.Fatalf("missing source error: %v", err)
			}
			if out.Len() != 0 {
				t.Fatalf("partial output on failure: %s", &out)
			}
		})
	}
}
