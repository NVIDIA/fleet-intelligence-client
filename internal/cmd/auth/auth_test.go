// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdutil"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
	"github.com/spf13/cobra"
)

// testCommonFlags is the resolved common flags a client is built from when the
// test is checking credential resolution rather than a command's own flags.
func testCommonFlags(profile string) cmdutil.Resolved {
	return cmdutil.Resolved{
		Timeout:    nvfleetint.DefaultTimeout,
		Profile:    profile,
		ProfileSet: profile != "",
	}
}

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

const (
	apiKey = "test-key"
	apiURL = "https://fleet.example.com"
)

// newAuthStatusServer starts a test server that mimics GET /v1/auth/status,
// asserting the bearer token and replying with the given status and body.
func newAuthStatusServer(t *testing.T, statusCode int, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/auth/status" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+apiKey {
			t.Errorf("unexpected auth header: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		_, _ = w.Write([]byte(body))
	}))
}

// mustRunAuth executes an auth subcommand that is expected to succeed.
func mustRunAuth(t *testing.T, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(append([]string{"auth"}, args...))

	if err := cmd.Execute(); err != nil {
		t.Fatalf("auth %s failed: %v", strings.Join(args, " "), err)
	}

	return out.String()
}

// runAuthErr executes an auth subcommand that is expected to fail.
func runAuthErr(t *testing.T, args ...string) error {
	t.Helper()
	return runAuthInputErr(t, "", args...)
}

// runAuthInputErr executes an auth subcommand with input on stdin, expecting
// it to fail.
func runAuthInputErr(t *testing.T, input string, args ...string) error {
	t.Helper()
	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetIn(strings.NewReader(input))
	cmd.SetArgs(append([]string{"auth"}, args...))

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected auth %s to fail", strings.Join(args, " "))
	}

	return err
}

// addAnswers is what `auth add` reads from a piped stdin: the API key, then
// the API URL. An empty answer keeps the stored (or default) value.
func addAnswers(apiKey, apiURL string) string {
	return apiKey + "\n" + apiURL + "\n"
}

// mustAddProfile stores a profile through `auth add`, answering its prompts on
// stdin. The reader is not a terminal, so this exercises the piped form —
// which is also why replacing a stored key here needs an explicit "--yes".
func mustAddProfile(t *testing.T, name, apiKey, apiURL string, extra ...string) string {
	t.Helper()

	args := []string{"auth", "add"}
	if name != "" {
		args = append(args, name)
	}
	args = append(args, extra...)

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader(addAnswers(apiKey, apiURL)))
	cmd.SetArgs(args)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("auth add %s failed: %v", strings.Join(args[2:], " "), err)
	}

	return out.String()
}

type authListErrorWriter struct{ err error }

func (w authListErrorWriter) Write([]byte) (int, error) { return 0, w.err }

// authFirstRead invokes the change after the command's initial config read,
// but before it receives any prompt answers.
type authFirstRead struct {
	reader *strings.Reader
	once   func()
}

func (r *authFirstRead) Read(p []byte) (int, error) {
	if r.once != nil {
		callback := r.once
		r.once = nil
		callback()
	}
	return r.reader.Read(p)
}
