// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package updatecheck

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
)

// The work directory can fit within PATH_MAX even though its installer file
// does not. Verify the error is surfaced before any installer is executed.
func TestDarwinUpgradeInstallerPathTooLong(t *testing.T) {
	const directoryLength = 993
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
	// MkdirTemp adds "nvfleetint-upgrade-" plus 1..10 decimal digits.
	// All those directory names fit, but appending /install.sh exceeds 1023.
	t.Setenv("TMPDIR", dir)
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		_, _ = w.Write([]byte("#!/bin/sh\nprintf 'installer must not run'\n"))
	}))
	t.Cleanup(server.Close)
	plan := UpgradePlan{InstallerURL: server.URL, Release: Release{Version: "v1.2.0"}}
	var progress bytes.Buffer
	err = plan.Run(context.Background(), &progress)
	if err == nil || !strings.Contains(err.Error(), "could not write the installer") || !errors.Is(err, syscall.ENAMETOOLONG) {
		t.Fatalf("Run error = %v, want installer write failure caused by PATH_MAX", err)
	}
	if progress.Len() != 0 {
		t.Errorf("installer produced output after failed write: %q", progress.String())
	}
	if requests.Load() != 1 {
		t.Errorf("downloads = %d, want exactly one", requests.Load())
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Errorf("temporary directory = %v, %v; want no leftover work directories", entries, err)
	}
}
