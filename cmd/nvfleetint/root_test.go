// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmd/release"
	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"

	"github.com/spf13/cobra"
)

// testBuild stands in for the values the linker stamps into a real build.
var testBuild = release.BuildInfo{Version: "dev", Commit: "unknown", BuildDate: "unknown"}

// newTestRootCmd assembles the full command tree the binary ships.
func newTestRootCmd() *cobra.Command {
	return newRootCmd(testBuild)
}

func TestHelpCommand(t *testing.T) {
	cmd := newTestRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("help command failed: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "Fleet Intelligence CLI") {
		t.Fatalf("help output missing description: %q", got)
	}
}

// Verifies common flags are command-local
func TestCommandsRejectUnsupportedCommonFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "root output", args: []string{"--output", "json"}, want: "unknown flag: --output"},
		{name: "auth pagination", args: []string{"auth", "add", "p", "--all"}, want: "unknown flag: --all"},
		{name: "version pagination", args: []string{"version", "--page", "1"}, want: "unknown flag: --page"},
		{name: "read pagination", args: []string{"node", "describe", "node-1", "--page-size", "10"}, want: "unknown flag: --page-size"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newTestRootCmd()
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetArgs(tt.args)

			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("unexpected error: got %v want %q", err, tt.want)
			}
		})
	}
}

// Verifies the --timeout flag rejects non-positive durations
func TestCommandsRejectNonPositiveTimeout(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "list zero", args: []string{"node", "list", "--timeout", "0"}},
		{name: "list negative", args: []string{"node", "list", "--timeout", "-5s"}},
		{name: "read zero", args: []string{"node", "describe", "node-1", "--timeout", "0"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newTestRootCmd()
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetArgs(tt.args)

			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), "--timeout must be greater than 0") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// Verifies every command that talks to the API can pick its credentials.
// The shared helpers (registerListCommonFlags / registerReadCommonFlags) cover
// most commands, but any command that registers its flags by hand — as
// `report verify` does — has to opt in explicitly, and this is what catches it.
func TestClientCommandsAcceptProfileFlag(t *testing.T) {
	// Commands that run entirely locally and so need no credentials.
	exempt := map[string]bool{
		"nvfleetint version":     true,
		"nvfleetint upgrade":     true,
		"nvfleetint auth list":   true,
		"nvfleetint auth add":    true,
		"nvfleetint auth remove": true,
		"nvfleetint auth use":    true,
	}

	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		if len(cmd.Commands()) > 0 {
			for _, child := range cmd.Commands() {
				walk(child)
			}
			return
		}
		path := cmd.CommandPath()
		if exempt[path] {
			return
		}
		if cmd.Flags().Lookup("profile") == nil {
			t.Errorf("%s does not accept --profile", path)
		}
	}
	walk(newTestRootCmd())
}

// The mirror image of the above: on the auth CRUD commands the profile is the
// object of the command and is named positionally, so --profile must not exist
// there. Registering it would give one flag two meanings — "which profile do I
// change" and "whose credentials do I use" — which is the confusion the
// positional argument removes.
func TestAuthProfileCommandsRejectProfileFlag(t *testing.T) {
	for _, name := range []string{"add", "remove", "use"} {
		t.Run(name, func(t *testing.T) {
			cmd, _, err := newTestRootCmd().Find([]string{"auth", name})
			if err != nil {
				t.Fatalf("find auth %s failed: %v", name, err)
			}
			if cmd.Flags().Lookup("profile") != nil {
				t.Errorf("auth %s must take the profile name positionally, not as --profile", name)
			}
			if !strings.Contains(cmd.Use, "<name>") {
				t.Errorf("auth %s should document its positional name, got Use %q", name, cmd.Use)
			}
		})
	}
}

// Verifies the top-level execute helper. testBuild is a dev build, which skips
// the release lookup, so this reaches the network no more than `--help` does.
func TestExecuteRunsRootCommand(t *testing.T) {
	if err := execute(context.Background(), testBuild, []string{"version"}); err != nil {
		t.Fatalf("execute failed: %v", err)
	}
}

// Cobra's default is to accept and silently ignore positional arguments, so a
// command that declares no Args validator runs normally when given one:
// `nvfleetint node list abcd` would print the whole node list, and a group like
// `nvfleetint node bogus` would print help and exit 0. Every command must
// therefore state what it takes. Root is exempt because cobra applies the same
// check to it for free (and there also suggests near-misses).
func TestEveryCommandDeclaresArgsValidator(t *testing.T) {
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		if cmd.Args == nil {
			t.Errorf("%s declares no Args validator, so it silently accepts stray arguments", cmd.CommandPath())
		}
		for _, child := range cmd.Commands() {
			walk(child)
		}
	}

	root := newTestRootCmd()
	for _, child := range root.Commands() {
		walk(child)
	}
}

