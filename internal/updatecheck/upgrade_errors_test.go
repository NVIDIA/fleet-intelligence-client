// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package updatecheck

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpgradePlanRunTruncatedDownloadPreservesExecutable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "100")
		_, _ = io.WriteString(w, "short installer")
	}))
	defer server.Close()

	dir := t.TempDir()
	executable := filepath.Join(dir, "nvfleetint")
	original := []byte("original executable bytes")
	if err := os.WriteFile(executable, original, 0o700); err != nil {
		t.Fatal(err)
	}
	plan := UpgradePlan{InstallerURL: server.URL, ExecutablePath: executable, InstallDir: dir}
	var progress bytes.Buffer
	err := plan.Run(context.Background(), &progress)
	if !errors.Is(err, io.ErrUnexpectedEOF) || !strings.Contains(err.Error(), "could not download the installer") {
		t.Errorf("Run error = %v, want truncated download error", err)
	}
	got, err := os.ReadFile(executable)
	if err != nil {
		t.Fatalf("read executable after failed download: %v", err)
	}
	if !bytes.Equal(got, original) {
		t.Errorf("executable changed: got %q, want %q", got, original)
	}
	if progress.Len() != 0 {
		t.Errorf("installer progress = %q, want empty", progress.String())
	}
}
