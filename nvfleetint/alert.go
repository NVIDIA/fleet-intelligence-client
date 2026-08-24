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
	alertTimelineMinPageSize = 1
	alertTimelineMaxPageSize = 100

	AlertSeverityCritical AlertSeverity = "Critical"
	AlertSeverityWarning  AlertSeverity = "Warning"

	AlertStateDetected  AlertState = "Detected"
	AlertStateTriggered AlertState = "Triggered"
	AlertStateResolved  AlertState = "Resolved"

	AlertTimelineStateCritical AlertTimelineState = "Critical"
	AlertTimelineStateWarning  AlertTimelineState = "Warning"
	AlertTimelineStateResolved AlertTimelineState = "Resolved"

	AlertTimelineNodeSortByHostname    AlertTimelineNodeSortBy = "hostname"
	AlertTimelineNodeSortByAlert       AlertTimelineNodeSortBy = "alert"
	AlertTimelineNodeSortByGPUType     AlertTimelineNodeSortBy = "gpuType"
	AlertTimelineNodeSortByNodeGroup   AlertTimelineNodeSortBy = "nodeGroup"
	AlertTimelineNodeSortByComputeZone AlertTimelineNodeSortBy = "computeZone"
	AlertTimelineNodeSortByLastUpdate  AlertTimelineNodeSortBy = "lastUpdate"

	AlertTimelineAlertSortByStartTime  AlertTimelineAlertSortBy = "startTime"
	AlertTimelineAlertSortByLastUpdate AlertTimelineAlertSortBy = "lastUpdate"

	AlertTimelineOrderAsc  AlertTimelineSortOrder = "asc"
	AlertTimelineOrderDesc AlertTimelineSortOrder = "desc"
)

// Represents supported alert severity filters
type AlertSeverity string

// Reports whether the alert severity is accepted by the API
func (severity AlertSeverity) Valid() bool {
	return fleetapi.ModelsAlertSeverity(severity).Valid()
}

// Represents supported alert state filters
type AlertState string

// Reports whether the alert state is accepted by the API
func (state AlertState) Valid() bool {
	return fleetapi.ModelsAlertState(state).Valid()
}

// Represents alert states accepted by alert timeline filters
type AlertTimelineState string

// Reports whether the alert timeline state is accepted by the API
func (state AlertTimelineState) Valid() bool {
	return fleetapi.GetV1AlertTimelineNodesParamsAlertStates(state).Valid()
}

// Represents supported sort fields for alert timeline nodes
type AlertTimelineNodeSortBy string

// Reports whether the node sort field is accepted by the API
func (sortBy AlertTimelineNodeSortBy) Valid() bool {
	return fleetapi.GetV1AlertTimelineNodesParamsSortBy(sortBy).Valid()
}

// Represents supported sort fields for alerts within one node
type AlertTimelineAlertSortBy string

// Reports whether the alert sort field is accepted by the API
func (sortBy AlertTimelineAlertSortBy) Valid() bool {
	return fleetapi.GetV1AlertTimelineNodesNodeUuidAlertsParamsSortBy(sortBy).Valid()
}

// Represents supported sort orders for alert timeline endpoints
type AlertTimelineSortOrder string

// Reports whether the sort order is accepted by alert timeline endpoints
func (order AlertTimelineSortOrder) Valid() bool {
	switch order {
	case AlertTimelineOrderAsc, AlertTimelineOrderDesc:
		return true
	default:
		return false
	}
}

// The alert timeline serves the same options envelope as the node and node
// group endpoints, so these names are aliases of the shared types in
// options.go rather than separate declarations.
type (
	// AlertTimelineFilterOptions represents the alert timeline filters and sorting choices available to the caller.
	AlertTimelineFilterOptions = FilterOptions
	// AlertTimelineFilters contains the filter fields available for the selected alert timeline view.
	AlertTimelineFilters = Filters
	// AlertTimelineFilterField represents one alert timeline filter and its available values.
	AlertTimelineFilterField = FilterField
	// AlertTimelineFilterOption represents one alert timeline filter value.
	AlertTimelineFilterOption = FilterOption
	// AlertTimelineSortingOptions describes the supported alert timeline sort fields, orders, and defaults.
	AlertTimelineSortingOptions = SortingOptions
	// AlertTimelineSortingDefaults reports the alert timeline sort defaults.
	AlertTimelineSortingDefaults = SortingDefaults
)

