// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

// This file implements the stdin credential prompts `auth add` uses. The API
// key is read from stdin rather than taken as a flag so it never reaches the
// user's shell history, the process list, or a CI transcript.

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/NVIDIA/fleet-intelligence-client/internal/config"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"

	"golang.org/x/term"
)

// maxPromptAttempts bounds re-asking after input that was rejected, so a
// terminal handing back the same unusable value cannot spin forever.
const maxPromptAttempts = 3

// credentialPrompt reads credentials from stdin and writes its questions to
// out — the command's stderr, so prompts never mix into piped output.
type credentialPrompt struct {
	// in is kept alongside reader because hiding a typed key needs the
	// underlying file descriptor, not a buffered view of it.
	in     io.Reader
	reader *bufio.Reader
	out    io.Writer
	// terminal records that stdin is a real TTY, which is what separates the
	// two ways this command is run. At a terminal a person is answering:
	// echo is suppressed, rejected input is asked for again, and entering a
	// value is itself the answer to a warning. With input piped in there is
	// nobody to ask, so the first answer is the only one.
	terminal bool
}

func newCredentialPrompt(in io.Reader, out io.Writer) *credentialPrompt {
	return &credentialPrompt{
		in:       in,
		reader:   bufio.NewReader(in),
		out:      out,
		terminal: isTerminalReader(in),
	}
}

// isTerminalReader reports whether in is a real terminal. Anything else — a
// pipe, a redirected file, an in-process reader supplied by a test — is read
// as plain lines. This deliberately differs from clihelpers.isAnswerable,
// which only has to decide whether a question can be answered: here a
// non-terminal is still a usable source of values, just not of a conversation.
func isTerminalReader(in io.Reader) bool {
	file, ok := in.(*os.File)
	if !ok {
		return false
	}

	return term.IsTerminal(int(file.Fd()))
}

// promptAPIKey reads the API key. Empty input keeps the key already stored, so
// rotating an endpoint never has to re-enter — or even see — the current key.
// With none stored there is nothing to keep and the value is required.
func (p *credentialPrompt) promptAPIKey(keepStored bool) (string, bool, error) {
	label := "API key: "
	if keepStored {
		label = "API key [press Enter to keep the stored key]: "
	}

	for attempt := 1; ; attempt++ {
		value, err := p.read(label, true)
		if err != nil {
			return "", false, err
		}
		if value != "" {
			return value, true, nil
		}
		if keepStored {
			return "", false, nil
		}
		if !p.terminal || attempt >= maxPromptAttempts {
			return "", false, errors.New("API key is required")
		}
		fmt.Fprintln(p.out, "An API key is required.")
	}
}

// promptAPIURL reads the API URL, offering current as the value to accept —
// the production endpoint for a new profile, the stored one for an existing
// profile, so pressing Enter is always the answer that changes nothing.
//
// The offered value is named, not just printed: most people cannot tell the
// production endpoint from a typo by reading it, so an unlabeled URL makes
// pressing Enter a guess rather than a decision.
//
// The answer is validated here rather than on the way to disk: at a terminal a
// rejected URL can simply be asked for again, which is the whole advantage of
// prompting over a flag.
func (p *credentialPrompt) promptAPIURL(current string) (string, error) {
	origin := "stored"
	if current == config.DefaultAPIURL {
		origin = "production"
	}
	label := fmt.Sprintf("API URL [%s: %s]: ", origin, current)

	for attempt := 1; ; attempt++ {
		value, err := p.read(label, false)
		if err != nil {
			return "", err
		}
		if value == "" {
			value = current
		}
		if err := nvfleetint.ValidateBaseURL(value); err != nil {
			if !p.terminal || attempt >= maxPromptAttempts {
				return "", err
			}
			fmt.Fprintf(p.out, "%v\n", err)
			continue
		}

		return value, nil
	}
}

// note writes a line of context above a prompt.
func (p *credentialPrompt) note(format string, args ...any) {
	fmt.Fprintf(p.out, format+"\n", args...)
}

// read writes label and returns the trimmed answer. Input that ends before an
// answer arrives reads as empty rather than as an error: whether an empty
// value is allowed depends on the question, so the caller decides.
func (p *credentialPrompt) read(label string, hide bool) (string, error) {
	fmt.Fprint(p.out, label)

	if hide {
		if file, ok := p.in.(*os.File); ok && p.terminal {
			data, err := term.ReadPassword(int(file.Fd()))
			// ReadPassword consumes the newline without echoing it, so
			// without this the next prompt lands on the same line.
			fmt.Fprintln(p.out)
			if err != nil && !errors.Is(err, io.EOF) {
				return "", err
			}

			return strings.TrimSpace(string(data)), nil
		}
	}

	line, err := p.reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}

	return strings.TrimSpace(line), nil
}
