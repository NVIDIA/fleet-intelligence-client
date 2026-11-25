// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
)

func TestInventoryBothSourcesPagination(t *testing.T) {
	for _, mode := range []string{"first", "second", "all", "empty"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			var mu sync.Mutex
			requests := map[string][]int{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query()
				source := q.Get("dataSource")
				page, _ := strconv.Atoi(q.Get("page"))
				mu.Lock()
				requests[source] = append(requests[source], page)
				mu.Unlock()
				if r.URL.Path != "/v1/reports/inventory" || q.Get("pageSize") != "1" || q.Get("computeZoneIds") != "cz-1" {
					t.Errorf("unexpected query: %s", r.URL)
				}
				total := 2
				field := "hostname"
				if source == "oob" {
					total = 3
					field = "bmcMac"
				} else if source != "inband" {
					t.Errorf("unexpected source: %s", source)
				}
				rows := fmt.Sprintf(`[{"nodeUUID":"%s-%d",%q:"%s-host-%d"}]`, source, page, field, source, page)
				if mode == "empty" && source == "oob" {
					total = 0
					rows = "[]"
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(w, `{"nodes":%s,"page":%d,"pageSize":1,"total":%d,"hasMore":%v}`, rows, page, total, page+1 < total)
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")
			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			args := []string{"report", "inventory", "--page-size", "1", "--compute-zone-ids", "cz-1"}
			if mode == "second" {
				args = append(args, "--page", "2")
			}
			if mode == "all" {
				args = append(args, "--all")
			}
			cmd.SetArgs(args)
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			sections := strings.Split(out.String(), "Out-of-band\n")
			if len(sections) != 2 || !strings.HasPrefix(sections[0], "In-band\n") {
				t.Fatalf("missing headings: %s", &out)
			}
			if !strings.Contains(sections[0], "GPU TYPE") || !strings.Contains(sections[1], "BMC MAC") || strings.Contains(sections[0], "oob-host-") || strings.Contains(sections[1], "inband-host-") {
				t.Fatalf("mixed source tables: %s", &out)
			}
			mu.Lock()
			defer mu.Unlock()
			for i, source := range []string{"inband", "oob"} {
				wantPages := []int{0}
				if mode == "second" {
					wantPages = []int{1}
				}
				if mode == "all" {
					wantPages = []int{0, 1}
					if source == "oob" {
						wantPages = append(wantPages, 2)
					}
				}
				if fmt.Sprint(requests[source]) != fmt.Sprint(wantPages) {
					t.Fatalf("%s pages: %v, want %v", source, requests[source], wantPages)
				}
				for _, page := range wantPages {
					if mode == "empty" && source == "oob" {
						continue
					}
					if !strings.Contains(sections[i], fmt.Sprintf("%s-host-%d", source, page)) {
						t.Fatalf("missing row: %s", sections[i])
					}
				}
				if mode == "all" {
					if strings.Contains(sections[i], "Total Pages:") {
						t.Fatalf("unexpected page footer: %s", sections[i])
					}
				} else {
					total := i + 2
					if mode == "empty" && source == "oob" {
						total = 0
					}
					if !strings.Contains(sections[i], fmt.Sprintf("Total Entries: %d", total)) {
						t.Fatalf("wrong total: %s", sections[i])
					}
				}
			}
		})
	}
}

func TestInventoryRequiresSourceForJSONAndDownloads(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer server.Close()
	cmdtest.SaveConfig(t, server.URL, "test-key")
	target := filepath.Join(t.TempDir(), "inventory.zip")
	for _, flags := range [][]string{
		{"--output", "json"}, {"--output", "json", "--all"},
		{"--format", "csv"}, {"--format", "csv", "--signed", "--output-path", target},
		{"--format", "csv", "--signed", "--output", "json", "--output-path", target},
	} {
		var out bytes.Buffer
		cmd := newRootCmd()
		cmd.SetOut(&out)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs(append([]string{"report", "inventory"}, flags...))
		if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "--data-source is required") {
			t.Fatalf("%v: unexpected validation: %v", flags, err)
		}
		if out.Len() != 0 {
			t.Fatalf("unexpected output: %s", &out)
		}
	}
	if requests != 0 {
		t.Fatalf("made %d HTTP requests", requests)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("unexpected download file: %v", err)
	}
}

func TestInventoryBothSourcesFailureWritesNoPartialOutput(t *testing.T) {
	for _, source := range []string{"inband", "oob"} {
		t.Run(source, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Query().Get("dataSource") == source {
					w.WriteHeader(http.StatusForbidden)
					fmt.Fprint(w, `{"error":"Forbidden"}`)
					return
				}
				fmt.Fprint(w, `{"nodes":[],"total":0,"hasMore":false}`)
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")
			for _, all := range []bool{false, true} {
				var out bytes.Buffer
				cmd := newRootCmd()
				cmd.SetOut(&out)
				cmd.SetErr(&bytes.Buffer{})
				args := []string{"report", "inventory"}
				if all {
					args = append(args, "--all")
				}
				cmd.SetArgs(args)
				label := "In-band"
				if source == "oob" {
					label = "Out-of-band"
				}
				if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "fetch "+label+" inventory report") {
					t.Fatalf("missing source error: %v", err)
				}
				if out.Len() != 0 {
					t.Fatalf("partial output: %s", &out)
				}
			}
		})
	}
}
