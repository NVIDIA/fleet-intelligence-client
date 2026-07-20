// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package fleetintelligence

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/NVIDIA/fleet-intelligence-client/internal/generated/fleetapi"
)

// Represents request options for fetching the fleet overview
type OverviewOptions struct {
	// IncludeMetrics toggles the API's metrics payload. Nil uses the API
	// default (metrics included).
	IncludeMetrics *bool
}

// Represents the overall system and fleet summary
type Overview struct {
	NodesCount         *int             `json:"nodesCount,omitempty"`
	HealthyNodeCount   *int             `json:"healthNodeCount,omitempty"`
	DegradedNodeCount  *int             `json:"degradedNodeCount,omitempty"`
	UnhealthyNodeCount *int             `json:"unhealthyNodeCount,omitempty"`
	UnknownNodeCount   *int             `json:"unknownNodeCount,omitempty"`
	HealthPercentage   *float32         `json:"healthPercentage,omitempty"`
	NodeGroupCount     *int             `json:"nodeGroupCount,omitempty"`
	ComputeZoneCount   *int             `json:"computeZoneCount,omitempty"`
	GPUsCount          *int             `json:"gpusCount,omitempty"`
	CPUCoresCount      *int             `json:"cpuCoresCount,omitempty"`
	Metrics            []OverviewMetric `json:"metrics,omitempty"`
	RawJSON            []byte           `json:"-"`
}

// Represents a single fleet-level metric in the overview
type OverviewMetric struct {
	Name        string   `json:"name,omitempty"`
	Description string   `json:"description,omitempty"`
	Unit        string   `json:"unit,omitempty"`
	Value       *float32 `json:"value,omitempty"`
	Aggregation string   `json:"aggregation,omitempty"`
	LastUpdated string   `json:"lastUpdated,omitempty"`
}

// Fetches the system and fleet overview using the configured API client
func (c *Client) GetOverview(ctx context.Context, opts OverviewOptions) (Overview, error) {
	ctx, cancel := c.requestContext(ctx)
	defer cancel()

	// IncludeMetrics is nil unless the caller set it; the generated param is
	// omitempty, so a nil pointer omits the query param and the backend applies
	// its own default.
	params := fleetapi.GetV1OverviewParams{
		IncludeMetrics: opts.IncludeMetrics,
	}

	resp, err := c.api.GetV1OverviewWithResponse(ctx, &params)
	if err != nil {
		return Overview{}, err
	}
	if resp.StatusCode() != http.StatusOK {
		return Overview{}, newAPIError(resp.StatusCode(), resp.Status(), resp.Body)
	}

	var data fleetapi.ModelsOverviewResponse
	if err := json.Unmarshal(resp.Body, &data); err != nil {
		return Overview{}, err
	}

	overview := overviewFromGenerated(data)
	overview.RawJSON = append([]byte(nil), resp.Body...)
	return overview, nil
}

// Maps the generated overview model into SDK values
func overviewFromGenerated(data fleetapi.ModelsOverviewResponse) Overview {
	overview := Overview{
		NodesCount:         cloneInt(data.NodesCount),
		HealthyNodeCount:   cloneInt(data.HealthNodeCount),
		DegradedNodeCount:  cloneInt(data.DegradedNodeCount),
		UnhealthyNodeCount: cloneInt(data.UnhealthyNodeCount),
		UnknownNodeCount:   cloneInt(data.UnknownNodeCount),
		HealthPercentage:   cloneFloat32(data.HealthPercentage),
		NodeGroupCount:     cloneInt(data.NodeGroupCount),
		ComputeZoneCount:   cloneInt(data.ComputeZoneCount),
		GPUsCount:          cloneInt(data.GpusCount),
		CPUCoresCount:      cloneInt(data.CpuCoresCount),
	}
	if data.Metrics != nil {
		overview.Metrics = make([]OverviewMetric, 0, len(*data.Metrics))
		for _, metric := range *data.Metrics {
			overview.Metrics = append(overview.Metrics, metricFromGenerated(metric))
		}
	}
	return overview
}

// Maps a generated metric model into SDK values
func metricFromGenerated(metric fleetapi.ModelsMetric) OverviewMetric {
	return OverviewMetric{
		Name:        stringValue(metric.Name),
		Description: stringValue(metric.Description),
		Unit:        stringValue(metric.Unit),
		Value:       cloneFloat32(metric.Value),
		Aggregation: enumStringValue(metric.Aggregation),
		LastUpdated: stringValue(metric.LastUpdated),
	}
}
