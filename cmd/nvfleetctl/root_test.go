package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
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
	if !strings.Contains(got, "nvfleetctl ") {
		t.Fatalf("version output missing binary name: %q", got)
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
		{name: "auth output", args: []string{"auth", "status", "--output", "json"}, want: "unknown flag: --output"},
		{name: "auth pagination", args: []string{"auth", "login", "--key", "test-key", "--all"}, want: "unknown flag: --all"},
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

// Verifies the top-level execute helper
func TestExecuteRunsRootCommand(t *testing.T) {
	if err := execute(context.Background(), []string{"version"}); err != nil {
		t.Fatalf("execute failed: %v", err)
	}
}
