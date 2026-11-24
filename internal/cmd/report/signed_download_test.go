// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
)

// Verifies a signed inventory report is written to the current directory
func TestReportInventorySignedWritesToCWD(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	payload := []byte("PK\x03\x04 signed-zip-bytes")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept"); got != "application/zip" {
			t.Fatalf("unexpected accept header: %q", got)
		}
		query := r.URL.Query()
		if got := query.Get("format"); got != "csv" {
			t.Fatalf("unexpected format: %q", got)
		}
		if got := query.Get("signed"); got != "true" {
			t.Fatalf("unexpected signed: %q", got)
		}

		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="fleet-inventory.zip"`)
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	dir := t.TempDir()
	chdir(t, dir)

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"report", "inventory", "--data-source", "inband", "--format", "csv", "--signed"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	written, err := os.ReadFile(filepath.Join(dir, "fleet-inventory.zip"))
	if err != nil {
		t.Fatalf("read written file failed: %v", err)
	}
	if !bytes.Equal(written, payload) {
		t.Fatalf("unexpected file contents: %q", string(written))
	}
	if !strings.Contains(out.String(), "fleet-inventory.zip") {
		t.Fatalf("output missing written path: %q", out.String())
	}
}

func TestReportInventorySignedJSONStatus(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	payload := []byte("PK\x03\x04 signed-zip-bytes")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="fleet-inventory.zip"`)
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	dir := t.TempDir()
	chdir(t, dir)

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"report", "inventory", "--data-source", "inband", "--format", "csv", "--signed", "--output", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	var got signedReportOutput
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode signed report JSON failed: %v", err)
	}
	if got.Status != "written" || !strings.HasSuffix(got.Path, "fleet-inventory.zip") {
		t.Fatalf("unexpected signed report JSON: %#v", got)
	}
}

// Verifies --output-path directs the signed bundle to an explicit file
func TestReportInventorySignedOutputPath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	payload := []byte("PK\x03\x04 signed-zip-bytes")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="fleet-inventory.zip"`)
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	dir := t.TempDir()
	target := filepath.Join(dir, "custom-name.zip")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"report", "inventory", "--data-source", "inband", "--format", "csv", "--signed", "--output-path", target})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	written, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read written file failed: %v", err)
	}
	if !bytes.Equal(written, payload) {
		t.Fatalf("unexpected file contents: %q", string(written))
	}
}

// Verifies --output-path pointing at a directory keeps the suggested filename
func TestReportInventorySignedOutputPathDirectory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	payload := []byte("PK\x03\x04 signed-zip-bytes")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="fleet-inventory.zip"`)
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	dir := t.TempDir()

	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"report", "inventory", "--data-source", "inband", "--format", "csv", "--signed", "--output-path", dir})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	written, err := os.ReadFile(filepath.Join(dir, "fleet-inventory.zip"))
	if err != nil {
		t.Fatalf("read written file failed: %v", err)
	}
	if !bytes.Equal(written, payload) {
		t.Fatalf("unexpected file contents: %q", string(written))
	}
}

func TestReportInventorySignedWriteFailure(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	type request struct {
		method, path, accept string
		query                url.Values
	}
	requests := make(chan request, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- request{r.Method, r.URL.Path, r.Header.Get("Accept"), r.URL.Query()}
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write([]byte("PK\x03\x04 signed-zip-bytes"))
	}))
	defer server.Close()
	cmdtest.SaveConfig(t, server.URL, "test-key")

	target := filepath.Join(t.TempDir(), "missing", "inventory.zip")
	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"report", "inventory", "--data-source", "inband", "--format", "csv", "--signed", "--output", "json", "--output-path", target})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "write signed report") || !errors.Is(err, os.ErrNotExist) {
		t.Errorf("error = %v, want write signed report wrapping os.ErrNotExist", err)
	}
	if out.Len() != 0 {
		t.Errorf("unexpected output after write failure: %q", out.String())
	}
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("destination stat error = %v, want os.ErrNotExist", err)
	}
	select {
	case req := <-requests:
		if req.method != http.MethodGet || req.path != "/v1/reports/inventory" || req.accept != "application/zip" {
			t.Errorf("request: %s %s Accept=%q", req.method, req.path, req.accept)
		}
		for key, want := range map[string]string{"dataSource": "inband", "format": "csv", "signed": "true"} {
			if got := req.query.Get(key); got != want {
				t.Errorf("query %s = %q, want %q", key, got, want)
			}
		}
	default:
		t.Error("signed inventory request not received")
	}
}

func TestWriteSignedReportDefaultFilename(t *testing.T) {
	dir := t.TempDir()
	payload := []byte("PK\x03\x04 known signed bundle")
	path, err := writeSignedReport(dir, "", payload)
	if err != nil {
		t.Fatalf("write signed report: %v", err)
	}
	if want := filepath.Join(dir, "inventory-report.zip"); path != want {
		t.Errorf("path = %q, want %q", path, want)
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read signed report: %v", err)
	}
	if !bytes.Equal(written, payload) {
		t.Errorf("contents = %q, want %q", written, payload)
	}
}
