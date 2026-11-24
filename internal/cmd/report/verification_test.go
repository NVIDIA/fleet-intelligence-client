// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
)

// Verifies a signed report passes verification with an offline --key
func TestReportVerifySucceedsWithKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	dir := t.TempDir()
	csvPath, bundlePath, keyPath := writeSignedFixture(t, dir, []byte("customer,issued_at\nacme,2026-06-15T00:00:00Z\n"))

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"report", "verify", "--csv", csvPath, "--bundle", bundlePath, "--key", keyPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}
	if !strings.Contains(out.String(), "Verified OK") {
		t.Fatalf("output missing success message: %q", out.String())
	}
}

func TestReportVerifySucceedsWithKeyJSON(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	dir := t.TempDir()
	csvPath, bundlePath, keyPath := writeSignedFixture(t, dir, []byte("customer,issued_at\nacme,2026-06-15T00:00:00Z\n"))

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"report", "verify", "--csv", csvPath, "--bundle", bundlePath, "--key", keyPath, "--output", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	var got commandStatusOutput
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode verify JSON failed: %v", err)
	}
	if got.Status != "verified" {
		t.Fatalf("unexpected verify JSON: %#v", got)
	}
}

// Verifies the signing key is fetched from the API when --key is omitted
func TestReportVerifyFetchesKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	dir := t.TempDir()
	csvPath, bundlePath, keyPath := writeSignedFixture(t, dir, []byte("customer,issued_at\nacme,2026-06-15T00:00:00Z\n"))
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("read key fixture failed: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/signing-key.pub" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write(keyPEM)
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"report", "verify", "--csv", csvPath, "--bundle", bundlePath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}
	if !strings.Contains(out.String(), "Verified OK") {
		t.Fatalf("output missing success message: %q", out.String())
	}
}

// Verifies required flags are enforced
func TestReportVerifyRequiresFlags(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	tests := []struct {
		name string
		args []string
		want string
	}{
		{"missing csv", []string{"report", "verify", "--bundle", "b.sig.bundle"}, "--csv is required"},
		{"missing bundle", []string{"report", "verify", "--csv", "report.csv"}, "--bundle is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newRootCmd()
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetArgs(tt.args)

			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
