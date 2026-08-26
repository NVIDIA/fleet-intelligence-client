// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/NVIDIA/fleet-intelligence-client/internal/generated/fleetapi"
)

const (
	NodeGroupViewDetail NodeGroupView = "detail"
	NodeGroupViewBasic  NodeGroupView = "basic"

	NodeGroupHealthHealthy   NodeGroupHealthStatus = "Healthy"
	NodeGroupHealthDegraded  NodeGroupHealthStatus = "Degraded"
	NodeGroupHealthUnhealthy NodeGroupHealthStatus = "Unhealthy"
	NodeGroupHealthUnknown   NodeGroupHealthStatus = "Unknown"

	NodeGroupSortByHealth NodeGroupSortBy = "health"
	NodeGroupSortByNodes  NodeGroupSortBy = "nodes"

	NodeGroupOrderAsc  NodeGroupSortOrder = "asc"
	NodeGroupOrderDesc NodeGroupSortOrder = "desc"
)

// Represents supported response shapes for listing node groups
type NodeGroupView string

// Reports whether the view is accepted by the API
func (view NodeGroupView) Valid() bool {
	return fleetapi.GetV1NodegroupsParamsView(view).Valid()
}

// Represents supported health filters for listing node groups
type NodeGroupHealthStatus string

// Reports whether the health status is accepted by the API
func (status NodeGroupHealthStatus) Valid() bool {
	return fleetapi.GetV1NodegroupsParamsHealthStatuses(status).Valid()
}

// Represents supported sort fields for listing node groups
type NodeGroupSortBy string

// Reports whether the sort field is accepted by the API
func (sortBy NodeGroupSortBy) Valid() bool {
	return fleetapi.GetV1NodegroupsParamsSortBy(sortBy).Valid()
}

// Represents supported sort orders for listing node groups
type NodeGroupSortOrder string

// Reports whether the sort order is accepted by the API
func (order NodeGroupSortOrder) Valid() bool {
	return fleetapi.GetV1NodegroupsParamsOrder(order).Valid()
}

// Represents request options for listing node groups
type ListNodeGroupsOptions struct {
	View             NodeGroupView
	IncludeMetrics   *bool
	ComputeZoneIDs   []string
	ComputeZoneNames []string
	NodeGroupIDs     []string
	HealthStatuses   []NodeGroupHealthStatus
	GPUTypes         []string
	SortBy           NodeGroupSortBy
	Order            NodeGroupSortOrder
	Page             *int
	PageSize         *int
}

// Represents a paginated node group response with the raw backend payload
type NodeGroupsPage struct {
	NodeGroups []NodeGroup `json:"nodeGroups"`
	HasMore    bool        `json:"hasMore"`
	Page       int         `json:"page"`
	PageSize   int         `json:"pageSize"`
	Total      int         `json:"total"`
	RawJSON    []byte      `json:"-"`
}

// PageInfo reports the pagination envelope of the response.
func (page NodeGroupsPage) PageInfo() PageInfo {
	hasMore := page.HasMore
	return PageInfo{
		Page:     page.Page,
		PageSize: page.PageSize,
		Total:    page.Total,
		HasMore:  &hasMore,
		RawJSON:  page.RawJSON,
	}
}

// Represents a node group
type NodeGroup struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	ComputeZoneID    string   `json:"computeZoneId,omitempty"`
	ComputeZoneName  string   `json:"computeZoneName,omitempty"`
	Health           string   `json:"health,omitempty"`
	HealthPercentage *float32 `json:"healthPercentage,omitempty"`
	NodeCount        *int     `json:"nodeCount,omitempty"`
}

// Lists node groups using the configured API client
func (c *Client) ListNodeGroups(ctx context.Context, opts ListNodeGroupsOptions) (NodeGroupsPage, error) {
	ctx, cancel := c.requestContext(ctx)
	defer cancel()

	view, err := opts.normalize()
	if err != nil {
		return NodeGroupsPage{}, err
	}

	params := fleetapi.GetV1NodegroupsParams{
		View:           nodeGroupViewParam(view),
		IncludeMetrics: cloneBool(opts.IncludeMetrics),
		ComputeZoneIds: optionalSlice(opts.ComputeZoneIDs),
		NodeGroupIds:   optionalSlice(opts.NodeGroupIDs),
		SortBy:         optionalEnum[fleetapi.GetV1NodegroupsParamsSortBy](opts.SortBy),
		Order:          optionalEnum[fleetapi.GetV1NodegroupsParamsOrder](opts.Order),
		Page:           cloneInt(opts.Page),
		PageSize:       cloneInt(opts.PageSize),
	}
	// Basic view rejects these three filters (validateNodeGroupOptions enforces
	// it), so they are only ever sent for the detail view.
	if view == NodeGroupViewDetail {
		params.ComputeZoneNames = optionalSlice(opts.ComputeZoneNames)
		params.HealthStatuses = optionalEnumSlice[fleetapi.GetV1NodegroupsParamsHealthStatuses](opts.HealthStatuses)
		params.GpuTypes = optionalSlice(opts.GPUTypes)
	}

	resp, err := c.api.GetV1NodegroupsWithResponse(ctx, &params)
	if err != nil {
		return NodeGroupsPage{}, err
	}
	if resp.StatusCode() != http.StatusOK {
		return NodeGroupsPage{}, newAPIError(resp.StatusCode(), resp.Status(), resp.Body)
	}

	if view == NodeGroupViewBasic {
		return decodeBasicNodeGroups(resp.Body)
	}

	return decodeDetailNodeGroups(resp.Body)
}