// Represents request options for listing alerts
type ListAlertsOptions struct {
	NodeUUID  string
	Component string
	State     AlertState
	Severity  AlertSeverity
	Page      *int
	PageSize  *int
}

// Represents a paginated alert response with the raw backend payload
type AlertsPage struct {
	Alerts         []Alert `json:"alerts"`
	Page           int     `json:"page"`
	PageSize       int     `json:"pageSize"`
	Total          int     `json:"total"`
	PageCursorNext string  `json:"pageCursorNext,omitempty"`
	RawJSON        []byte  `json:"-"`
}

// Represents an alert
type Alert struct {
	UUID                 string `json:"alertUUID"`
	NodeUUID             string `json:"nodeUUID,omitempty"`
	Component            string `json:"component,omitempty"`
	ComponentDisplayName string `json:"componentDisplayName,omitempty"`
	Severity             string `json:"severity,omitempty"`
	State                string `json:"state,omitempty"`
	Message              string `json:"message,omitempty"`
	Error                string `json:"error,omitempty"`
	DetectedAt           string `json:"detectedAt,omitempty"`
	TriggeredAt          string `json:"triggeredAt,omitempty"`
	EventTimestamp       string `json:"eventTimestamp,omitempty"`
	LastUpdated          string `json:"lastUpdated,omitempty"`
	FiredAt              string `json:"firedAt,omitempty"`
}

// Represents request options for listing alert timeline nodes
type ListAlertTimelineNodesOptions struct {
	Active         bool
	Hostname       string
	SortBy         AlertTimelineNodeSortBy
	Order          AlertTimelineSortOrder
	GPUTypes       []string
	NodeGroupIDs   []string
	ComputeZoneIDs []string
	AlertStates    []AlertTimelineState
	ComponentTypes []string
	Page           *int
	PageSize       *int
}

// Represents a paginated alert timeline node response
type AlertTimelineNodesPage struct {
	Nodes                    []AlertTimelineNode `json:"nodes"`
	HasMore                  bool                `json:"hasMore"`
	Page                     int                 `json:"page"`
	PageSize                 int                 `json:"pageSize"`
	Total                    int                 `json:"total"`
	TotalCritical            int                 `json:"totalCritical"`
	TotalWarning             int                 `json:"totalWarning"`
	TotalResolved            int                 `json:"totalResolved"`
	DistinctGPUTypeCount     int                 `json:"distinctGpuTypeCount"`
	DistinctNodeGroupCount   int                 `json:"distinctNodeGroupCount"`
	DistinctComputeZoneCount int                 `json:"distinctComputeZoneCount"`
	RawJSON                  []byte              `json:"-"`
}

// Represents a node that has alert timeline history
type AlertTimelineNode struct {
	NodeUUID      string `json:"nodeUuid"`
	Hostname      string `json:"hostname,omitempty"`
	ComputeZone   string `json:"computeZone,omitempty"`
	NodeGroup     string `json:"nodeGroup,omitempty"`
	GPUType       string `json:"gpuType,omitempty"`
	CriticalCount int    `json:"criticalCount,omitempty"`
	WarningCount  int    `json:"warningCount,omitempty"`
	ResolvedCount int    `json:"resolvedCount,omitempty"`
	HostStatus    string `json:"hostStatus,omitempty"`
	LastAlertTime string `json:"lastAlertTime,omitempty"`
}

// Represents request options for listing alert history for one node
type ListNodeAlertTimelineOptions struct {
	NodeUUID       string
	Active         bool
	WithoutPSIRT   bool
	SortBy         AlertTimelineAlertSortBy
	Order          AlertTimelineSortOrder
	AlertStates    []AlertTimelineState
	ComponentTypes []string
	GPUTypes       []string
	NodeGroupIDs   []string
	ComputeZoneIDs []string
	Page           *int
	PageSize       *int
}

// Represents a paginated alert history response for one node
type NodeAlertTimelinePage struct {
	NodeUUID string                   `json:"nodeUuid"`
	Hostname string                   `json:"hostname,omitempty"`
	Alerts   []AlertTimelineNodeAlert `json:"alerts"`
	HasMore  bool                     `json:"hasMore"`
	Page     int                      `json:"page"`
	PageSize int                      `json:"pageSize"`
	Total    int                      `json:"total"`
	RawJSON  []byte                   `json:"-"`
}

