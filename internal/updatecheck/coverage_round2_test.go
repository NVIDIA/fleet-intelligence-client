// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package updatecheck

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRound2UpgradeRunWindowsRestoreFailure(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("requires Windows executable move and restore behavior")
	}

	dir := t.TempDir()
	executable := filepath.Join(dir, "nvfleetint.exe")
	original := []byte("original executable bytes")
	if err := os.WriteFile(executable, original, 0o700); err != nil {
		t.Fatalf("write original executable: %v", err)
	}

	// PowerShell single-quoted strings escape an apostrophe by doubling it.
	quotedPath := strings.ReplaceAll(executable, "'", "''")
	script := "$ErrorActionPreference = 'Stop'\n" +
		"$target = '" + quotedPath + "'\n" +
		"[void][IO.Directory]::CreateDirectory($target)\n" +
		"[IO.File]::WriteAllText([IO.Path]::Combine($target, 'child'), 'replacement')\n" +
		"exit 1\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, script)
	}))
	defer server.Close()

	plan := UpgradePlan{
		Release:        Release{Version: "v1.2.3"},
		ExecutablePath: executable,
		InstallDir:     dir,
		InstallerURL:   server.URL,
	}
	var progress bytes.Buffer
	err := plan.Run(context.Background(), &progress)
	if err == nil {
		t.Fatal("Run succeeded, want installer and restore failures")
	}
	for _, want := range []string{
		"the installer failed",
		"additionally failed to restore the original executable",
		"could not remove the failed replacement executable",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Run error = %v, want %q", err, want)
		}
	}

	backup, err := os.ReadFile(executable + ".old")
	if err != nil {
		t.Fatalf("read original backup: %v", err)
	}
	if !bytes.Equal(backup, original) {
		t.Errorf("backup = %q, want %q", backup, original)
	}
	info, err := os.Stat(executable)
	if err != nil {
		t.Fatalf("stat replacement directory: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("replacement at %q is not a directory", executable)
	}
	child, err := os.ReadFile(filepath.Join(executable, "child"))
	if err != nil {
		t.Fatalf("read replacement child: %v", err)
	}
	if string(child) != "replacement" {
		t.Errorf("replacement child = %q, want %q", child, "replacement")
	}
}
