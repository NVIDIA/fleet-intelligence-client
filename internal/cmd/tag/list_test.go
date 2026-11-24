// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package tag

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

// Verifies tag list table output and prefix filter translation
func TestTagListTable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/tags" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("prefix"); got != "gpu" {
			t.Fatalf("unexpected prefix: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tags":["gpu-health","gpu-burn"]}`))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"tag", "list", "--prefix", "gpu"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{"TAG", "gpu-health", "gpu-burn"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

// Verifies tag list JSON output emits the raw backend payload
func TestTagListJSON(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	body := `{"tags":["gpu-health","gpu-burn"]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"tag", "list", "--output", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}
	if strings.TrimSpace(out.String()) != body {
		t.Fatalf("unexpected JSON:\n%s", out.String())
	}
}

func TestTagListTrimmedComputeZoneFilter(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	var rec tagRequestRecorder
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.record(r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tags":["gpu-health"]}`))
	}))
	defer server.Close()
	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"tag", "list", "--prefix", "  gpu  ", "--computezone", "  zone-1  "})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}
	want := []tagRecordedRequest{{http.MethodGet, "/v1/tags", url.Values{"prefix": {"gpu"}, "computeZoneId": {"zone-1"}}}}
	if got := rec.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("requests = %+v, want %+v", got, want)
	}
	if !strings.Contains(out.String(), "gpu-health") {
		t.Fatalf("output missing tag: %q", out.String())
	}
}
