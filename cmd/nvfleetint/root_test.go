// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"

	"github.com/spf13/cobra"
)

func TestVersionCommand(t *testing.T) {
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("version command failed: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "nvfleetint ") {
		t.Fatalf("version output missing binary name: %q", got)
	}
}

func TestVersionCommandJSON(t *testing.T) {
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"version", "--output", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("version command failed: %v", err)
	}

	var got versionOutput
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode version JSON failed: %v", err)
	}
	if got.Name != "nvfleetint" || got.Version == "" {
		t.Fatalf("unexpected version JSON: %#v", got)
	}
}

func TestHelpCommand(t *testing.T) {
	cmd := newRootCmd()
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
			cmd := newRootCmd()
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
			cmd := newRootCmd()
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
	walk(newRootCmd())
}

// The mirror image of the above: on the auth CRUD commands the profile is the
// object of the command and is named positionally, so --profile must not exist
// there. Registering it would give one flag two meanings — "which profile do I
// change" and "whose credentials do I use" — which is the confusion the
// positional argument removes.
func TestAuthProfileCommandsRejectProfileFlag(t *testing.T) {
	for _, name := range []string{"add", "remove", "use"} {
		t.Run(name, func(t *testing.T) {
			cmd, _, err := newRootCmd().Find([]string{"auth", name})
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

// Verifies the top-level execute helper
func TestExecuteRunsRootCommand(t *testing.T) {
	if err := execute(context.Background(), []string{"version"}); err != nil {
		t.Fatalf("execute failed: %v", err)
	}
}

func TestWriteCLIErrorJSON(t *testing.T) {
	var out bytes.Buffer
	writeCLIError(&out, []string{"node", "list", "--output", "json"}, errors.New("bad input"))

	var got errorOutput
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode error JSON failed: %v", err)
	}
	if got.Error.Code != "command_error" || got.Error.Message != "bad input" {
		t.Fatalf("unexpected error JSON: %#v", got)
	}
}

func TestWriteCLIErrorJSONForParseErrorArgs(t *testing.T) {
	var out bytes.Buffer
	writeCLIError(&out, []string{"version", "--output", "json", "--badflag"}, errors.New("unknown flag: --badflag"))

	var got errorOutput
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode error JSON failed: %v", err)
	}
	if got.Error.Code != "command_error" || got.Error.Message != "unknown flag: --badflag" {
		t.Fatalf("unexpected error JSON: %#v", got)
	}
}

func TestWriteCLIErrorIncludesAPIDetails(t *testing.T) {
	var out bytes.Buffer
	err := &nvfleetint.APIError{
		StatusCode: 403,
		Status:     "Forbidden",
		Message:    "permission denied",
		Details:    "missing role",
	}
	writeCLIError(&out, []string{"node", "list", "--output", "json"}, err)

	var got errorOutput
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode error JSON failed: %v", err)
	}
	if got.Error.Code != "api_error" || got.Error.StatusCode != 403 || got.Error.Status != "Forbidden" || got.Error.Message != "permission denied" || got.Error.Details != "missing role" {
		t.Fatalf("unexpected error JSON: %#v", got)
	}
}

func TestExitCodeForPermissionErrors(t *testing.T) {
	if got := exitCodeFor(&nvfleetint.APIError{StatusCode: 403}); got != exitNoPermission {
		t.Fatalf("unexpected permission exit code: %d", got)
	}
	if got := exitCodeFor(&nvfleetint.APIError{StatusCode: 500}); got != exitError {
		t.Fatalf("unexpected general exit code: %d", got)
	}
}

func TestRequireSingleArg(t *testing.T) {
	validate := requireSingleArg("node UUID")

	if err := validate(nil, []string{"only-one"}); err != nil {
		t.Fatalf("expected exactly one arg to pass, got: %v", err)
	}

	err := validate(nil, nil)
	if err == nil || !strings.Contains(err.Error(), "node UUID is required") {
		t.Fatalf("expected missing-arg error, got: %v", err)
	}

	err = validate(nil, []string{"a", "b"})
	if err == nil || !strings.Contains(err.Error(), "only one node UUID may be given, got 2") {
		t.Fatalf("expected too-many-args error, got: %v", err)
	}
}
