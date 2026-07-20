// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package fleetintelligence

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/NVIDIA/fleet-intelligence-client/internal/generated/fleetapi"
)

const (
	AlertSeverityCritical AlertSeverity = "Critical"
	AlertSeverityWarning  AlertSeverity = "Warning"

	AlertStateDetected  AlertState = "Detected"
	AlertStateTriggered AlertState = "Triggered"
	AlertStateResolved  AlertState = "Resolved"
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
	Active   bool
	Page     *int
	PageSize *int
}

// Represents a paginated alert timeline node response
type AlertTimelineNodesPage struct {
	Nodes    []AlertTimelineNode `json:"nodes"`
	HasMore  bool                `json:"hasMore"`
	Page     int                 `json:"page"`
	PageSize int                 `json:"pageSize"`
	Total    int                 `json:"total"`
	RawJSON  []byte              `json:"-"`
}

// Represents a node that has alert timeline history
type AlertTimelineNode struct {
	NodeUUID      string `json:"nodeUuid"`
	Hostname      string `json:"hostname,omitempty"`
	HostStatus    string `json:"hostStatus,omitempty"`
	LastAlertTime string `json:"lastAlertTime,omitempty"`
}

// Represents request options for listing alert history for one node
type ListNodeAlertTimelineOptions struct {
	NodeUUID string
	Active   bool
	Page     *int
	PageSize *int
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
	LastEventTime        string `json:"lastEventTime,omitempty"`
}

// Represents the full timeline for one alert
type AlertTimelineDetails struct {
	AlertUUID            string               `json:"alertUuid"`
	NodeUUID             string               `json:"nodeUuid,omitempty"`
	Component            string               `json:"component,omitempty"`
	ComponentDisplayName string               `json:"componentDisplayName,omitempty"`
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

	params := fleetapi.GetV1AlertsParams{}
	if opts.NodeUUID != "" {
		params.NodeUUID = &opts.NodeUUID
	}
	if opts.Component != "" {
		params.Component = &opts.Component
	}
	if opts.State != "" {
		state := string(opts.State)
		params.State = &state
	}
	if opts.Severity != "" {
		severity := string(opts.Severity)
		params.Severity = &severity
	}
	if opts.Page != nil {
		// /v1/alerts is the only Fleet Intelligence list endpoint that numbers
		// pages from 1; shift the caller's 0-indexed page to match it.
		apiPage := *opts.Page + alertsAPIPageOffset
		params.Page = &apiPage
	}
	if opts.PageSize != nil {
		params.PageSize = cloneInt(opts.PageSize)
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

// Lists nodes with alert timeline history
func (c *Client) ListAlertTimelineNodes(ctx context.Context, opts ListAlertTimelineNodesOptions) (AlertTimelineNodesPage, error) {
	ctx, cancel := c.requestContext(ctx)
	defer cancel()

	params := fleetapi.GetV1AlertTimelineNodesParams{}
	if opts.Active {
		params.Active = boolPointer(opts.Active)
	}
	if opts.Page != nil {
		params.Page = cloneInt(opts.Page)
	}
	if opts.PageSize != nil {
		params.PageSize = cloneInt(opts.PageSize)
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

	params := fleetapi.GetV1AlertTimelineNodesNodeUuidAlertsParams{}
	if opts.Active {
		params.Active = boolPointer(opts.Active)
	}
	if opts.Page != nil {
		params.Page = cloneInt(opts.Page)
	}
	if opts.PageSize != nil {
		params.PageSize = cloneInt(opts.PageSize)
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
	ctx, cancel := c.requestContext(ctx)
	defer cancel()

	if nodeUUID == "" {
		return AlertTimelineDetails{}, fmt.Errorf("node UUID is required")
	}
	if alertUUID == "" {
		return AlertTimelineDetails{}, fmt.Errorf("alert UUID is required")
	}

	resp, err := c.api.GetV1AlertTimelineNodesNodeUuidAlertsAlertUuidWithResponse(ctx, nodeUUID, alertUUID)
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

// alertsAPIPageOffset bridges the /v1/alerts endpoint's 1-indexed paging to the
// 0-indexed page numbers this SDK exposes for every list call. /v1/alerts is the
// only Fleet Intelligence list endpoint that starts at page 1 (see
// api/openapi/openapi.yaml); ListAlerts shifts the page number by this offset on
// the way out and rewrites it on the way back so callers never see the quirk.
const alertsAPIPageOffset = 1

// Decodes alert responses and preserves the original payload
func decodeAlerts(data []byte) (AlertsPage, error) {
	var resp fleetapi.ModelsAlertResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return AlertsPage{}, err
	}

	page := AlertsPage{
		Page:           zeroIndexedAlertsPage(intValue(resp.Page)),
		PageSize:       intValue(resp.PageSize),
		Total:          intValue(resp.Total),
		PageCursorNext: stringValue(resp.PageCursorNext),
		RawJSON:        normalizeRawAlertsPage(data),
	}
	if resp.Alerts != nil {
		page.Alerts = make([]Alert, 0, len(*resp.Alerts))
		for _, alert := range *resp.Alerts {
			page.Alerts = append(page.Alerts, alertFromGenerated(alert))
		}
	}

	return page, nil
}

// zeroIndexedAlertsPage converts a 1-indexed /v1/alerts page number to the
// 0-indexed value the SDK exposes, guarding against an unexpected non-positive
// page from the backend.
func zeroIndexedAlertsPage(apiPage int) int {
	if apiPage <= 0 {
		return 0
	}
	return apiPage - alertsAPIPageOffset
}

// normalizeRawAlertsPage rewrites the 1-indexed "page" field in a raw /v1/alerts
// payload to its 0-indexed equivalent so JSON consumers see the same paging
// contract as every other list command. The original bytes are returned
// unchanged when the payload has no usable page field.
func normalizeRawAlertsPage(data []byte) []byte {
	var body map[string]json.RawMessage
	if err := json.Unmarshal(data, &body); err != nil {
		return append([]byte(nil), data...)
	}
	raw, ok := body["page"]
	if !ok {
		return append([]byte(nil), data...)
	}
	var apiPage int
	if err := json.Unmarshal(raw, &apiPage); err != nil {
		return append([]byte(nil), data...)
	}
	normalized, err := json.Marshal(zeroIndexedAlertsPage(apiPage))
	if err != nil {
		return append([]byte(nil), data...)
	}
	body["page"] = normalized
	out, err := json.Marshal(body)
	if err != nil {
		return append([]byte(nil), data...)
	}
	return out
}

// Decodes alert timeline node responses and preserves the original payload
func decodeAlertTimelineNodes(data []byte) (AlertTimelineNodesPage, error) {
	var resp fleetapi.ModelsAlertTimelineNodesResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return AlertTimelineNodesPage{}, err
	}

	page := AlertTimelineNodesPage{
		HasMore:  boolValue(resp.HasMore),
		Page:     intValue(resp.Page),
		PageSize: intValue(resp.PageSize),
		Total:    intValue(resp.Total),
		RawJSON:  append([]byte(nil), data...),
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

// Returns the first non-empty value in order
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
