// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMainVersionJSONBuildInfo(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("HOME", configDir)
	t.Setenv("XDG_CONFIG_HOME", configDir)

	out, err := os.Create(filepath.Join(t.TempDir(), "stdout"))
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()

	oldArgs, oldStdout := os.Args, os.Stdout
	oldVersion, oldCommit, oldBuildDate := version, commit, buildDate
	defer func() {
		os.Args, os.Stdout = oldArgs, oldStdout
		version, commit, buildDate = oldVersion, oldCommit, oldBuildDate
	}()

	version, commit, buildDate = "dev", "commit-sentinel", "build-date-sentinel"
	os.Args = []string{"nvfleetint", "version", "--output", "json"}
	os.Stdout = out
	main()

	data, err := os.ReadFile(out.Name())
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Name      string `json:"name"`
		Version   string `json:"version"`
		Commit    string `json:"commit"`
		BuildDate string `json:"buildDate"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("decode version output %q: %v", data, err)
	}
	if got.Name != "nvfleetint" || got.Version != "dev" || got.Commit != "commit-sentinel" || got.BuildDate != "build-date-sentinel" {
		t.Errorf("unexpected build information: %+v", got)
	}
}

func TestMainCommandErrorJSON(t *testing.T) {
	const helperEnv = "NVFLEETINT_MAIN_ERROR_HELPER"
	if os.Getenv(helperEnv) == "1" {
		os.Args = []string{"nvfleetint", "version", "--not-a-real-flag", "--output", "json"}
		main()
		return
	}

	configDir := t.TempDir()
	t.Setenv("HOME", configDir)
	t.Setenv("XDG_CONFIG_HOME", configDir)
	t.Setenv(helperEnv, "1")
	cmd := exec.Command(os.Args[0], "-test.run=^TestMainCommandErrorJSON$")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	exitErr, ok := err.(*exec.ExitError)
	if !ok || exitErr.ExitCode() != 1 {
		t.Fatalf("expected exit code 1, got %v; stdout: %q; stderr: %q", err, stdout.String(), stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("expected empty stdout, got %q", stdout.String())
	}
	var got struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stderr.Bytes(), &got); err != nil {
		t.Fatalf("decode error output %q: %v", stderr.String(), err)
	}
	if got.Error.Code != "command_error" {
		t.Errorf("error code = %q, want command_error", got.Error.Code)
	}
	if !strings.Contains(got.Error.Message, "unknown flag: --not-a-real-flag") {
		t.Errorf("error message does not identify unknown flag: %q", got.Error.Message)
	}
}
