// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
)

// Verifies a tampered CSV fails the verify command
func TestReportVerifyTamperedFails(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	dir := t.TempDir()
	csvPath, bundlePath, keyPath := writeSignedFixture(t, dir, []byte("customer,issued_at\nacme,2026-06-15T00:00:00Z\n"))
	if err := os.WriteFile(csvPath, []byte("customer,issued_at\nevil,2026-06-15T00:00:00Z\n"), 0o644); err != nil {
		t.Fatalf("tamper csv failed: %v", err)
	}

	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"report", "verify", "--csv", csvPath, "--bundle", bundlePath, "--key", keyPath})

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected verification to fail for tampered csv")
	}
}

// Verifies a clear message when a non-bundle file is passed to --bundle
func TestReportVerifyRejectsNonBundle(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	dir := t.TempDir()
	csvPath, _, keyPath := writeSignedFixture(t, dir, []byte("customer,issued_at\nacme,2026-06-15T00:00:00Z\n"))

	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	// Point --bundle at the CSV instead of the .sig.bundle file.
	cmd.SetArgs([]string{"report", "verify", "--csv", csvPath, "--bundle", csvPath, "--key", keyPath})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not a valid signature bundle") {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(err.Error(), "proto:") {
		t.Fatalf("error leaks internal details: %v", err)
	}
}

// Verifies a clear message when the report does not match the signature
func TestReportVerifyMismatchMessage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	dir := t.TempDir()
	csvPath, bundlePath, keyPath := writeSignedFixture(t, dir, []byte("customer,issued_at\nacme,2026-06-15T00:00:00Z\n"))
	// Point --csv at the bundle file: a valid bundle, but not the signed artifact.
	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"report", "verify", "--csv", bundlePath, "--bundle", bundlePath, "--key", keyPath})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "verification failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(err.Error(), "ASN.1") {
		t.Fatalf("error leaks internal details: %v", err)
	}
	_ = csvPath
}

// Verifies a clear message when a flag points at a missing file
func TestReportVerifyMissingFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	dir := t.TempDir()
	csvPath, bundlePath, _ := writeSignedFixture(t, dir, []byte("data\n"))

	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"report", "verify", "--csv", csvPath, "--bundle", bundlePath, "--key", "test-key"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), `--key file "test-key" does not exist`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReportVerifyArtifactAndKeyErrors(t *testing.T) {
	cases := []struct {
		name string
	}{
		{name: "missing csv"},
		{name: "missing bundle"},
		{name: "csv is a directory"},
		{name: "invalid local key"},
		{name: "invalid fetched key"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			cmdtest.ClearCredentialEnv(t)
			dir := t.TempDir()
			csvPath, bundlePath, keyPath := writeSignedFixture(t, dir, []byte("customer,issued_at\nacme,2026-06-15T00:00:00Z\n"))
			var want []string
			var requests chan string
			switch tc.name {
			case "missing csv":
				csvPath = filepath.Join(dir, "missing.csv")
				want = []string{"--csv", csvPath, "does not exist"}
			case "missing bundle":
				bundlePath = filepath.Join(dir, "missing.sig.bundle")
				want = []string{"--bundle", bundlePath, "does not exist"}
			case "csv is a directory":
				csvPath = dir
				want = []string{"cannot read --csv", csvPath}
			case "invalid local key":
				if err := os.WriteFile(keyPath, []byte("invalid PEM"), 0o644); err != nil {
					t.Fatalf("write invalid key: %v", err)
				}
				want = []string{keyPath, "not a valid PEM public key"}
			case "invalid fetched key":
				requests = make(chan string, 1)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					select {
					case requests <- r.URL.Path:
					default:
					}
					_, _ = w.Write([]byte("invalid PEM"))
				}))
				defer server.Close()
				cmdtest.SaveConfig(t, server.URL, "test-key")
				keyPath = ""
				want = []string{"the signing key fetched from the API is not a valid PEM public key"}
			}

			args := []string{"report", "verify", "--csv", csvPath, "--bundle", bundlePath}
			if keyPath != "" {
				args = append(args, "--key", keyPath)
			}
			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetArgs(args)
			err := cmd.Execute()
			if err == nil {
				t.Fatal("command succeeded, want an error")
			}
			for _, part := range want {
				if !strings.Contains(err.Error(), part) {
					t.Errorf("error = %q, want it to contain %q", err, part)
				}
			}
			if tc.name == "csv is a directory" && strings.Contains(err.Error(), "does not exist") {
				t.Errorf("directory reported as missing: %v", err)
			}
			if out.Len() != 0 {
				t.Errorf("unexpected output after failure: %q", out.String())
			}
			if requests != nil {
				select {
				case path := <-requests:
					if path != "/.well-known/signing-key.pub" {
						t.Errorf("requested path = %q, want signing-key endpoint", path)
					}
				default:
					t.Error("signing-key endpoint was not requested")
				}
			}
		})
	}
}

func TestReportVerifyTimeoutPrecedesArtifactAccess(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	cmdtest.ClearCredentialEnv(t)
	dir := t.TempDir()
	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"report", "verify", "--timeout", "0", "--csv", filepath.Join(dir, "missing.csv"), "--bundle", filepath.Join(dir, "missing.bundle")})
	err := cmd.Execute()
	if err == nil || err.Error() != "--timeout must be greater than 0" {
		t.Errorf("error = %v, want timeout validation error", err)
	}
	if out.Len() != 0 {
		t.Errorf("unexpected output after failure: %q", out.String())
	}
}

func TestReportVerifyErrorPreservesUnexpectedError(t *testing.T) {
	sentinel := errors.New("unexpected verification error")
	wrapped := fmt.Errorf("verification context: %w", sentinel)
	got := reportVerifyError(reportVerifyFlags{}, wrapped)
	if got != wrapped || !errors.Is(got, sentinel) {
		t.Errorf("error = %v, want unchanged wrapped sentinel %v", got, wrapped)
	}
}