// Represents one alert in a node's timeline history
type AlertTimelineNodeAlert struct {
	AlertUUID            string `json:"alertUuid"`
	Component            string `json:"component,omitempty"`
	ComponentDisplayName string `json:"componentDisplayName,omitempty"`
	AlertStatus          string `json:"alertStatus,omitempty"`
	StartTime            string `json:"startTime,omitempty"`
	LastEventTime        string `json:"lastEventTime,omitempty"`
}

// Represents request options for retrieving one alert's event timeline
type DescribeAlertTimelineOptions struct {
	Order    AlertTimelineSortOrder
	Page     *int
	PageSize *int
}

// Represents the full timeline for one alert
type AlertTimelineDetails struct {
	AlertUUID            string               `json:"alertUuid"`
	NodeUUID             string               `json:"nodeUuid,omitempty"`
	Component            string               `json:"component,omitempty"`
	ComponentDisplayName string               `json:"componentDisplayName,omitempty"`
	AlertStatus          string               `json:"alertStatus,omitempty"`
	NodeGroup            string               `json:"nodeGroup,omitempty"`
	ComputeZone          string               `json:"computeZone,omitempty"`
	CustomerID           string               `json:"customerID,omitempty"`
	IsBackendComponent   bool                 `json:"isBackendComponent,omitempty"`
	HasMore              bool                 `json:"hasMore,omitempty"`
	Page                 int                  `json:"page,omitempty"`
	PageSize             int                  `json:"pageSize,omitempty"`
	Total                int                  `json:"total,omitempty"`
	Timeline             []AlertTimelineEvent `json:"timeline"`
	RawJSON              []byte               `json:"-"`
}

// Represents one event in an alert timeline
type AlertTimelineEvent struct {
	EventType      string            `json:"eventType,omitempty"`
	AlertStatus    string            `json:"alertStatus,omitempty"`
	EventTimestamp string            `json:"eventTimestamp,omitempty"`
	Message        string            `json:"message,omitempty"`
	Error          string            `json:"error,omitempty"`
	ExtraInfo      map[string]any    `json:"extraInfo,omitempty"`
	Incidents      []any             `json:"incidents,omitempty"`
	Actions        []SuggestedAction `json:"suggestedActions,omitempty"`
}

// Represents an operator action attached to an alert event
type SuggestedAction struct {
	Action  string `json:"action,omitempty"`
	Code    string `json:"code,omitempty"`
	Persona string `json:"persona,omitempty"`
	Type    string `json:"type,omitempty"`
}

// Lists alerts using the configured API client
func (c *Client) ListAlerts(ctx context.Context, opts ListAlertsOptions) (AlertsPage, error) {
	ctx, cancel := c.requestContext(ctx)
	defer cancel()

	if err := validateAlertOptions(opts); err != nil {
		return AlertsPage{}, err
	}

	params := fleetapi.GetV1AlertsParams{
		NodeUUID:  optionalString(opts.NodeUUID),
		Component: optionalString(opts.Component),
		State:     optionalEnum[string](opts.State),
		Severity:  optionalEnum[string](opts.Severity),
		Page:      cloneInt(opts.Page),
		PageSize:  cloneInt(opts.PageSize),
	}

	resp, err := c.api.GetV1AlertsWithResponse(ctx, &params)
	if err != nil {
		return AlertsPage{}, err
	}
	if resp.StatusCode() != http.StatusOK {
		return AlertsPage{}, newAPIError(resp.StatusCode(), resp.Status(), resp.Body)
	}

	return decodeAlerts(resp.Body)
}

// Gets alert timeline filter and sorting options for the active or historical view.
func (c *Client) GetAlertTimelineFilterOptions(ctx context.Context, active bool) (AlertTimelineFilterOptions, error) {
	ctx, cancel := c.requestContext(ctx)
	defer cancel()

	params := fleetapi.GetV1AlertTimelineFilterOptionsParams{Active: boolPointer(active)}
	resp, err := c.api.GetV1AlertTimelineFilterOptions(ctx, &params)
	if err != nil {
		return AlertTimelineFilterOptions{}, err
	}
	return decodeFilterOptions(resp, "alert timeline")
}

