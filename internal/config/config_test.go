// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"bytes"
	"os"
	"testing"
)

// clearEnv removes the credential environment variables so a test starts from a
// known state regardless of the developer's shell.
func clearEnv(t *testing.T) {
	t.Helper()
	t.Setenv(EnvAPIURL, "")
	t.Setenv(EnvAPIKey, "")
}

func assertBlockerUnchanged(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("blocking file changed: got %q, want %q, err=%v", got, want, err)
	}
}
