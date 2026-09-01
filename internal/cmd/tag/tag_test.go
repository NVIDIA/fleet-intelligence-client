// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package tag

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdutil"
)

// newRootCmd builds a root command carrying only this package's commands, so
// the tests drive them through the same argument path a user types.
func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "nvfleetint",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(NewCmd())
	return root
}

// Verifies tag list table output and prefix filter translation
func TestTagListTable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

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

// Verifies more than one resource filter is rejected before any request
func TestTagListRejectsMultipleResourceFilters(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("did not expect a request for invalid flags")
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"tag", "list", "--node", "node-1", "--nodegroup", "ng-1"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for multiple resource filters")
	}
}

// Verifies tag set writes the requested tags and renders the result
func TestTagSetTable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/v1/nodes/node-1/tags" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body failed: %v", err)
		}
		if got := strings.TrimSpace(string(body)); got != `{"tags":["gpu-health","burn_in"]}` {
			t.Fatalf("unexpected body: %s", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodeUUID":"node-1","tags":["burn_in","gpu-health"]}`))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"tag", "set", "node-1", "--tags", "gpu-health, burn_in", "--yes"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{"NODE UUID", "TAGS", "node-1", "burn_in, gpu-health"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

// Verifies tag set JSON output emits the raw backend payload
func TestTagSetJSON(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	body := `{"nodeUUID":"node-1","tags":["gpu-health"]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"tag", "set", "node-1", "--tags", "gpu-health", "--yes", "--output", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}
	if strings.TrimSpace(out.String()) != body {
		t.Fatalf("unexpected JSON:\n%s", out.String())
	}
}

// Verifies --clear sends an empty tag list
func TestTagSetClear(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body failed: %v", err)
		}
		if got := strings.TrimSpace(string(body)); got != `{"tags":[]}` {
			t.Fatalf("unexpected body: %s", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodeUUID":"node-1","tags":[]}`))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"tag", "set", "node-1", "--clear", "--yes"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}
	if !strings.Contains(out.String(), "node-1") {
		t.Fatalf("output missing the node:\n%s", out.String())
	}
}

// Verifies the confirmation prompt names the pending replacement and that
// answering yes writes.
func TestTagSetConfirms(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodeUUID":"node-1","tags":["gpu-health"]}`))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out, errOut bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetIn(strings.NewReader("y\n"))
	cmd.SetArgs([]string{"tag", "set", "node-1", "--tags", "gpu-health"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}
	if requests != 1 {
		t.Fatalf("expected exactly one request, got %d", requests)
	}
	for _, want := range []string{"replaces every tag on node node-1", "gpu-health", "Are you sure?"} {
		if !strings.Contains(errOut.String(), want) {
			t.Fatalf("prompt missing %q:\n%s", want, errOut.String())
		}
	}
	// The prompt goes to stderr so `-o json` stdout stays parseable.
	if strings.Contains(out.String(), "Are you sure?") {
		t.Fatalf("prompt leaked into stdout:\n%s", out.String())
	}
}

// Verifies declining the confirmation writes nothing
func TestTagSetAbortsWhenDeclined(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("did not expect a request after declining")
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetIn(strings.NewReader("n\n"))
	cmd.SetArgs([]string{"tag", "set", "node-1", "--clear"})

	err := cmd.Execute()
	if !errors.Is(err, cmdutil.ErrAborted) {
		t.Fatalf("expected an aborted error, got %v", err)
	}
}

// Verifies --dry-run reports the resulting tags without sending a request or
// prompting for confirmation
func TestTagSetDryRun(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("did not expect a request during --dry-run")
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"tag", "set", "node-1", "--tags", "gpu-health", "--dry-run"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}
	for _, want := range []string{
		"PUT " + server.URL + "/v1/nodes/node-1/tags",
		`"tags"`,
		`"gpu-health"`,
		"Dry run: no request was sent.",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, out.String())
		}
	}
}

// Verifies --dry-run with -o json emits the method, URL, and body as a
// structured document instead of the backend payload, since no request is made
func TestTagSetDryRunJSON(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("did not expect a request during --dry-run")
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"tag", "set", "node-1", "--clear", "--dry-run", "-o", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	var decoded struct {
		Method string `json:"method"`
		URL    string `json:"url"`
		Body   struct {
			Tags []string `json:"tags"`
		} `json:"body"`
	}
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("json output not decodable: %v (%s)", err, out.String())
	}
	if decoded.Method != http.MethodPut {
		t.Fatalf("unexpected method: %s", decoded.Method)
	}
	if decoded.URL != server.URL+"/v1/nodes/node-1/tags" {
		t.Fatalf("unexpected url: %s", decoded.URL)
	}
	if len(decoded.Body.Tags) != 0 {
		t.Fatalf("unexpected body: %#v", decoded.Body)
	}
}

// Verifies flag and tag validation happens before any request
func TestTagSetRejectsInvalidFlags(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "missing node UUID",
			args: []string{"tag", "set", "--tags", "gpu-health", "--yes"},
			want: "node UUID is required",
		},
		{
			name: "no tag source",
			args: []string{"tag", "set", "node-1", "--yes"},
			want: "exactly one of --tags or --clear is required",
		},
		{
			name: "both tag sources",
			args: []string{"tag", "set", "node-1", "--tags", "gpu-health", "--clear", "--yes"},
			want: "--tags cannot be used with --clear",
		},
		{
			name: "empty tags",
			args: []string{"tag", "set", "node-1", "--tags", "", "--yes"},
			want: "use --clear",
		},
		{
			name: "empty tag in list",
			args: []string{"tag", "set", "node-1", "--tags", "gpu-health,,burn_in", "--yes"},
			want: "invalid --tags",
		},
		{
			name: "invalid tag format",
			args: []string{"tag", "set", "node-1", "--tags", "GPU-Health", "--yes"},
			want: "lowercase letters",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())

			server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
				t.Fatal("did not expect a request for invalid flags")
			}))
			defer server.Close()

			cmdtest.SaveConfig(t, server.URL, "test-key")

			cmd := newRootCmd()
			cmd.SetOut(new(bytes.Buffer))
			cmd.SetErr(new(bytes.Buffer))
			cmd.SetArgs(testCase.args)

			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected an error for invalid flags")
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
