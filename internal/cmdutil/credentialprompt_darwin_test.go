// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package cmdutil

import (
	"bytes"
	"os"
	"strings"
	"syscall"
	"testing"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Exercise the real terminal password path on macOS, without reading the
// user's terminal or requiring a separate executable or a deployed service.
func TestDarwinTerminalPasswordInput(t *testing.T) {
	for _, tc := range []struct {
		name, input, wantKey, wantPrompt string
		keepStored, wantSet              bool
	}{
		{name: "trim new key", input: "  test-key  \n", wantKey: "test-key", wantSet: true, wantPrompt: "API key: \n"},
		{name: "keep stored key", input: "\n", keepStored: true, wantPrompt: "API key [press Enter to keep the stored key]: \n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			master, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = master.Close() })
			for _, request := range []uint{unix.TIOCPTYGRANT, unix.TIOCPTYUNLK} {
				if err := unix.IoctlSetInt(int(master.Fd()), request, 0); err != nil {
					t.Fatalf("prepare pseudo-terminal: %v", err)
				}
			}
			// Darwin's TIOCPTYGNAME requires a caller-owned 128-byte buffer.
			// x/sys/unix has no typed wrapper for this ioctl.
			var name [128]byte
			_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, master.Fd(), unix.TIOCPTYGNAME, uintptr(unsafe.Pointer(&name[0])))
			if errno != 0 {
				t.Fatalf("get pseudo-terminal name: %v", errno)
			}
			slave, err := os.OpenFile(strings.TrimRight(string(name[:]), "\x00"), os.O_RDWR, 0)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = slave.Close() })
			if n, err := master.WriteString(tc.input); err != nil || n != len(tc.input) {
				t.Fatalf("write terminal input: n=%d, err=%v", n, err)
			}
			var out bytes.Buffer
			prompt := NewCredentialPrompt(slave, &out)
			if !prompt.Interactive() {
				t.Fatal("pseudo-terminal was not recognized as interactive")
			}
			key, set, err := prompt.APIKey(tc.keepStored)
			if err != nil || key != tc.wantKey || set != tc.wantSet {
				t.Fatalf("APIKey = %q, %v, %v; want %q, %v, nil", key, set, err, tc.wantKey, tc.wantSet)
			}
			if out.String() != tc.wantPrompt {
				t.Errorf("prompt = %q, want %q", out.String(), tc.wantPrompt)
			}
		})
	}
}
