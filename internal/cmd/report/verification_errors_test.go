// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
)

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
