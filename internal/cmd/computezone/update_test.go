// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package computezone

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdutil"
)

// Verifies that per-field flags merge over the stored zone and that the result
// is rendered as a FIELD/VALUE table
func TestComputeZoneUpdateTable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	var requests []string
	var body []byte
	server := updateServer(t, &requests, &body)
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"computezone", "update", "cz-1",
		"--type", "cloud provider",
		"--contact-email", "new@example.com",
		"--location-country", "United States",
		"--yes",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	if !slices.Equal(requests, []string{"GET /v1/computezones", "PUT /v1/computezones"}) {
		t.Fatalf("unexpected requests: %#v", requests)
	}

	want := `{"id":"cz-1","type":"cloud provider","contact":{"email":"new@example.com","pic":"Jane Doe"},` +
		`"location":{"city":"Baltimore","country":"United States","region":"us-east-1","latitude":39.04581234}}`
	if string(body) != want {
		t.Fatalf("unexpected body:\n got %s\nwant %s", body, want)
	}

	got := out.String()
	for _, expected := range []string{
		"FIELD", "VALUE", "cz-1", "cloud provider", "new@example.com", "Jane Doe",
		"Baltimore", "United States", "us-east-1", "39.04581234",
	} {
		if !strings.Contains(got, expected) {
			t.Fatalf("output missing %q:\n%s", expected, got)
		}
	}
}

// Verifies that an empty flag value clears one field and that JSON output is
// the backend's own payload
func TestComputeZoneUpdateClearsFieldJSON(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	var requests []string
	var body []byte
	server := updateServer(t, &requests, &body)
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"computezone", "update", "cz-1", "--contact-pic", "", "--yes", "-o", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	want := `{"id":"cz-1","type":"datacenter","contact":{"email":"ops@example.com"},` +
		`"location":{"city":"Baltimore","region":"us-east-1","latitude":39.04581234}}`
	if string(body) != want {
		t.Fatalf("unexpected body:\n got %s\nwant %s", body, want)
	}

	var decoded map[string]any
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("json output not decodable: %v (%s)", err, out.String())
	}
	if decoded["id"] != "cz-1" {
		t.Fatalf("unexpected json output: %s", out.String())
	}
}

// Verifies that a declined confirmation writes nothing
func TestComputeZoneUpdateDeclinedWritesNothing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	var requests []string
	var body []byte
	server := updateServer(t, &requests, &body)
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	// A strings.Reader is not an *os.File, so the prompt treats it as
	// answerable and reads the refusal from it.
	cmd.SetIn(strings.NewReader("n\n"))
	cmd.SetArgs([]string{"computezone", "update", "cz-1", "--type", "datacenter"})

	err := cmd.Execute()
	if !errors.Is(err, cmdutil.ErrAborted) {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(requests) != 0 {
		t.Fatalf("declined update still issued requests: %#v", requests)
	}
	if !strings.Contains(out.String(), "This updates compute zone cz-1") {
		t.Fatalf("confirmation summary missing:\n%s", out.String())
	}
}

// Verifies --dry-run reports the requested changes without sending a
// request or prompting for confirmation
func TestComputeZoneUpdateDryRun(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	var requests []string
	var body []byte
	server := updateServer(t, &requests, &body)
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"computezone", "update", "cz-1", "--type", "cloud provider", "--dry-run"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}
	// The read-modify-write merge still needs to read the zone; only the PUT
	// is skipped.
	if !slices.Equal(requests, []string{"GET /v1/computezones"}) {
		t.Fatalf("unexpected requests: %#v", requests)
	}
	for _, want := range []string{
		"PUT " + server.URL + "/v1/computezones",
		`"type": "cloud provider"`,
		`"email": "ops@example.com"`,
		"Dry run: no request was sent.",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, out.String())
		}
	}
}

// Verifies --dry-run with -o json emits the method, URL, and body as a
// structured document instead of the backend payload, since no PUT is made
func TestComputeZoneUpdateDryRunJSON(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	var requests []string
	var body []byte
	server := updateServer(t, &requests, &body)
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"computezone", "update", "cz-1", "--contact-pic", "", "--dry-run", "-o", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}
	if !slices.Equal(requests, []string{"GET /v1/computezones"}) {
		t.Fatalf("unexpected requests: %#v", requests)
	}

	var decoded struct {
		Method string         `json:"method"`
		URL    string         `json:"url"`
		Body   map[string]any `json:"body"`
	}
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("json output not decodable: %v (%s)", err, out.String())
	}
	if decoded.Method != http.MethodPut {
		t.Fatalf("unexpected method: %s", decoded.Method)
	}
	if decoded.URL != server.URL+"/v1/computezones" {
		t.Fatalf("unexpected url: %s", decoded.URL)
	}
	if decoded.Body["id"] != "cz-1" {
		t.Fatalf("unexpected body: %#v", decoded.Body)
	}
}

// Verifies the flag validation that runs before any request
func TestComputeZoneUpdateRejectsBadFlags(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	var requests []string
	var body []byte
	server := updateServer(t, &requests, &body)
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	cases := []struct {
		name string
		args []string
		want string
	}{
		{"no fields", []string{"computezone", "update", "cz-1", "--yes"}, "no changes requested"},
		{"bad type", []string{"computezone", "update", "cz-1", "--type", "bogus", "--yes"}, `invalid type "bogus"`},
		{"bad latitude", []string{"computezone", "update", "cz-1", "--location-latitude", "91", "--yes"}, `invalid location-latitude "91"`},
		{"nan latitude", []string{"computezone", "update", "cz-1", "--location-latitude", "NaN", "--yes"}, `invalid location-latitude "NaN"`},
		{"missing id", []string{"computezone", "update", "--yes"}, "compute zone ID is required"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs(testCase.args)

			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}

	if len(requests) != 0 {
		t.Fatalf("rejected updates still issued requests: %#v", requests)
	}
}

func TestComputeZoneUpdateEarlyValidation(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "invalid timeout", args: []string{"computezone", "update", "cz-1", "--type", "datacenter", "--yes", "--timeout", "0s"}, want: "--timeout must be greater than 0"},
		{name: "whitespace ID", args: []string{"computezone", "update", "   ", "--type", "datacenter", "--yes"}, want: "compute zone ID is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			var requests atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")

			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs(tt.args)
			err := cmd.Execute()
			if err == nil || err.Error() != tt.want {
				t.Errorf("error = %v, want %q", err, tt.want)
			}
			if out.Len() != 0 {
				t.Errorf("unexpected result output: %q", out.String())
			}
			if got := requests.Load(); got != 0 {
				t.Errorf("HTTP requests = %d, want 0", got)
			}
		})
	}
}
