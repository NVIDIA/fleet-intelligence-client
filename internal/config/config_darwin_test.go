// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// A legal target path can leave too little room for the atomic temporary
// filename. Failure must preserve the original credentials and leave no debris.
func TestDarwinAtomicConfigPathLimitPreservesOriginal(t *testing.T) {
	const directoryLength = 1012 // Darwin PATH_MAX is 1024, including the NUL.
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(dir) > directoryLength-2 {
		t.Fatalf("temporary root is too long: %s", dir)
	}
	for len(dir) < directoryLength {
		n := min(200, directoryLength-len(dir)-1)
		if directoryLength-len(dir)-n-1 == 1 {
			n--
		}
		dir = filepath.Join(dir, strings.Repeat("d", n))
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	target := filepath.Join(dir, "c")
	original := []byte("original credential bytes\n")
	if err := os.WriteFile(target, original, 0600); err != nil {
		t.Fatal(err)
	}
	err = writeFileAtomic(target, []byte("replacement credentials\n"))
	var pathErr *os.PathError
	if !errors.As(err, &pathErr) || !errors.Is(err, syscall.ENAMETOOLONG) {
		t.Fatalf("write error = %v, want filename-too-long path error", err)
	}
	if pathErr.Op != "open" || !strings.HasPrefix(pathErr.Path, filepath.Join(dir, ".config-")) {
		t.Errorf("failure = %#v, want atomic temporary file creation", pathErr)
	}
	got, err := os.ReadFile(target)
	if err != nil || !bytes.Equal(got, original) {
		t.Fatalf("original file = %q, %v; want %q, nil", got, err, original)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || entries[0].Name() != "c" {
		t.Errorf("directory = %v, %v; want only original file", entries, err)
	}
}
