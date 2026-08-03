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

// Builds an SDK client from the stored auth config
func newConfiguredClient(opts ...nvfleetint.Option) (*nvfleetint.Client, error) {
	cfg, err := config.LoadWithEnv()
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.ServiceKey) == "" {
		return nil, errors.New("service key is not configured; run `nvfleetint auth login --key <service-key>`")
	}

	client, err := nvfleetint.NewClient(cfg.APIURL, cfg.ServiceKey, opts...)
	if err != nil {
		// The URL can come from the config file or the environment, so name
		// both places the user might need to fix.
		if errors.Is(err, nvfleetint.ErrInsecureBaseURL) {
			return nil, fmt.Errorf(
				"%w; update %s or run `nvfleetint auth login --key <service-key> --api-url <https-url>`",
				err, config.EnvAPIURL,
			)
		}
		return nil, err
	}

	return client, nil
}

// Builds the SDK options implied by resolved common flags
func commonClientOptions(common resolvedCommonFlags) []nvfleetint.Option {
	return []nvfleetint.Option{
		nvfleetint.WithTimeout(common.timeout),
	}
}