// Lists nodes with alert timeline history
func (c *Client) ListAlertTimelineNodes(ctx context.Context, opts ListAlertTimelineNodesOptions) (AlertTimelineNodesPage, error) {
	ctx, cancel := c.requestContext(ctx)
	defer cancel()
	if err := validateAlertTimelineNodeOptions(opts); err != nil {
		return AlertTimelineNodesPage{}, err
	}

	params := fleetapi.GetV1AlertTimelineNodesParams{
		Active:         optionalTrueBool(opts.Active),
		Page:           cloneInt(opts.Page),
		PageSize:       cloneInt(opts.PageSize),
		Hostname:       optionalString(opts.Hostname),
		SortBy:         optionalEnum[fleetapi.GetV1AlertTimelineNodesParamsSortBy](opts.SortBy),
		Order:          optionalEnum[fleetapi.GetV1AlertTimelineNodesParamsOrder](opts.Order),
		GpuTypes:       optionalSlice(opts.GPUTypes),
		NodeGroupIds:   optionalSlice(opts.NodeGroupIDs),
		ComputeZoneIds: optionalSlice(opts.ComputeZoneIDs),
		AlertStates:    optionalEnumSlice[fleetapi.GetV1AlertTimelineNodesParamsAlertStates](opts.AlertStates),
		ComponentTypes: optionalSlice(opts.ComponentTypes),
	}

	resp, err := c.api.GetV1AlertTimelineNodesWithResponse(ctx, &params)
	if err != nil {
		return AlertTimelineNodesPage{}, err
	}
	if resp.StatusCode() != http.StatusOK {
		return AlertTimelineNodesPage{}, newAPIError(resp.StatusCode(), resp.Status(), resp.Body)
	}

	return decodeAlertTimelineNodes(resp.Body)
}

// Lists alert timeline history for a specific node
func (c *Client) ListNodeAlertTimeline(ctx context.Context, opts ListNodeAlertTimelineOptions) (NodeAlertTimelinePage, error) {
	ctx, cancel := c.requestContext(ctx)
	defer cancel()

	if opts.NodeUUID == "" {
		return NodeAlertTimelinePage{}, fmt.Errorf("node UUID is required")
	}
	if err := validateNodeAlertTimelineOptions(opts); err != nil {
		return NodeAlertTimelinePage{}, err
	}

	params := fleetapi.GetV1AlertTimelineNodesNodeUuidAlertsParams{
		Active:         optionalTrueBool(opts.Active),
		WithoutPsirt:   optionalTrueBool(opts.WithoutPSIRT),
		SortBy:         optionalEnum[fleetapi.GetV1AlertTimelineNodesNodeUuidAlertsParamsSortBy](opts.SortBy),
		Order:          optionalEnum[fleetapi.GetV1AlertTimelineNodesNodeUuidAlertsParamsOrder](opts.Order),
		AlertStates:    optionalEnumSlice[fleetapi.GetV1AlertTimelineNodesNodeUuidAlertsParamsAlertStates](opts.AlertStates),
		ComponentTypes: optionalSlice(opts.ComponentTypes),
		GpuTypes:       optionalSlice(opts.GPUTypes),
		NodeGroupIds:   optionalSlice(opts.NodeGroupIDs),
		ComputeZoneIds: optionalSlice(opts.ComputeZoneIDs),
		Page:           cloneInt(opts.Page),
		PageSize:       cloneInt(opts.PageSize),
	}

	resp, err := c.api.GetV1AlertTimelineNodesNodeUuidAlertsWithResponse(ctx, opts.NodeUUID, &params)
	if err != nil {
		return NodeAlertTimelinePage{}, err
	}
	if resp.StatusCode() != http.StatusOK {
		return NodeAlertTimelinePage{}, newAPIError(resp.StatusCode(), resp.Status(), resp.Body)
	}

	return decodeNodeAlertTimeline(resp.Body)
}

// Retrieves the full event timeline for one alert
func (c *Client) DescribeAlertTimeline(ctx context.Context, nodeUUID, alertUUID string) (AlertTimelineDetails, error) {
	return c.DescribeAlertTimelineWithOptions(ctx, nodeUUID, alertUUID, DescribeAlertTimelineOptions{})
}

