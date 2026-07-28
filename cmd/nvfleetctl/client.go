// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/NVIDIA/fleet-intelligence-client/internal/config"
	"github.com/NVIDIA/fleet-intelligence-client/pkg/fleetintelligence"
)

// Builds an SDK client from the stored auth config
func newConfiguredClient(opts ...fleetintelligence.Option) (*fleetintelligence.Client, error) {
	cfg, err := config.LoadWithEnv()
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.ServiceKey) == "" {
		return nil, errors.New("service key is not configured; run `nvfleetctl auth login --key <service-key>`")
	}

	client, err := fleetintelligence.NewClient(cfg.APIURL, cfg.ServiceKey, opts...)
	if err != nil {
		// The URL can come from the config file or the environment, so name
		// both places the user might need to fix.
		if errors.Is(err, fleetintelligence.ErrInsecureBaseURL) {
			return nil, fmt.Errorf(
				"%w; update %s or run `nvfleetctl auth login --key <service-key> --api-url <https-url>`",
				err, config.EnvAPIURL,
			)
		}
		return nil, err
	}

	return client, nil
}

// Builds the SDK options implied by resolved common flags
func commonClientOptions(common resolvedCommonFlags) []fleetintelligence.Option {
	return []fleetintelligence.Option{
		fleetintelligence.WithTimeout(common.timeout),
	}
}
