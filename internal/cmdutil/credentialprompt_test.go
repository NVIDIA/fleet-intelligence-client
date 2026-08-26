// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package cmdutil

import (
	"bufio"
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/config"
)

// The credential values the prompt tests answer with.
const (
	apiKey = "test-key"
	apiURL = "https://fleet.example.com"
)

// newTestPrompt builds a prompt over a canned answer script. terminal is set
// directly rather than detected: the behavior that separates a person from a
// pipe — re-asking after bad input — has to be testable without a real TTY.
// Echo suppression is the one part this cannot reach, since it needs a file
// descriptor; read() falls back to a plain line read for anything else.
func newTestPrompt(answers string, terminal bool) (*CredentialPrompt, *bytes.Buffer) {
	in := strings.NewReader(answers)
	out := &bytes.Buffer{}

	return &CredentialPrompt{
		in:       in,
		reader:   bufio.NewReader(in),
		out:      out,
		terminal: terminal,
	}, out
}

// A person who presses Enter by mistake gets asked again rather than losing
// the command.
func TestPromptAPIKeyAsksAgainAtATerminal(t *testing.T) {
	prompt, out := newTestPrompt("\n"+apiKey+"\n", true)

	key, set, err := prompt.APIKey(false)
	if err != nil {
		t.Fatalf("promptAPIKey failed: %v", err)
	}
	if !set || key != apiKey {
		t.Fatalf("unexpected key: %q (set=%v)", key, set)
	}
	if !strings.Contains(out.String(), "An API key is required.") {
		t.Fatalf("expected the empty answer to be explained: %q", out.String())
	}
}

// Re-asking is bounded, so a terminal that keeps handing back the same empty
// answer ends the command instead of looping.
func TestPromptAPIKeyGivesUpAtATerminal(t *testing.T) {
	prompt, out := newTestPrompt(strings.Repeat("\n", maxPromptAttempts+3), true)

	if _, _, err := prompt.APIKey(false); err == nil ||
		!strings.Contains(err.Error(), "API key is required") {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.Count(out.String(), "API key: "); got != maxPromptAttempts {
		t.Fatalf("asked %d times, want %d: %q", got, maxPromptAttempts, out.String())
	}
}

// With input piped in there is nobody to ask again: the first answer is the
// only one, and the rest of the script belongs to the next question.
func TestPromptAPIKeyDoesNotAskAgainWhenPiped(t *testing.T) {
	prompt, out := newTestPrompt("\n"+apiKey+"\n", false)

	if _, _, err := prompt.APIKey(false); err == nil ||
		!strings.Contains(err.Error(), "API key is required") {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.Count(out.String(), "API key: "); got != 1 {
		t.Fatalf("asked %d times, want 1: %q", got, out.String())
	}
}

// Keeping a stored key is an offer, so an empty answer accepts it rather than
// being rejected as missing.
func TestPromptAPIKeyKeepsStoredKey(t *testing.T) {
	prompt, out := newTestPrompt("\n", true)

	key, set, err := prompt.APIKey(true)
	if err != nil {
		t.Fatalf("promptAPIKey failed: %v", err)
	}
	if set || key != "" {
		t.Fatalf("expected the stored key to be kept, got %q (set=%v)", key, set)
	}
	if !strings.Contains(out.String(), "keep the stored key") {
		t.Fatalf("expected the offer to be stated: %q", out.String())
	}
}

// The offered URL is what pressing Enter answers, and it is named in the
// prompt so the user knows what they are accepting.
func TestPromptAPIURLAcceptsTheOfferedValue(t *testing.T) {
	prompt, out := newTestPrompt("\n", true)

	url, err := prompt.APIURL(config.DefaultAPIURL)
	if err != nil {
		t.Fatalf("promptAPIURL failed: %v", err)
	}
	if url != config.DefaultAPIURL {
		t.Fatalf("unexpected API URL: %q", url)
	}
	if !strings.Contains(out.String(), "API URL [production: "+config.DefaultAPIURL+"]: ") {
		t.Fatalf("expected the offered value to be named and shown: %q", out.String())
	}
}

// An existing profile is offered its own endpoint, which has to be labelled as
// such: "production" against a URL that is not the production one would be a
// lie, and the difference is exactly what a person cannot see by reading it.
func TestPromptAPIURLNamesAStoredValue(t *testing.T) {
	prompt, out := newTestPrompt("\n", true)

	url, err := prompt.APIURL(apiURL)
	if err != nil {
		t.Fatalf("promptAPIURL failed: %v", err)
	}
	if url != apiURL {
		t.Fatalf("unexpected API URL: %q", url)
	}
	if !strings.Contains(out.String(), "API URL [stored: "+apiURL+"]: ") {
		t.Fatalf("expected the stored value to be named: %q", out.String())
	}
	if strings.Contains(out.String(), "production") {
		t.Fatalf("a stored endpoint must not be called production: %q", out.String())
	}
}

// A typo'd endpoint is worth another try — that is the advantage of asking
// over taking a flag.
func TestPromptAPIURLAsksAgainAtATerminal(t *testing.T) {
	prompt, out := newTestPrompt("example.com\n"+apiURL+"\n", true)

	url, err := prompt.APIURL(config.DefaultAPIURL)
	if err != nil {
		t.Fatalf("promptAPIURL failed: %v", err)
	}
	if url != apiURL {
		t.Fatalf("unexpected API URL: %q", url)
	}
	if !strings.Contains(out.String(), "absolute https URL is required") {
		t.Fatalf("expected the rejection to be explained: %q", out.String())
	}
}

func TestPromptAPIURLRejectsInvalidValueWhenPiped(t *testing.T) {
	prompt, _ := newTestPrompt("example.com\n"+apiURL+"\n", false)

	if _, err := prompt.APIURL(config.DefaultAPIURL); err == nil ||
		!strings.Contains(err.Error(), "absolute https URL is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Only a real terminal is a conversation. /dev/null is the case a
// character-device check gets wrong — it looks like a terminal by mode.
func TestIsTerminalReader(t *testing.T) {
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("open %s failed: %v", os.DevNull, err)
	}
	t.Cleanup(func() { _ = devNull.Close() })

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe failed: %v", err)
	}
	t.Cleanup(func() {
		_ = reader.Close()
		_ = writer.Close()
	})

	tests := []struct {
		name string
		in   interface{ Read([]byte) (int, error) }
	}{
		{name: "in-process reader", in: strings.NewReader("")},
		{name: "dev null", in: devNull},
		{name: "pipe", in: reader},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if isTerminalReader(tt.in) {
				t.Fatal("expected a non-terminal")
			}
		})
	}
}
