// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package cmdutil

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Only an explicit yes proceeds; everything else, including an empty line and a
// bare EOF, takes the No default.
func TestConfirmAnswers(t *testing.T) {
	tests := []struct {
		name    string
		answer  string
		accepts bool
	}{
		{name: "y", answer: "y\n", accepts: true},
		{name: "yes", answer: "yes\n", accepts: true},
		{name: "mixed case", answer: "YeS\n", accepts: true},
		{name: "padded", answer: "  y  \n", accepts: true},
		{name: "no trailing newline", answer: "y", accepts: true},
		{name: "n", answer: "n\n", accepts: false},
		{name: "empty line", answer: "\n", accepts: false},
		{name: "eof", answer: "", accepts: false},
		{name: "unrelated word", answer: "yep\n", accepts: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Confirm(strings.NewReader(tt.answer), &bytes.Buffer{}, "summary")

			if tt.accepts {
				if err != nil {
					t.Fatalf("expected the answer to be accepted, got %v", err)
				}
				return
			}
			if !errors.Is(err, ErrAborted) {
				t.Fatalf("expected ErrAborted, got %v", err)
			}
		})
	}
}

// The summary and the prompt both go to out — callers pass stderr so that
// `-o json` keeps stdout parseable.
func TestConfirmWritesSummaryAndPromptToOut(t *testing.T) {
	var out bytes.Buffer
	if err := Confirm(strings.NewReader("y\n"), &out, "This deletes everything."); err != nil {
		t.Fatalf("confirm failed: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "This deletes everything.") {
		t.Fatalf("expected the summary in the Output: %q", got)
	}
	if !strings.Contains(got, "Are you sure? [y/N]: ") {
		t.Fatalf("expected the prompt in the Output: %q", got)
	}
}

// A cron or CI run gets a clear error instead of blocking or silently aborting.
// /dev/null is the case a character-device check gets wrong: it looks like a
// terminal by mode, so a redirected run would read EOF and abort with a
// confusing "aborted" instead of naming --yes.
func TestConfirmRefusesToPromptWithoutATerminal(t *testing.T) {
	tests := []struct {
		name string
		open func(t *testing.T) *os.File
	}{
		{
			name: "closed pipe",
			open: func(t *testing.T) *os.File {
				t.Helper()
				reader, writer, err := os.Pipe()
				if err != nil {
					t.Fatalf("pipe failed: %v", err)
				}
				t.Cleanup(func() { _ = reader.Close() })
				_ = writer.Close()
				return reader
			},
		},
		{
			name: "dev null",
			open: func(t *testing.T) *os.File {
				t.Helper()
				file, err := os.Open(os.DevNull)
				if err != nil {
					t.Fatalf("open %s failed: %v", os.DevNull, err)
				}
				t.Cleanup(func() { _ = file.Close() })
				return file
			},
		},
		{
			name: "regular file holding yes",
			open: func(t *testing.T) *os.File {
				t.Helper()
				path := filepath.Join(t.TempDir(), "stdin")
				if err := os.WriteFile(path, []byte("y\n"), 0o600); err != nil {
					t.Fatalf("write failed: %v", err)
				}
				file, err := os.Open(path)
				if err != nil {
					t.Fatalf("open failed: %v", err)
				}
				t.Cleanup(func() { _ = file.Close() })
				return file
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			err := Confirm(tt.open(t), &out, "summary")

			if err == nil {
				t.Fatal("expected a refusal to prompt")
			}
			// Not an abort: the caller has to be told how to proceed.
			if errors.Is(err, ErrAborted) {
				t.Fatalf("a non-terminal stdin must not read as a declined prompt: %v", err)
			}
			if !strings.Contains(err.Error(), "--yes") {
				t.Fatalf("expected the error to name --yes: %v", err)
			}
			if out.Len() != 0 {
				t.Fatalf("nothing should be printed when the prompt is refused: %q", out.String())
			}
		})
	}
}
