// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"os"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
	"github.com/NVIDIA/fleet-intelligence-client/internal/config"
)

func TestAuthListReportsEnvironmentOverrides(t *testing.T) {
	const storedKey = "stored-secret-for-list"
	const envKey = "environment-secret-for-list"
	tests := []struct {
		name, url, key, note string
	}{
		{"url", "https://env.example.com", "", config.EnvAPIURL + " is"},
		{"key", "", envKey, config.EnvAPIKey + " is"},
		{"both", "https://env.example.com", envKey, config.EnvAPIURL + " / " + config.EnvAPIKey + " are"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			cmdtest.ClearCredentialEnv(t)
			cfg := config.Config{CurrentProfile: "selected", Profiles: map[string]config.Profile{
				"selected": {APIURL: apiURL, APIKey: storedKey},
				"keyless":  {APIURL: "https://keyless.example.com"},
			}}
			if err := config.Save(cfg); err != nil {
				t.Fatalf("save config: %v", err)
			}
			t.Setenv(config.EnvAPIURL, tt.url)
			t.Setenv(config.EnvAPIKey, tt.key)
			out := mustRunAuth(t, "list")
			rows := make(map[string]string)
			for _, line := range strings.Split(out, "\n") {
				fields := strings.Fields(line)
				if len(fields) > 0 {
					rows[fields[0]] = strings.Join(fields, " ")
				}
			}
			if rows["selected"] != "selected "+apiURL+" configured *" {
				t.Fatalf("unexpected selected row: %q", rows["selected"])
			}
			if rows["keyless"] != "keyless https://keyless.example.com not configured -" {
				t.Fatalf("unexpected keyless row: %q", rows["keyless"])
			}
			wantNote := tt.note + ` overriding values stored in profile "selected"; run ` + "`nvfleetint auth status`" + " for effective credentials."
			if !strings.Contains(out, wantNote) {
				t.Fatalf("missing override note %q in %q", wantNote, out)
			}
			if strings.Contains(out, storedKey) || strings.Contains(out, envKey) {
				t.Fatalf("list exposed a key: %q", out)
			}
		})
	}
}