// Retrieves one alert's event timeline with optional sorting and pagination
func (c *Client) DescribeAlertTimelineWithOptions(ctx context.Context, nodeUUID, alertUUID string, opts DescribeAlertTimelineOptions) (AlertTimelineDetails, error) {
	ctx, cancel := c.requestContext(ctx)
	defer cancel()

	if nodeUUID == "" {
		return AlertTimelineDetails{}, fmt.Errorf("node UUID is required")
	}
	if alertUUID == "" {
		return AlertTimelineDetails{}, fmt.Errorf("alert UUID is required")
	}
	if err := validateDescribeAlertTimelineOptions(opts); err != nil {
		return AlertTimelineDetails{}, err
	}

	params := fleetapi.GetV1AlertTimelineNodesNodeUuidAlertsAlertUuidParams{}
	if opts.Order != "" {
		order := fleetapi.GetV1AlertTimelineNodesNodeUuidAlertsAlertUuidParamsOrder(opts.Order)
		params.Order = &order
	}
	if opts.Page != nil {
		params.Page = cloneInt(opts.Page)
	}
	if opts.PageSize != nil {
		params.PageSize = cloneInt(opts.PageSize)
	}

	resp, err := c.api.GetV1AlertTimelineNodesNodeUuidAlertsAlertUuidWithResponse(ctx, nodeUUID, alertUUID, &params)
	if err != nil {
		return AlertTimelineDetails{}, err
	}
	if resp.StatusCode() != http.StatusOK {
		return AlertTimelineDetails{}, newAPIError(resp.StatusCode(), resp.Status(), resp.Body)
	}

	var data fleetapi.ModelsAlertTimelineDetailResponse
	if err := json.Unmarshal(resp.Body, &data); err != nil {
		return AlertTimelineDetails{}, err
	}

	details := alertTimelineDetailsFromGenerated(data)
	details.RawJSON = append([]byte(nil), resp.Body...)
	return details, nil
}

// Checks alert list options before making the request
func validateAlertOptions(opts ListAlertsOptions) error {
	if opts.Severity != "" && !opts.Severity.Valid() {
		return fmt.Errorf("invalid alert severity %q: expected Critical or Warning", opts.Severity)
	}
	if opts.State != "" && !opts.State.Valid() {
		return fmt.Errorf("invalid alert state %q: expected Detected, Triggered, or Resolved", opts.State)
	}
	return nil
}

// Checks level-1 alert timeline options before making the request
func validateAlertTimelineNodeOptions(opts ListAlertTimelineNodesOptions) error {
	if opts.SortBy != "" && !opts.SortBy.Valid() {
		return fmt.Errorf("invalid alert timeline node sort %q", opts.SortBy)
	}
	if opts.Order != "" && !opts.Order.Valid() {
		return fmt.Errorf("invalid alert timeline order %q: expected asc or desc", opts.Order)
	}
	return validateAlertTimelineStates(opts.AlertStates)
}

// Checks level-2 alert timeline options before making the request
func validateNodeAlertTimelineOptions(opts ListNodeAlertTimelineOptions) error {
	if opts.SortBy != "" && !opts.SortBy.Valid() {
		return fmt.Errorf("invalid node alert timeline sort %q", opts.SortBy)
	}
	if opts.Order != "" && !opts.Order.Valid() {
		return fmt.Errorf("invalid alert timeline order %q: expected asc or desc", opts.Order)
	}
	return validateAlertTimelineStates(opts.AlertStates)
}

// Checks level-3 alert timeline options before making the request
func validateDescribeAlertTimelineOptions(opts DescribeAlertTimelineOptions) error {
	if opts.Order != "" && !opts.Order.Valid() {
		return fmt.Errorf("invalid alert timeline order %q: expected asc or desc", opts.Order)
	}
	if opts.Page != nil && opts.PageSize == nil {
		return fmt.Errorf("alert timeline page requires page size")
	}
	if opts.Page != nil && *opts.Page < 0 {
		return fmt.Errorf("alert timeline page must be non-negative")
	}
	if opts.PageSize != nil && (*opts.PageSize < alertTimelineMinPageSize || *opts.PageSize > alertTimelineMaxPageSize) {
		return fmt.Errorf(
			"alert timeline page size must be between %d and %d",
			alertTimelineMinPageSize,
			alertTimelineMaxPageSize,
		)
	}
	return nil
}

