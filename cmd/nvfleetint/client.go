// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/NVIDIA/fleet-intelligence-client/internal/config"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
)

// Builds an SDK client from the credentials selected by the common flags
func newConfiguredClient(common resolvedCommonFlags) (*nvfleetint.Client, error) {
	resolved, err := resolveCredentials(common.profile)
	if err != nil {
		return nil, err
	}

	return clientFromResolved(resolved, common)
}

func clientFromResolved(resolved config.Resolved, common resolvedCommonFlags) (*nvfleetint.Client, error) {
	if strings.TrimSpace(resolved.ServiceKey) == "" {
		return nil, missingServiceKeyError(resolved)
	}

	client, err := nvfleetint.NewClient(resolved.APIURL, resolved.ServiceKey, commonClientOptions(common)...)
	if err != nil {
		// The URL can come from a profile or the environment, so name the place
		// the user actually needs to fix.
		if errors.Is(err, nvfleetint.ErrInsecureBaseURL) {
			return nil, fmt.Errorf("%w; %s", err, fixAPIURLHint(resolved))
		}
		return nil, err
	}

	return client, nil
}

// Resolves credentials, adding CLI hints to the config package's errors
func resolveCredentials(profile string) (config.Resolved, error) {
	resolved, err := config.Resolve(profile)
	if err != nil {
		if errors.Is(err, config.ErrProfileNotFound) {
			return config.Resolved{}, fmt.Errorf("%w; run `nvfleetint auth list` to see the configured profiles", err)
		}
		return config.Resolved{}, err
	}

	return resolved, nil
}

// Explains which profile (if any) is missing a service key
func missingServiceKeyError(resolved config.Resolved) error {
	if resolved.Profile == "" {
		if resolved.ProfilesConfigured {
			return fmt.Errorf(
				"%w; run `nvfleetint auth use --profile <name>`, or pass --profile <name>",
				config.ErrNoProfile,
			)
		}
		return fmt.Errorf(
			"%w; run `nvfleetint auth add --profile <name> --key <service-key>`, or set %s",
			config.ErrNoProfile, config.EnvServiceKey,
		)
	}

	return fmt.Errorf(
		"profile %q has no service key; run `nvfleetint auth update --profile %s --key <service-key>`",
		resolved.Profile, resolved.Profile,
	)
}

// Names the place a rejected API URL came from
func fixAPIURLHint(resolved config.Resolved) string {
	if resolved.APIURLSource == config.SourceEnvironment {
		return "update " + config.EnvAPIURL
	}
	if resolved.Profile != "" {
		return fmt.Sprintf("run `nvfleetint auth update --profile %s --api-url <https-url>`", resolved.Profile)
	}

	return "run `nvfleetint auth add --profile <name> --key <service-key> --api-url <https-url>`"
}

// Builds the SDK options implied by resolved common flags
func commonClientOptions(common resolvedCommonFlags) []nvfleetint.Option {
	return []nvfleetint.Option{
		nvfleetint.WithTimeout(common.timeout),
	}
}
