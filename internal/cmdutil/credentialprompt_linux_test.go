//go:build linux

// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package cmdutil

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"golang.org/x/sys/unix"
)

func TestTerminalPasswordAcceptsNewOrStoredKey(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		keepStored bool
		wantKey    string
		wantSet    bool
		wantOutput string
	}{
		{name: "new key", input: "  test-key  \n", wantKey: "test-key", wantSet: true, wantOutput: "API key: \n"},
		{name: "keep stored key", input: "\n", keepStored: true, wantOutput: "API key [press Enter to keep the stored key]: \n"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			master, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
			if err != nil {
				t.Fatalf("open pseudo-terminal master: %v", err)
			}
			t.Cleanup(func() { _ = master.Close() })

			if err := unix.IoctlSetPointerInt(int(master.Fd()), unix.TIOCSPTLCK, 0); err != nil {
				t.Fatalf("unlock pseudo-terminal: %v", err)
			}
			number, err := unix.IoctlGetInt(int(master.Fd()), unix.TIOCGPTN)
			if err != nil {
				t.Fatalf("get pseudo-terminal number: %v", err)
			}
			slave, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", number), os.O_RDWR, 0)
			if err != nil {
				t.Fatalf("open pseudo-terminal slave: %v", err)
			}
			t.Cleanup(func() { _ = slave.Close() })

			if n, err := master.WriteString(tc.input); err != nil || n != len(tc.input) {
				t.Fatalf("write pseudo-terminal input: wrote %d of %d bytes, error: %v", n, len(tc.input), err)
			}
			var out bytes.Buffer
			prompt := NewCredentialPrompt(slave, &out)
			if !prompt.Interactive() {
				t.Fatal("expected pseudo-terminal to be interactive")
			}
			key, set, err := prompt.APIKey(tc.keepStored)
			if err != nil || key != tc.wantKey || set != tc.wantSet {
				t.Fatalf("APIKey(%v) = %q, %v, %v; want %q, %v, nil", tc.keepStored, key, set, err, tc.wantKey, tc.wantSet)
			}
			if got := out.String(); got != tc.wantOutput {
				t.Fatalf("prompt output = %q, want %q", got, tc.wantOutput)
			}
		})
	}
}
