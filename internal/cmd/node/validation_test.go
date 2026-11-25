// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package node

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestNodeCommandsRequireCredentials(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "list", args: []string{"node", "list"}},
		{name: "describe", args: []string{"node", "describe", "node-1"}},
		{name: "options", args: []string{"node", "options"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			t.Setenv("NVFLEETINT_API_KEY", "")
			t.Setenv("NVFLEETINT_API_URL", "")
			t.Setenv("NVFLEETINT_SERVICE_KEY", "")
			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected missing credential error")
			}
			for _, want := range []string{"no profile configured", "auth add", "NVFLEETINT_API_KEY"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
			if out.Len() != 0 {
				t.Errorf("expected empty stdout, got %q", out.String())
			}
		})
	}
}