// Checks alert states shared by the level-1 and level-2 timeline endpoints
func validateAlertTimelineStates(states []AlertTimelineState) error {
	for _, state := range states {
		if !state.Valid() {
			return fmt.Errorf("invalid alert timeline state %q: expected Critical, Warning, or Resolved", state)
		}
	}
	return nil
}

// Decodes alert responses and preserves the original payload
func decodeAlerts(data []byte) (AlertsPage, error) {
	var resp fleetapi.ModelsAlertResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return AlertsPage{}, err
	}

	page := AlertsPage{
		Page:           intValue(resp.Page),
		PageSize:       intValue(resp.PageSize),
		Total:          intValue(resp.Total),
		PageCursorNext: stringValue(resp.PageCursorNext),
		RawJSON:        append([]byte(nil), data...),
	}
	if resp.Alerts != nil {
		page.Alerts = make([]Alert, 0, len(*resp.Alerts))
		for _, alert := range *resp.Alerts {
			page.Alerts = append(page.Alerts, alertFromGenerated(alert))
		}
	}

	return page, nil
}

// Decodes alert timeline node responses and preserves the original payload
func decodeAlertTimelineNodes(data []byte) (AlertTimelineNodesPage, error) {
	var resp fleetapi.ModelsAlertTimelineNodesResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return AlertTimelineNodesPage{}, err
	}

	page := AlertTimelineNodesPage{
		HasMore:                  boolValue(resp.HasMore),
		Page:                     intValue(resp.Page),
		PageSize:                 intValue(resp.PageSize),
		Total:                    intValue(resp.Total),
		TotalCritical:            intValue(resp.TotalCritical),
		TotalWarning:             intValue(resp.TotalWarning),
		TotalResolved:            intValue(resp.TotalResolved),
		DistinctGPUTypeCount:     intValue(resp.DistinctGpuTypeCount),
		DistinctNodeGroupCount:   intValue(resp.DistinctNodeGroupCount),
		DistinctComputeZoneCount: intValue(resp.DistinctComputeZoneCount),
		RawJSON:                  append([]byte(nil), data...),
	}
	if resp.Nodes != nil {
		page.Nodes = make([]AlertTimelineNode, 0, len(*resp.Nodes))
		for _, node := range *resp.Nodes {
			page.Nodes = append(page.Nodes, alertTimelineNodeFromGenerated(node))
		}
	}

	return page, nil
}

// Decodes node alert timeline responses and preserves the original payload
func decodeNodeAlertTimeline(data []byte) (NodeAlertTimelinePage, error) {
	var resp fleetapi.ModelsAlertTimelineNodeAlertsResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return NodeAlertTimelinePage{}, err
	}

	page := NodeAlertTimelinePage{
		NodeUUID: stringValue(resp.NodeUuid),
		Hostname: stringValue(resp.Hostname),
		HasMore:  boolValue(resp.HasMore),
		Page:     intValue(resp.Page),
		PageSize: intValue(resp.PageSize),
		Total:    intValue(resp.Total),
		RawJSON:  append([]byte(nil), data...),
	}
	if resp.Alerts != nil {
		page.Alerts = make([]AlertTimelineNodeAlert, 0, len(*resp.Alerts))
		for _, alert := range *resp.Alerts {
			page.Alerts = append(page.Alerts, alertTimelineNodeAlertFromGenerated(alert))
		}
	}

	return page, nil
}

// Maps alert API models into SDK values
func alertFromGenerated(alert fleetapi.ModelsAlert) Alert {
	detectedAt := stringValue(alert.DetectedAt)
	triggeredAt := stringValue(alert.TriggeredAt)
	eventTimestamp := stringValue(alert.EventTimestamp)
	return Alert{
		UUID:                 stringValue(alert.AlertUUID),
		NodeUUID:             stringValue(alert.NodeUUID),
		Component:            stringValue(alert.Component),
		ComponentDisplayName: stringValue(alert.ComponentDisplayName),
		Severity:             enumStringValue(alert.Severity),
		State:                enumStringValue(alert.State),
		Message:              stringValue(alert.Message),
		Error:                stringValue(alert.Error),
		DetectedAt:           detectedAt,
		TriggeredAt:          triggeredAt,
		EventTimestamp:       eventTimestamp,
		LastUpdated:          stringValue(alert.LastUpdated),
		FiredAt:              firstNonEmpty(triggeredAt, detectedAt, eventTimestamp),
	}
}

