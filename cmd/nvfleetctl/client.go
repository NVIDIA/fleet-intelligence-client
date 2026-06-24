// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"errors"
	"strings"

	"github.com/NVIDIA/fleet-intelligence-client/internal/config"
	"github.com/NVIDIA/fleet-intelligence-client/pkg/fleetintelligence"
)

// Builds an SDK client from the stored auth config
func newConfiguredClient(opts ...fleetintelligence.Option) (*fleetintelligence.Client, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.ServiceKey) == "" {
		return nil, errors.New("service key is not configured; run `nvfleetctl auth login --key <service-key>`")
	}

	return fleetintelligence.NewClient(cfg.APIURL, cfg.ServiceKey, opts...)
}

// Builds the SDK options implied by resolved common flags
func commonClientOptions(common resolvedCommonFlags) []fleetintelligence.Option {
	return []fleetintelligence.Option{
		fleetintelligence.WithTimeout(common.timeout),
	}
}
