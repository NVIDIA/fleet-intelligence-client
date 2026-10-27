// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
)

func TestInventoryOOBSourceOutput(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		q := r.URL.Query()
		if q.Get("dataSource") != "oob" || q.Has("agentType") {
			t.Errorf("unexpected source query: %s", r.URL.RawQuery)
		}
		if q.Get("sortBy") != "gpuCount" {
			t.Error("missing OOB GPU sort field")
		}
		w.Header().Set("Content-Type", "application/json")
		page := 0
		if q.Get("page") == "1" {
			page = 1
		}
		fmt.Fprintf(w, `{"nodes":[{"nodeUUID":"n%d","nodeName":"machine%d","bmcMac":"bmc%d","bmcIP":"192.0.2.%d","gpuType":"NVIDIA-H100-80GB-HBM3","gpuCount":8,"verificationCheck":"Verified","enrolledAt":"2026-01-01T00:00:00Z","removedAt":"2026-09-01T00:00:00Z"}],"page":%d,"pageSize":1,"total":2,"hasMore":%v}`, page, page, page, page+1, page, page == 0)
	}))
	defer server.Close()
	cmdtest.SaveConfig(t, server.URL, "test-key")
	for _, all := range []bool{false, true} {
		for _, output := range []string{"table", "json"} {
			before := requests
			cmd := newRootCmd()
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&bytes.Buffer{})
			args := []string{"report", "inventory", "--data-source", "oob", "--sort-by", "gpuCount", "--output", output, "--page-size", "1"}
			if all {
				args = append(args, "--all")
			}
			cmd.SetArgs(args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("%v: %v", args, err)
			}
			wantRequests := 1
			if all {
				wantRequests = 2
			}
			if requests-before != wantRequests {
				t.Fatalf("requests=%d, want %d", requests-before, wantRequests)
			}
			for _, want := range []string{"machine0", "bmc0", "192.0.2.1", "Verified", "NVIDIA-H100-80GB-HBM3"} {
				if !strings.Contains(out.String(), want) {
					t.Fatalf("missing %s: %s", want, &out)
				}
			}
			if all && !strings.Contains(out.String(), "machine1") {
				t.Fatalf("missing second page: %s", &out)
			}
			if output == "json" && (!json.Valid(out.Bytes()) || !strings.Contains(out.String(), "removedAt")) {
				t.Fatalf("invalid/lossy JSON: %s", &out)
			}
			if output == "table" && (!strings.Contains(out.String(), "BMC MAC") || !strings.Contains(out.String(), "GPU TYPE") || !strings.Contains(out.String(), "GPU COUNT")) {
				t.Fatalf("wrong source columns: %s", &out)
			}
		}
	}
	before := requests
	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"report", "inventory", "--data-source", "inband", "--agent-type", "oob"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "unknown flag: --agent-type") {
		t.Fatalf("removed flag: %v", err)
	}
	if before != requests {
		t.Fatal("removed flag made an HTTP request")
	}
}

func TestInventoryOOBDownloads(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, signed := range []bool{false, true} {
		t.Run(fmt.Sprintf("signed=%v", signed), func(t *testing.T) {
			payload := "nodeUUID,nodeName,bmcMac,gpuType,gpuCount\nn1,machine,bmc,NVIDIA-H100-80GB-HBM3,8\n"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("dataSource") != "oob" {
					t.Error("download lost OOB source")
				}
				contentType := "text/csv"
				if signed {
					contentType = "application/zip"
				}
				w.Header().Set("Content-Type", contentType)
				fmt.Fprint(w, payload)
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")
			cmd := newRootCmd()
			var out bytes.Buffer
			cmd.SetOut(&out)
			args := []string{"report", "inventory", "--data-source", "oob", "--format", "csv"}
			path := filepath.Join(t.TempDir(), "inventory.zip")
			if signed {
				args = append(args, "--signed", "--output-path", path)
			}
			cmd.SetArgs(args)
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			got := out.Bytes()
			if signed {
				var err error
				got, err = os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
			}
			if string(got) != payload {
				t.Fatalf("download changed: %q", got)
			}
		})
	}
}
