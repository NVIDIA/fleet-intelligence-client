// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/NVIDIA/fleet-intelligence-client/internal/generated/fleetapi"
)

// FilterOptions represents the filter values and sorting choices a list
// endpoint accepts. The backend returns this same envelope for nodes, node
// groups, and the alert timeline.
type FilterOptions struct {
	Filters Filters        `json:"filters"`
	Sorting SortingOptions `json:"sorting"`
	RawJSON []byte         `json:"-"`
}

// Filters contains the filter fields available for the requested view.
type Filters struct {
	Fields []FilterField `json:"fields"`
}

// FilterField represents one filter and the values it accepts.
type FilterField struct {
	Name    string         `json:"name"`
	Options []FilterOption `json:"options"`
}

// FilterOption represents one filter value. The backend sends either a bare
// string or an object carrying an ID and a display value; an object may also
// nest child options, as a compute zone nests its node groups.
type FilterOption struct {
	ID      string
	Value   string
	Options []FilterOption
}

// UnmarshalJSON decodes the string and object forms used by the shared options
// API.
func (option *FilterOption) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err == nil {
		option.ID = ""
		option.Value = value
		option.Options = nil
		return nil
	}

	var object struct {
		ID      string         `json:"id"`
		Value   string         `json:"value"`
		Options []FilterOption `json:"options"`
	}
	if err := json.Unmarshal(data, &object); err != nil {
		return fmt.Errorf("filter option must be a string or object: %w", err)
	}
	option.ID = object.ID
	option.Value = object.Value
	option.Options = object.Options
	return nil
}

// MarshalJSON preserves the option's original string or object shape.
func (option FilterOption) MarshalJSON() ([]byte, error) {
	if option.ID == "" && len(option.Options) == 0 {
		return json.Marshal(option.Value)
	}
	return json.Marshal(struct {
		ID      string         `json:"id"`
		Value   string         `json:"value"`
		Options []FilterOption `json:"options,omitempty"`
	}{ID: option.ID, Value: option.Value, Options: option.Options})
}

// SortingOptions describes the supported sort fields, orders, and defaults.
type SortingOptions struct {
	Fields   []string        `json:"fields"`
	Orders   []string        `json:"orders"`
	Defaults SortingDefaults `json:"defaults"`
}

// SortingDefaults reports the sort field and order applied when none is given.
type SortingDefaults struct {
	Field string `json:"field"`
	Order string `json:"order"`
}

// GetNodeFilterOptions gets the filters and sorting choices available when
// listing nodes. An empty agentType leaves the view to the backend default.
// The OOB (BMC/Redfish) view returns a reduced set of filters and sort fields.
func (c *Client) GetNodeFilterOptions(ctx context.Context, agentType NodeAgentType) (FilterOptions, error) {
	if agentType != "" && !agentType.Valid() {
		return FilterOptions{}, fmt.Errorf("invalid node agent type %q: expected inband or oob", agentType)
	}

	ctx, cancel := c.requestContext(ctx)
	defer cancel()

	params := fleetapi.GetV1NodesOptionsParams{}
	if agentType != "" {
		value := fleetapi.GetV1NodesOptionsParamsAgentType(agentType)
		params.AgentType = &value
	}

	resp, err := c.api.GetV1NodesOptions(ctx, &params)
	if err != nil {
		return FilterOptions{}, err
	}
	return decodeFilterOptions(resp, "node")
}

// GetNodeGroupFilterOptions gets the filters and sorting choices available when
// listing node groups.
func (c *Client) GetNodeGroupFilterOptions(ctx context.Context) (FilterOptions, error) {
	ctx, cancel := c.requestContext(ctx)
	defer cancel()

	resp, err := c.api.GetV1NodegroupsOptions(ctx)
	if err != nil {
		return FilterOptions{}, err
	}
	return decodeFilterOptions(resp, "node group")
}

// Reads and decodes a shared options envelope, preserving the raw payload. The
// generated client models a filter option as an empty object, so the response
// is decoded from the raw body rather than the generated type.
func decodeFilterOptions(resp *http.Response, resource string) (FilterOptions, error) {
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return FilterOptions{}, fmt.Errorf("read %s filter options: %w", resource, err)
	}
	if resp.StatusCode != http.StatusOK {
		return FilterOptions{}, newAPIError(resp.StatusCode, resp.Status, body)
	}

	var options FilterOptions
	if err := json.Unmarshal(body, &options); err != nil {
		return FilterOptions{}, fmt.Errorf("decode %s filter options: %w", resource, err)
	}
	options.RawJSON = append([]byte(nil), body...)
	return options, nil
}