// The behavior the validators above buy, end to end. Each case must fail
// *because of* the argument: a bare "expected an error" would also pass on the
// "no profile configured" error these commands raise once they reach the API.
func TestCommandsRejectUnexpectedArguments(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"list command", []string{"node", "list", "abcd"}, `unknown command "abcd" for "nvfleetint node list"`},
		{"read command", []string{"overview", "abcd"}, `unknown command "abcd" for "nvfleetint overview"`},
		{"local command", []string{"version", "abcd"}, `unknown command "abcd" for "nvfleetint version"`},
		{"report subcommand", []string{"report", "inventory", "abcd"}, `unknown command "abcd" for "nvfleetint report inventory"`},
		{"group typo", []string{"node", "bogus"}, `unknown command "bogus" for "nvfleetint node"`},
		{"auth group typo", []string{"auth", "update"}, `unknown command "update" for "nvfleetint auth"`},
		{"unknown top-level command", []string{"bogus"}, `unknown command "bogus" for "nvfleetint"`},
		{"too many args", []string{"node", "describe", "uuid-a", "uuid-b"}, "only one node UUID may be given, got 2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			// Credentials have to be configured for the command to get as far as
			// a request, so that "no request was made" means the argument was
			// rejected rather than that there was nothing to authenticate with.
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Errorf("%v reached the API: %s %s", tt.args, r.Method, r.URL.Path)
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")

			cmd := newTestRootCmd()
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs(tt.args)

			err := cmd.Execute()
			if err == nil {
				t.Fatalf("%v was accepted; output: %q", tt.args, out.String())
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("%v: got error %q, want it to contain %q", tt.args, err, tt.wantErr)
			}
		})
	}
}

// Reads the positional arity a command advertises in its Use line: `<uuid>` is
// required, `[<name>]` is optional. Keeping this in sync with the Args
// validator is the point of the test below — a command whose help promises one
// argument but accepts three is as much a bug as one that accepts a stray word.
func arityFromUse(use string) (minArgs, maxArgs int) {
	for _, token := range strings.Fields(use)[1:] {
		switch {
		case strings.HasPrefix(token, "[<"):
			maxArgs++
		case strings.HasPrefix(token, "<"):
			minArgs++
			maxArgs++
		}
	}
	return minArgs, maxArgs
}

// Sweeps every command in the tree at every arity it does not accept — too few
// as well as too many — rather than trusting a hand-picked sample. A command
// that takes one UUID has to reject zero and two, not just the stray-word case.
func TestEveryCommandRejectsWrongArgCount(t *testing.T) {
	var commands []*cobra.Command
	var collect func(cmd *cobra.Command)
	collect = func(cmd *cobra.Command) {
		commands = append(commands, cmd)
		for _, child := range cmd.Commands() {
			collect(child)
		}
	}
	for _, child := range newTestRootCmd().Commands() {
		collect(child)
	}

	for _, cmd := range commands {
		path := strings.Fields(cmd.CommandPath())[1:]
		minArgs, maxArgs := arityFromUse(cmd.Use)

		for count := 0; count <= maxArgs+1; count++ {
			if count >= minArgs && count <= maxArgs {
				continue // an accepted arity; running it would call the API
			}
			t.Run(fmt.Sprintf("%s/%d args", cmd.CommandPath(), count), func(t *testing.T) {
				t.Setenv("HOME", t.TempDir())
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					t.Errorf("reached the API: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusInternalServerError)
				}))
				defer server.Close()
				cmdtest.SaveConfig(t, server.URL, "test-key")

				args := append([]string{}, path...)
				for i := 0; i < count; i++ {
					args = append(args, fmt.Sprintf("extra%d", i))
				}

				root := newTestRootCmd()
				var out bytes.Buffer
				root.SetOut(&out)
				root.SetErr(&out)
				root.SetIn(strings.NewReader(""))
				root.SetArgs(args)

				if err := root.Execute(); err == nil {
					t.Errorf("%v took %d positional args, want %d-%d; output: %q",
						args, count, minArgs, maxArgs, out.String())
				}
			})
		}
	}
}
