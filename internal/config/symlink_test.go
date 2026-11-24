// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveWritesThroughSymlinkedConfig(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	target := filepath.Join(homeDir, "dotfiles", "nvfleetint.yaml")
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatalf("mkdir target dir failed: %v", err)
	}
	if err := os.WriteFile(target, []byte("profiles: {}\n"), fileMode); err != nil {
		t.Fatalf("write target failed: %v", err)
	}

	path := filepath.Join(homeDir, ".config", "nvfleetint", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		t.Fatalf("mkdir config dir failed: %v", err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatalf("symlink failed: %v", err)
	}

	var cfg Config
	if err := cfg.AddProfile("dev", Profile{APIKey: "dev-key"}); err != nil {
		t.Fatalf("add failed: %v", err)
	}
	if err := Save(cfg); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("lstat failed: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected config path to remain a symlink, got mode %s", info.Mode())
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target failed: %v", err)
	}
	if !strings.Contains(string(data), "dev-key") {
		t.Fatalf("symlink target was not updated:\n%s", data)
	}
}