// The accepted values named in each node group option's error
const (
	nodeGroupViewValues   = "basic or detail"
	nodeGroupHealthValues = "Healthy, Degraded, Unhealthy, or Unknown"
	nodeGroupSortByValues = "health or nodes"
	nodeGroupOrderValues  = "asc or desc"
)

// Validate reports whether the options describe a request the API accepts.
// ListNodeGroups calls it, and a caller can call it first to reject a bad
// request without opening a connection.
func (opts ListNodeGroupsOptions) Validate() error {
	_, err := opts.normalize()
	return err
}

// Defaults an omitted view and checks every option against it
func (opts ListNodeGroupsOptions) normalize() (NodeGroupView, error) {
	view := opts.View
	if view == "" {
		view = NodeGroupViewDetail
	} else if !view.Valid() {
		return "", invalidOption("view", "node group view", string(view), nodeGroupViewValues)
	}

	for _, status := range opts.HealthStatuses {
		if !status.Valid() {
			return "", invalidOption("health", "node group health", string(status), nodeGroupHealthValues)
		}
	}
	if opts.SortBy != "" && !opts.SortBy.Valid() {
		return "", invalidOption("sortBy", "node group sort", string(opts.SortBy), nodeGroupSortByValues)
	}
	if opts.Order != "" && !opts.Order.Valid() {
		return "", invalidOption("order", "node group order", string(opts.Order), nodeGroupOrderValues)
	}

	if view == NodeGroupViewBasic {
		switch {
		case opts.IncludeMetrics != nil:
			return "", errors.New("basic node group view is incompatible with include metrics")
		case len(opts.ComputeZoneNames) > 0:
			return "", errors.New("basic node group view is incompatible with compute zone name filters")
		case len(opts.HealthStatuses) > 0 || len(opts.GPUTypes) > 0:
			return "", errors.New("basic node group view is incompatible with health and GPU type filters")
		case opts.SortBy != "":
			return "", fmt.Errorf("basic node group view is incompatible with sort %q", opts.SortBy)
		case opts.Order != "":
			return "", fmt.Errorf("basic node group view is incompatible with sort order %q", opts.Order)
		}
	}

	return view, nil
}

// Converts a normalized view into the generated parameter type
func nodeGroupViewParam(view NodeGroupView) *fleetapi.GetV1NodegroupsParamsView {
	param := fleetapi.GetV1NodegroupsParamsView(view)
	return &param
}

// Decodes detail responses and preserves the original payload
func decodeDetailNodeGroups(data []byte) (NodeGroupsPage, error) {
	var resp fleetapi.ModelsNodeGroupsResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return NodeGroupsPage{}, err
	}

	page := NodeGroupsPage{
		HasMore:  boolValue(resp.HasMore),
		Page:     intValue(resp.Page),
		PageSize: intValue(resp.PageSize),
		Total:    intValue(resp.Total),
		RawJSON:  append([]byte(nil), data...),
	}
	if resp.NodeGroups != nil {
		page.NodeGroups = make([]NodeGroup, 0, len(*resp.NodeGroups))
		for _, group := range *resp.NodeGroups {
			page.NodeGroups = append(page.NodeGroups, nodeGroupFromOverview(group))
		}
	}

	return page, nil
}

// Decodes basic responses and preserves the original payload
func decodeBasicNodeGroups(data []byte) (NodeGroupsPage, error) {
	var resp fleetapi.ModelsListNodeGroupsResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return NodeGroupsPage{}, err
	}

	page := NodeGroupsPage{
		HasMore:  boolValue(resp.HasMore),
		Page:     intValue(resp.Page),
		PageSize: intValue(resp.PageSize),
		Total:    intValue(resp.Total),
		RawJSON:  append([]byte(nil), data...),
	}
	if resp.NodeGroups != nil {
		page.NodeGroups = make([]NodeGroup, 0, len(*resp.NodeGroups))
		for _, group := range *resp.NodeGroups {
			page.NodeGroups = append(page.NodeGroups, nodeGroupFromSimple(group))
		}
	}

	return page, nil
}

// Maps detail API models into SDK values
func nodeGroupFromOverview(group fleetapi.ModelsNodeGroupOverview) NodeGroup {
	return NodeGroup{
		ID:               stringValue(group.Id),
		Name:             stringValue(group.Name),
		ComputeZoneID:    stringValue(group.ComputeZoneId),
		ComputeZoneName:  stringValue(group.ComputeZoneName),
		Health:           enumStringValue(group.HealthState),
		HealthPercentage: cloneFloat32(group.HealthPercentage),
		NodeCount:        cloneInt(group.NodesCount),
	}
}

// Maps basic API models into SDK values
func nodeGroupFromSimple(group fleetapi.ModelsSimpleNodeGroup) NodeGroup {
	return NodeGroup{
		ID:              stringValue(group.Id),
		Name:            stringValue(group.Name),
		ComputeZoneID:   stringValue(group.ComputeZoneId),
		ComputeZoneName: stringValue(group.ComputeZoneName),
	}
}
