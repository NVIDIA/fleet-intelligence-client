// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"context"
	"encoding/json"
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
	View           NodeGroupView
	NodeGroupIDs   []string
	HealthStatuses []NodeGroupHealthStatus
	GPUTypes       []string
	SortBy         NodeGroupSortBy
	Order          NodeGroupSortOrder
	Page           *int
	PageSize       *int
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

	view, err := normalizeNodeGroupView(opts.View)
	if err != nil {
		return NodeGroupsPage{}, err
	}
	if err := validateNodeGroupOptions(view, opts); err != nil {
		return NodeGroupsPage{}, err
	}

	params := fleetapi.GetV1NodegroupsParams{
		View: nodeGroupViewParam(view),
	}
	if len(opts.NodeGroupIDs) > 0 {
		nodeGroupIDs := append([]string(nil), opts.NodeGroupIDs...)
		params.NodeGroupIds = &nodeGroupIDs
	}
	if view == NodeGroupViewDetail {
		if len(opts.HealthStatuses) > 0 {
			statuses := make([]fleetapi.GetV1NodegroupsParamsHealthStatuses, 0, len(opts.HealthStatuses))
			for _, status := range opts.HealthStatuses {
				statuses = append(statuses, fleetapi.GetV1NodegroupsParamsHealthStatuses(status))
			}
			params.HealthStatuses = &statuses
		}
		if len(opts.GPUTypes) > 0 {
			gpuTypes := append([]string(nil), opts.GPUTypes...)
			params.GpuTypes = &gpuTypes
		}
	}
	if opts.SortBy != "" {
		sortBy := fleetapi.GetV1NodegroupsParamsSortBy(opts.SortBy)
		params.SortBy = &sortBy
	}
	if opts.Order != "" {
		order := fleetapi.GetV1NodegroupsParamsOrder(opts.Order)
		params.Order = &order
	}
	if opts.Page != nil {
		params.Page = cloneInt(opts.Page)
	}
	if opts.PageSize != nil {
		params.PageSize = cloneInt(opts.PageSize)
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

// Defaults an omitted view and rejects unsupported values
func normalizeNodeGroupView(view NodeGroupView) (NodeGroupView, error) {
	if view == "" {
		return NodeGroupViewDetail, nil
	}
	if !view.Valid() {
		return "", fmt.Errorf("invalid node group view %q: expected basic or detail", view)
	}

	return view, nil
}

// Checks node group list options before making the request
func validateNodeGroupOptions(view NodeGroupView, opts ListNodeGroupsOptions) error {
	for _, status := range opts.HealthStatuses {
		if !status.Valid() {
			return fmt.Errorf("invalid node group health %q: expected Healthy, Degraded, Unhealthy, or Unknown", status)
		}
	}
	if opts.SortBy != "" && !opts.SortBy.Valid() {
		return fmt.Errorf("invalid node group sort %q: expected health or nodes", opts.SortBy)
	}
	if opts.Order != "" && !opts.Order.Valid() {
		return fmt.Errorf("invalid node group order %q: expected asc or desc", opts.Order)
	}
	if view == NodeGroupViewBasic && (len(opts.HealthStatuses) > 0 || len(opts.GPUTypes) > 0) {
		return fmt.Errorf("basic node group view is incompatible with health and GPU type filters")
	}
	if view == NodeGroupViewBasic && opts.SortBy != "" {
		return fmt.Errorf("basic node group view is incompatible with sort %q", opts.SortBy)
	}
	if view == NodeGroupViewBasic && opts.Order != "" {
		return fmt.Errorf("basic node group view is incompatible with sort order %q", opts.Order)
	}

	return nil
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
