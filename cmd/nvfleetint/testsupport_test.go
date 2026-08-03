// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/config"
)

// testProfile is the profile name written by saveTestConfig. Commands under
// test resolve it as the current profile, so they need no --profile flag.
const testProfile = "default"

// saveTestConfig points the CLI at a test server by storing a single profile.
// It also clears the credential environment variables, so a developer's shell
// cannot bleed into a test run.
func saveTestConfig(t *testing.T, apiURL, serviceKey string) {
	t.Helper()
	clearCredentialEnv(t)

	var cfg config.Config
	if err := cfg.AddProfile(testProfile, config.Profile{APIURL: apiURL, ServiceKey: serviceKey}); err != nil {
		t.Fatalf("add profile failed: %v", err)
	}
	if err := config.Save(cfg); err != nil {
		t.Fatalf("save config failed: %v", err)
	}
}

// clearCredentialEnv unsets the variables that would otherwise override the
// stored profile.
func clearCredentialEnv(t *testing.T) {
	t.Helper()
	t.Setenv(config.EnvAPIURL, "")
	t.Setenv(config.EnvServiceKey, "")
	t.Setenv(config.EnvProfile, "")
}
