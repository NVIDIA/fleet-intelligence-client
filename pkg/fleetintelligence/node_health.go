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

package fleetintelligence

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/NVIDIA/fleet-intelligence-client/internal/generated/fleetapi"
)

// NodeHealthHistoryOptions represents the time window for a node health history query
type NodeHealthHistoryOptions struct {
	// StartTime is the start of the window in RFC3339 format (required)
	StartTime string
	// EndTime is the end of the window in RFC3339 format (required)
	EndTime string
}

// NodeHealthHistory represents a node's health timeline and summary over a window
type NodeHealthHistory struct {
	EnrolledAt    string              `json:"enrolledAt,omitempty"`
	HealthSummary *NodeHealthSummary  `json:"healthSummary,omitempty"`
	Segments      []NodeHealthSegment `json:"machineStatus,omitempty"`
	RawJSON       []byte              `json:"-"`
}

// NodeHealthSummary represents aggregate health durations and percentages for a node
type NodeHealthSummary struct {
	HealthyPercentage        *float32 `json:"healthyPercentage,omitempty"`
	DegradedPercentage       *float32 `json:"degradedPercentage,omitempty"`
	UnhealthyPercentage      *float32 `json:"unhealthyPercentage,omitempty"`
	HealthyDurationSeconds   *float32 `json:"healthyDurationSeconds,omitempty"`
	DegradedDurationSeconds  *float32 `json:"degradedDurationSeconds,omitempty"`
	UnhealthyDurationSeconds *float32 `json:"unhealthyDurationSeconds,omitempty"`
}

// NodeHealthSegment represents a single health state interval in a node's timeline
type NodeHealthSegment struct {
	Status    string `json:"status,omitempty"`
	StartTime string `json:"startTime,omitempty"`
	EndTime   string `json:"endTime,omitempty"`
}

// NodeHealthHistory retrieves the health timeline and summary for a single node
func (c *Client) NodeHealthHistory(ctx context.Context, nodeUUID string, opts NodeHealthHistoryOptions) (NodeHealthHistory, error) {
	ctx, cancel := c.requestContext(ctx)
	defer cancel()

	nodeUUID = strings.TrimSpace(nodeUUID)
	opts.StartTime = strings.TrimSpace(opts.StartTime)
	opts.EndTime = strings.TrimSpace(opts.EndTime)

	if nodeUUID == "" {
		return NodeHealthHistory{}, fmt.Errorf("node UUID is required")
	}
	if opts.StartTime == "" || opts.EndTime == "" {
		return NodeHealthHistory{}, fmt.Errorf("start and end times are required")
	}
	if err := validateNodeHealthRFC3339("start time", opts.StartTime); err != nil {
		return NodeHealthHistory{}, err
	}
	if err := validateNodeHealthRFC3339("end time", opts.EndTime); err != nil {
		return NodeHealthHistory{}, err
	}

	params := fleetapi.GetV1NodesNodeUuidHealthHistoryParams{
		StartTime: opts.StartTime,
		EndTime:   opts.EndTime,
	}

	resp, err := c.api.GetV1NodesNodeUuidHealthHistoryWithResponse(ctx, nodeUUID, &params)
	if err != nil {
		return NodeHealthHistory{}, err
	}
	if resp.StatusCode() != http.StatusOK {
		return NodeHealthHistory{}, newAPIError(resp.StatusCode(), resp.Status(), resp.Body)
	}

	var data fleetapi.ModelsNodeHealthHistoryResponse
	if err := json.Unmarshal(resp.Body, &data); err != nil {
		return NodeHealthHistory{}, err
	}

	history := nodeHealthHistoryFromGenerated(data)
	history.RawJSON = append([]byte(nil), resp.Body...)
	return history, nil
}

// Checks a node health timestamp value is RFC3339
func validateNodeHealthRFC3339(name, value string) error {
	if _, err := time.Parse(time.RFC3339, value); err != nil {
		return fmt.Errorf("node health %s must be RFC3339", name)
	}
	return nil
}

// Maps the generated health history model into SDK values
func nodeHealthHistoryFromGenerated(data fleetapi.ModelsNodeHealthHistoryResponse) NodeHealthHistory {
	history := NodeHealthHistory{
		EnrolledAt:    stringValue(data.EnrolledAt),
		HealthSummary: nodeHealthSummaryFromGenerated(data.HealthSummary),
	}
	if data.MachineStatus != nil {
		history.Segments = make([]NodeHealthSegment, 0, len(*data.MachineStatus))
		for _, segment := range *data.MachineStatus {
			history.Segments = append(history.Segments, nodeHealthSegmentFromGenerated(segment))
		}
	}
	return history
}

// Maps the generated health summary model into SDK values
func nodeHealthSummaryFromGenerated(summary *fleetapi.ModelsHealthSummary) *NodeHealthSummary {
	if summary == nil {
		return nil
	}
	return &NodeHealthSummary{
		HealthyPercentage:        cloneFloat32(summary.HealthyPercentage),
		DegradedPercentage:       cloneFloat32(summary.DegradedPercentage),
		UnhealthyPercentage:      cloneFloat32(summary.UnhealthyPercentage),
		HealthyDurationSeconds:   cloneFloat32(summary.HealthyDurationSeconds),
		DegradedDurationSeconds:  cloneFloat32(summary.DegradedDurationSeconds),
		UnhealthyDurationSeconds: cloneFloat32(summary.UnhealthyDurationSeconds),
	}
}

// Maps a generated health segment model into SDK values
func nodeHealthSegmentFromGenerated(segment fleetapi.ModelsHealthSegment) NodeHealthSegment {
	return NodeHealthSegment{
		Status:    enumStringValue(segment.Status),
		StartTime: stringValue(segment.StartTime),
		EndTime:   stringValue(segment.EndTime),
	}
}
