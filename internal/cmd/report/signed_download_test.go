// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"bytes"
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
