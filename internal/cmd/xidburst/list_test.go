// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package xidburst

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
)

// Verifies xidburst list table output and filter translation
func TestXIDBurstListTable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/xid/bursts/count" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"total":1}`))
			return
		}
		if r.URL.Path != "/v1/xid/bursts/page" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		query := r.URL.Query()
		if got := query.Get("timeMode"); got != "absolute" {
			t.Fatalf("unexpected timeMode: %q", got)
		}
		if got := query.Get("window"); got != "" {
			t.Fatalf("unexpected window: %q", got)
		}
		if got := query["xidNumbers"]; len(got) != 2 || got[0] != "48" || got[1] != "94" {
			t.Fatalf("unexpected xidNumbers: %#v", got)
		}
		if got := query.Get("jobDisruption"); got != "true" {
			t.Fatalf("unexpected jobDisruption: %q", got)
		}
		if got := query.Get("sortBy"); got != "startTime" {
			t.Fatalf("unexpected sortBy: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(xidBurstsBody))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{
		"xidburst", "list", "--window", "24h", "--xid-numbers", "48,94",
		"--job-disruption", "--sort-by", "startTime",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"BURST ID", "HOSTNAME", "XIDS", "START TIME", "DURATION (S)",
		"JOB DISRUPTED", "PLATFORM RELATED", "CATEGORY", "SUBCATEGORY", "GPU", "Memory",
		"burst-1", "gpu-01", "48, 94", "2026-05-01T00:00:00Z", "300", "true",
		"Total Entries: 1",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
	headers := strings.Fields(strings.SplitN(got, "\n", 2)[0])
	wantPrefix := strings.Fields("BURST ID XIDS JOB DISRUPTED PLATFORM RELATED CATEGORY SUBCATEGORY")
	if len(headers) < len(wantPrefix) || !reflect.DeepEqual(headers[:len(wantPrefix)], wantPrefix) {
		t.Fatalf("unexpected column order: %v", headers)
	}
	values := strings.Fields(strings.SplitN(got, "\n", 3)[1])
	wantValues := []string{"burst-1", "48,", "94", "true", "-", "GPU", "Memory"}
	if len(values) < len(wantValues) || !reflect.DeepEqual(values[:len(wantValues)], wantValues) {
		t.Fatalf("unexpected leading values: %v", values)
	}
	// Detail-only fields stay out of the list table.
	for _, absent := range []string{"NODE GROUP", "COMPUTE ZONE", "XID COUNT", "Double Bit ECC", "0000:0f:00.0", "Drain the node"} {
		if strings.Contains(got, absent) {
			t.Fatalf("output should not contain %q:\n%s", absent, got)
		}
	}
}

// Verifies xidburst list JSON output is the raw payload with a 1-based page
func TestXIDBurstListJSON(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(xidBurstsBody))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"xidburst", "list", "--window", "24h", "--output", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	got := strings.TrimSpace(out.String())
	if !strings.Contains(got, `"page":1`) {
		t.Fatalf("expected 1-based page in JSON output:\n%s", got)
	}
	// The raw backend payload is preserved, including the fields the table drops.
	for _, want := range []string{`"burstId":"burst-1"`, `"deviceIds"`, `"Double Bit ECC"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

// Verifies the exclusion filter flags reach the backend as query parameters
func TestXIDBurstListExclusionFlags(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	var query url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/xid/bursts/count" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"total":1}`))
			return
		}
		query = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(xidBurstsBody))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{
		"xidburst", "list", "--window", "24h",
		"--exclude-nodegroup-ids", "ng-1,ng-2",
		"--exclude-compute-zone-ids", "cz-1",
		"--sort-by", "xidCount",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	if got := query["excludeNodeGroupIds"]; !reflect.DeepEqual(got, []string{"ng-1", "ng-2"}) {
		t.Fatalf("unexpected excludeNodeGroupIds: %#v", got)
	}
	if got := query["excludeComputeZoneIds"]; !reflect.DeepEqual(got, []string{"cz-1"}) {
		t.Fatalf("unexpected excludeComputeZoneIds: %#v", got)
	}
	if got := query.Get("sortBy"); got != "xidCount" {
		t.Fatalf("unexpected sortBy: %q", got)
	}
}

func TestXIDBurstPlatformDisruptionFilterBoundaries(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	type observedRequest struct {
		path  string
		query url.Values
	}
	queries := make(chan observedRequest, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case queries <- observedRequest{r.URL.Path, r.URL.Query()}:
		default:
			t.Error("unexpected extra API request while recorder is full")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/xid/bursts/page":
			_, _ = w.Write([]byte(xidBurstsBody))
		case "/v1/xid/bursts/count":
			_, _ = w.Write([]byte(`{"total":1}`))
		default:
			t.Errorf("unexpected endpoint: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	cmdtest.SaveConfig(t, server.URL, "test-key")

	for _, tt := range []struct {
		name string
		flag string
		want []string
	}{
		{name: "omitted"},
		{name: "false", flag: "--platform-disruption=false", want: []string{"false"}},
		{name: "true", flag: "--platform-disruption=true", want: []string{"true"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			args := []string{"xidburst", "list", "--window", "24h"}
			if tt.flag != "" {
				args = append(args, tt.flag)
			}
			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs(args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("command failed: %v", err)
			}
			for _, path := range []string{"/v1/xid/bursts/page", "/v1/xid/bursts/count"} {
				select {
				case observed := <-queries:
					if observed.path != path {
						t.Fatalf("path = %s, want %s", observed.path, path)
					}
					got, present := observed.query["jobDisruptionDueToPlatformIssue"]
					if present != (tt.want != nil) || !reflect.DeepEqual(got, tt.want) {
						t.Fatalf("%s platform disruption query: got %v (present %t), want %v", path, got, present, tt.want)
					}
				default:
					t.Fatalf("expected request to %s", path)
				}
			}
			if !strings.Contains(out.String(), "burst-1") {
				t.Fatalf("output missing burst-1: %s", out.String())
			}
		})
	}
}