// Maps timeline node API models into SDK values
func alertTimelineNodeFromGenerated(node fleetapi.ModelsAlertTimelineNode) AlertTimelineNode {
	return AlertTimelineNode{
		NodeUUID:      stringValue(node.NodeUuid),
		Hostname:      stringValue(node.Hostname),
		ComputeZone:   stringValue(node.ComputeZone),
		NodeGroup:     stringValue(node.NodeGroup),
		GPUType:       stringValue(node.GpuType),
		CriticalCount: intValue(node.CriticalCount),
		WarningCount:  intValue(node.WarningCount),
		ResolvedCount: intValue(node.ResolvedCount),
		HostStatus:    stringValue(node.HostStatus),
		LastAlertTime: stringValue(node.LastAlertTime),
	}
}

// Maps timeline alert API models into SDK values
func alertTimelineNodeAlertFromGenerated(alert fleetapi.ModelsAlertTimelineNodeAlert) AlertTimelineNodeAlert {
	return AlertTimelineNodeAlert{
		AlertUUID:            stringValue(alert.AlertUuid),
		Component:            stringValue(alert.Component),
		ComponentDisplayName: stringValue(alert.ComponentDisplayName),
		AlertStatus:          stringValue(alert.AlertStatus),
		StartTime:            stringValue(alert.StartTime),
		LastEventTime:        stringValue(alert.LastEventTime),
	}
}

// Maps timeline detail API models into SDK values
func alertTimelineDetailsFromGenerated(details fleetapi.ModelsAlertTimelineDetailResponse) AlertTimelineDetails {
	out := AlertTimelineDetails{
		AlertUUID:            stringValue(details.AlertUuid),
		NodeUUID:             stringValue(details.NodeUuid),
		Component:            stringValue(details.Component),
		ComponentDisplayName: stringValue(details.ComponentDisplayName),
		AlertStatus:          stringValue(details.AlertStatus),
		NodeGroup:            stringValue(details.NodeGroup),
		ComputeZone:          stringValue(details.ComputeZone),
		CustomerID:           stringValue(details.CustomerID),
		IsBackendComponent:   boolValue(details.IsBackendComponent),
		HasMore:              boolValue(details.HasMore),
		Page:                 intValue(details.Page),
		PageSize:             intValue(details.PageSize),
		Total:                intValue(details.Total),
	}
	if details.Timeline != nil {
		out.Timeline = make([]AlertTimelineEvent, 0, len(*details.Timeline))
		for _, event := range *details.Timeline {
			out.Timeline = append(out.Timeline, alertTimelineEventFromGenerated(event))
		}
	}
	return out
}

// Maps timeline event API models into SDK values
func alertTimelineEventFromGenerated(event fleetapi.ModelsAlertTimelineEvent) AlertTimelineEvent {
	return AlertTimelineEvent{
		EventType:      enumStringValue(event.EventType),
		AlertStatus:    stringValue(event.AlertStatus),
		EventTimestamp: stringValue(event.EventTimestamp),
		Message:        stringValue(event.Message),
		Error:          stringValue(event.Error),
		ExtraInfo:      mapValue(event.ExtraInfo),
		Incidents:      interfaceSliceValue(event.Incidents),
		Actions:        suggestedActionsFromGenerated(event.SuggestedActions),
	}
}

// Maps suggested action API models into SDK values
func suggestedActionsFromGenerated(actions *[]fleetapi.ModelsSuggestedAction) []SuggestedAction {
	if actions == nil {
		return nil
	}
	out := make([]SuggestedAction, 0, len(*actions))
	for _, action := range *actions {
		out = append(out, SuggestedAction{
			Action:  stringValue(action.Action),
			Code:    stringValue(action.Code),
			Persona: enumStringValue(action.Persona),
			Type:    enumStringValue(action.Type),
		})
	}
	return out
}

// Copies optional maps without sharing backing storage
func mapValue(value *map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	out := make(map[string]any, len(*value))
	for key, item := range *value {
		out[key] = item
	}
	return out
}

// Copies optional untyped slices without sharing backing storage
func interfaceSliceValue(value *[]any) []any {
	if value == nil {
		return nil
	}
	return append([]any(nil), (*value)...)
}

// Returns the first non-empty value in order
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
