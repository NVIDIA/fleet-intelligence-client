// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package clihelpers

// This file implements the confirmation prompt shared by destructive commands.

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

// ErrAborted reports a confirmation prompt the user declined. It exits 1
// through main.go like any other command error.
var ErrAborted = errors.New("aborted")

// Confirm prints summary and an "Are you sure? [y/N]" prompt to out and waits
// for an answer on in. Callers pass the command's stderr as out so `-o json`
// keeps stdout parseable. Anything other than an explicit yes returns
// ErrAborted, so the default is No.
//
// It refuses to prompt when in is a non-terminal file, so a cron or CI run gets
// a clear "re-run with --yes" error instead of blocking forever or silently
// aborting on EOF.
func Confirm(in io.Reader, out io.Writer, summary string) error {
	if !isAnswerable(in) {
		return errors.New("cannot prompt for confirmation: stdin is not a terminal; re-run with --yes")
	}

	fmt.Fprintln(out, summary)
	fmt.Fprint(out, "Are you sure? [y/N]: ")

	answer, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}

	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return nil
	default:
		return ErrAborted
	}
}

// isAnswerable reports whether reading from in can plausibly reach a human.
// Anything that is not an *os.File is an in-process reader supplied by a test.
// For a real file this must be a true terminal check, not a character-device
// check: `< /dev/null` is a character device, and treating it as answerable is
// how a prompt turns into a silent EOF abort.
func isAnswerable(in io.Reader) bool {
	file, ok := in.(*os.File)
	if !ok {
		return true
	}

	return term.IsTerminal(int(file.Fd()))
}
