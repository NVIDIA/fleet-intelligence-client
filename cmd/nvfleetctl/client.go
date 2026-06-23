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
