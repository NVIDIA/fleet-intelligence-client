// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package cmdutil

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
)

func TestWriteCLIErrorJSON(t *testing.T) {
	var out bytes.Buffer
	Write(&out, []string{"node", "list", "--output", "json"}, errors.New("bad input"))

	var got errorOutput
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode error JSON failed: %v", err)
	}
	if got.Error.Code != "command_error" || got.Error.Message != "bad input" {
		t.Fatalf("unexpected error JSON: %#v", got)
	}
}
func TestWriteCLIErrorJSONForParseErrorArgs(t *testing.T) {
	var out bytes.Buffer
	Write(&out, []string{"version", "--output", "json", "--badflag"}, errors.New("unknown flag: --badflag"))

	var got errorOutput
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode error JSON failed: %v", err)
	}
	if got.Error.Code != "command_error" || got.Error.Message != "unknown flag: --badflag" {
		t.Fatalf("unexpected error JSON: %#v", got)
	}
}
func TestWriteCLIErrorIncludesAPIDetails(t *testing.T) {
	var out bytes.Buffer
	err := &nvfleetint.APIError{
		StatusCode: 403,
		Status:     "Forbidden",
		Message:    "permission denied",
		Details:    "missing role",
	}
	Write(&out, []string{"node", "list", "--output", "json"}, err)

	var got errorOutput
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode error JSON failed: %v", err)
	}
	if got.Error.Code != "api_error" || got.Error.StatusCode != 403 || got.Error.Status != "Forbidden" || got.Error.Message != "permission denied" || got.Error.Details != "missing role" {
		t.Fatalf("unexpected error JSON: %#v", got)
	}
}
func TestExitCodeForPermissionErrors(t *testing.T) {
	if got := ExitCode(&nvfleetint.APIError{StatusCode: 403}); got != exitNoPermission {
		t.Fatalf("unexpected permission exit code: %d", got)
	}
	if got := ExitCode(&nvfleetint.APIError{StatusCode: 500}); got != exitError {
		t.Fatalf("unexpected general exit code: %d", got)
	}
}
