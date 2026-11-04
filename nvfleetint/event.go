// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/NVIDIA/fleet-intelligence-client/internal/generated/fleetapi"
)

// MaxEventBuckets is the largest number of histogram buckets the events buckets
// endpoint accepts.
const MaxEventBuckets = 1000

const (
	eventTimeModeAbsolute = "absolute"
	eventTimeModeRelative = "relative"
)

// EventSortBy selects the sort field for event listings.
type EventSortBy string

// Valid reports whether the API supports the sort field.
func (value EventSortBy) Valid() bool {
	return fleetapi.GetV1EventsParamsSortBy(value).Valid()
}

// EventSortOrder selects ascending or descending event order.
type EventSortOrder string

// Valid reports whether the API supports the sort direction.
func (value EventSortOrder) Valid() bool {
	return fleetapi.GetV1EventsParamsSortOrder(value).Valid()
}

const (
	EventSortByTimestamp   EventSortBy    = "timestamp"
	EventSortByEventName   EventSortBy    = "eventName"
	EventSortBySeverity    EventSortBy    = "severity"
	EventSortByComponent   EventSortBy    = "component"
	EventSortByGPUType     EventSortBy    = "gpuType"
	EventSortByHostname    EventSortBy    = "hostname"
	EventSortByNodeName    EventSortBy    = "nodeName"
	EventSortByBMCMAC      EventSortBy    = "bmcMac"
	EventSortByNodeUUID    EventSortBy    = "nodeUUID"
	EventSortByNodeGroup   EventSortBy    = "nodeGroup"
	EventSortByComputeZone EventSortBy    = "computeZone"
	EventOrderAsc          EventSortOrder = "asc"
	EventOrderDesc         EventSortOrder = "desc"
)

// EventListOptions represents request options for listing events. A time range
// is required: supply either Window (relative) or StartTime and EndTime
// (absolute), but not both.
type EventListOptions struct {
	// Values within a slice are ORed; different filters are ANDed.
	// Omitted DataSources includes both event-producing sources.
	// Component and Components cannot be combined, nor can inclusion and
	// exclusion filters for the same node-group or compute-zone dimension.
	NodeGroupIDs          []string
	ComputeZoneIDs        []string
	ExcludeNodeGroupIDs   []string
	ExcludeComputeZoneIDs []string
	NameSearch            string
	EventNames            []string
	Severities            []string
	Components            []string
	DataSources           []DataSource
	GPUTypes              []string
	SortBy                EventSortBy
	SortOrder             EventSortOrder

	NodeUUID  string
	Component string
	Window    string
	StartTime string
	EndTime   string
	Page      *int
	PageSize  *int
}

// EventBucketsOptions represents request options for time-bucketed event counts.
// A time range is required, following the same rules as EventListOptions.
type EventBucketsOptions struct {
	// Values within a slice are ORed; different filters are ANDed.
	// Omitted DataSources includes both event-producing sources.
	// Component and Components cannot be combined, nor can inclusion and
	// exclusion filters for the same node-group or compute-zone dimension.
	NodeGroupIDs          []string
	ComputeZoneIDs        []string
	ExcludeNodeGroupIDs   []string
	ExcludeComputeZoneIDs []string
	NameSearch            string
	EventNames            []string
	Severities            []string
	Components            []string
	DataSources           []DataSource
	GPUTypes              []string

	NodeUUID   string
	Component  string
	Window     string
	StartTime  string
	EndTime    string
	MaxBuckets *int
}

// EventsPage represents a paginated event list with the raw backend payload
type EventsPage struct {
	Events   []Event `json:"events"`
	HasMore  bool    `json:"hasMore"`
	Page     int     `json:"page"`
	PageSize int     `json:"pageSize"`
	Total    int     `json:"total"`
	RawJSON  []byte  `json:"-"`
}

// PageInfo reports the pagination envelope of the response.
func (page EventsPage) PageInfo() PageInfo {
	hasMore := page.HasMore
	return PageInfo{
		Page:     page.Page,
		PageSize: page.PageSize,
		Total:    page.Total,
		HasMore:  &hasMore,
		RawJSON:  page.RawJSON,
	}
}

// Event represents a single fleet event. Identity, membership, and GPU type
// reflect current node inventory, independently of the event-producing source.
type Event struct {
	Hostname      string `json:"hostname,omitempty"`
	NodeName      string `json:"nodeName,omitempty"`
	BMCMAC        string `json:"bmcMac,omitempty"`
	DataSource    string `json:"dataSource,omitempty"`
	GPUType       string `json:"gpuType,omitempty"`
	NodeGroup     string `json:"nodeGroup,omitempty"`
	NodeGroupID   string `json:"nodeGroupId,omitempty"`
	ComputeZone   string `json:"computeZone,omitempty"`
	ComputeZoneID string `json:"computeZoneId,omitempty"`

	EventID          string            `json:"eventId,omitempty"`
	NodeUUID         string            `json:"nodeUUID,omitempty"`
	Component        string            `json:"component,omitempty"`
	Name             string            `json:"name,omitempty"`
	Type             string            `json:"type,omitempty"`
	Message          string            `json:"message,omitempty"`
	Timestamp        string            `json:"timestamp,omitempty"`
	CreatedAt        string            `json:"createdAt,omitempty"`
	SuggestedActions []SuggestedAction `json:"suggestedActions,omitempty"`
}

// EventBuckets represents time-bucketed event counts with the raw backend payload
type EventBuckets struct {
	BucketInterval string        `json:"bucketInterval,omitempty"`
	Buckets        []EventBucket `json:"buckets,omitempty"`
	RawJSON        []byte        `json:"-"`
}

// EventBucket represents the event count for a single time interval
type EventBucket struct {
	StartTime      string `json:"startTime,omitempty"`
	EndTime        string `json:"endTime,omitempty"`
	FirstEventTime string `json:"firstEventTime,omitempty"`
	Count          *int   `json:"count,omitempty"`
}

// ListEvents retrieves events filtered by source, inventory, event fields, and time range.
func (c *Client) ListEvents(ctx context.Context, opts EventListOptions) (EventsPage, error) {
	ctx, cancel := c.requestContext(ctx)
	defer cancel()

	if err := opts.Validate(); err != nil {
		return EventsPage{}, err
	}

	timeRange, err := normalizeEventTimeRange(opts.Window, opts.StartTime, opts.EndTime)
	if err != nil {
		return EventsPage{}, err
	}

	mode := fleetapi.GetV1EventsParamsTimeMode(timeRange.timeMode)
	params := fleetapi.GetV1EventsParams{
		NodeGroupIds:          optionalSlice(opts.NodeGroupIDs),
		ComputeZoneIds:        optionalSlice(opts.ComputeZoneIDs),
		ExcludeNodeGroupIds:   optionalSlice(opts.ExcludeNodeGroupIDs),
		ExcludeComputeZoneIds: optionalSlice(opts.ExcludeComputeZoneIDs),
		NameSearch:            optionalTrimmedString(opts.NameSearch),
		EventNames:            optionalSlice(opts.EventNames),
		Severities:            optionalSlice(opts.Severities),
		Components:            optionalSlice(opts.Components),
		DataSources:           optionalEnumSlice[fleetapi.GetV1EventsParamsDataSources](opts.DataSources),
		GpuTypes:              optionalSlice(opts.GPUTypes),
		SortBy:                optionalEnum[fleetapi.GetV1EventsParamsSortBy](opts.SortBy),
		SortOrder:             optionalEnum[fleetapi.GetV1EventsParamsSortOrder](opts.SortOrder),
		TimeMode:              &mode,
		Window:                optionalString(timeRange.window),
		StartTime:             optionalString(timeRange.startTime),
		EndTime:               optionalString(timeRange.endTime),
		NodeUUID:              optionalTrimmedString(opts.NodeUUID),
		Component:             optionalTrimmedString(opts.Component),
		Page:                  cloneInt(opts.Page),
		PageSize:              cloneInt(opts.PageSize),
	}

	resp, err := c.api.GetV1EventsWithResponse(ctx, &params)
	if err != nil {
		return EventsPage{}, err
	}
	if resp.StatusCode() != http.StatusOK {
		return EventsPage{}, newAPIError(resp.StatusCode(), resp.Status(), resp.Body)
	}

	var data fleetapi.ModelsEventsResponse
	if err := json.Unmarshal(resp.Body, &data); err != nil {
		return EventsPage{}, err
	}

	page := EventsPage{
		HasMore:  boolValue(data.HasMore),
		Page:     intValue(data.Page),
		PageSize: intValue(data.PageSize),
		Total:    intValue(data.Total),
		RawJSON:  append([]byte(nil), resp.Body...),
	}
	if data.Events != nil {
		page.Events = make([]Event, 0, len(*data.Events))
		for _, event := range *data.Events {
			page.Events = append(page.Events, eventFromGenerated(event))
		}
	}
	return page, nil
}

// GetEventBuckets retrieves time-bucketed event counts for histogram display
func (c *Client) GetEventBuckets(ctx context.Context, opts EventBucketsOptions) (EventBuckets, error) {
	ctx, cancel := c.requestContext(ctx)
	defer cancel()

	if err := opts.Validate(); err != nil {
		return EventBuckets{}, err
	}

	timeRange, err := normalizeEventTimeRange(opts.Window, opts.StartTime, opts.EndTime)
	if err != nil {
		return EventBuckets{}, err
	}
	if opts.MaxBuckets != nil && (*opts.MaxBuckets < 1 || *opts.MaxBuckets > MaxEventBuckets) {
		return EventBuckets{}, fmt.Errorf("max buckets must be between 1 and %d", MaxEventBuckets)
	}

	mode := fleetapi.GetV1EventsBucketsParamsTimeMode(timeRange.timeMode)
	params := fleetapi.GetV1EventsBucketsParams{
		TimeMode:              &mode,
		NodeGroupIds:          optionalSlice(opts.NodeGroupIDs),
		ComputeZoneIds:        optionalSlice(opts.ComputeZoneIDs),
		ExcludeNodeGroupIds:   optionalSlice(opts.ExcludeNodeGroupIDs),
		ExcludeComputeZoneIds: optionalSlice(opts.ExcludeComputeZoneIDs),
		NameSearch:            optionalTrimmedString(opts.NameSearch),
		EventNames:            optionalSlice(opts.EventNames),
		Severities:            optionalSlice(opts.Severities),
		Components:            optionalSlice(opts.Components),
		DataSources:           optionalEnumSlice[fleetapi.GetV1EventsBucketsParamsDataSources](opts.DataSources),
		GpuTypes:              optionalSlice(opts.GPUTypes),
	}
	if timeRange.window != "" {
		value := timeRange.window
		params.Window = &value
	}
	if timeRange.startTime != "" {
		value := timeRange.startTime
		params.StartTime = &value
	}
	if timeRange.endTime != "" {
		value := timeRange.endTime
		params.EndTime = &value
	}
	if node := strings.TrimSpace(opts.NodeUUID); node != "" {
		params.NodeUUID = &node
	}
	if component := strings.TrimSpace(opts.Component); component != "" {
		params.Component = &component
	}
	if opts.MaxBuckets != nil {
		params.MaxBuckets = cloneInt(opts.MaxBuckets)
	}

	resp, err := c.api.GetV1EventsBucketsWithResponse(ctx, &params)
	if err != nil {
		return EventBuckets{}, err
	}
	if resp.StatusCode() != http.StatusOK {
		return EventBuckets{}, newAPIError(resp.StatusCode(), resp.Status(), resp.Body)
	}

	var data fleetapi.ModelsEventBucketsResponse
	if err := json.Unmarshal(resp.Body, &data); err != nil {
		return EventBuckets{}, err
	}

	buckets := EventBuckets{
		BucketInterval: stringValue(data.BucketInterval),
		RawJSON:        append([]byte(nil), resp.Body...),
	}
	if data.Buckets != nil {
		buckets.Buckets = make([]EventBucket, 0, len(*data.Buckets))
		for _, bucket := range *data.Buckets {
			buckets.Buckets = append(buckets.Buckets, eventBucketFromGenerated(bucket))
		}
	}
	return buckets, nil
}

// Holds a resolved and validated events time range
type eventTimeRange struct {
	timeMode  string
	window    string
	startTime string
	endTime   string
}

// Infers and validates the time range shared by the events endpoints. A window
// selects relative mode; start and end select absolute mode. The two are
// mutually exclusive and at least one must be supplied.
func normalizeEventTimeRange(window, startTime, endTime string) (eventTimeRange, error) {
	window = strings.TrimSpace(window)
	startTime = strings.TrimSpace(startTime)
	endTime = strings.TrimSpace(endTime)

	hasWindow := window != ""
	hasStart := startTime != ""
	hasEnd := endTime != ""

	switch {
	case !hasWindow && !hasStart && !hasEnd:
		return eventTimeRange{}, fmt.Errorf("a time range is required: provide window, or start time and end time")
	case hasWindow && (hasStart || hasEnd):
		return eventTimeRange{}, fmt.Errorf("window cannot be combined with start time or end time")
	case hasWindow:
		if err := ValidateWindow(window); err != nil {
			return eventTimeRange{}, err
		}
		return eventTimeRange{timeMode: eventTimeModeRelative, window: window}, nil
	case !hasStart || !hasEnd:
		return eventTimeRange{}, fmt.Errorf("start time and end time must be provided together")
	default:
		if err := validateEventRFC3339("start time", startTime); err != nil {
			return eventTimeRange{}, err
		}
		if err := validateEventRFC3339("end time", endTime); err != nil {
			return eventTimeRange{}, err
		}
		return eventTimeRange{timeMode: eventTimeModeAbsolute, startTime: startTime, endTime: endTime}, nil
	}
}

// Checks an events timestamp value is RFC3339
func validateEventRFC3339(name, value string) error {
	if _, err := time.Parse(time.RFC3339, value); err != nil {
		return fmt.Errorf("event %s must be RFC3339", name)
	}
	return nil
}

// Maps an event API model into SDK values
func eventFromGenerated(event fleetapi.ModelsEvent) Event {
	out := Event{
		Hostname:      stringValue(event.Hostname),
		NodeName:      stringValue(event.NodeName),
		BMCMAC:        stringValue(event.BmcMac),
		DataSource:    stringValue(event.DataSource),
		GPUType:       stringValue(event.GpuType),
		NodeGroup:     stringValue(event.NodeGroup),
		NodeGroupID:   stringValue(event.NodeGroupId),
		ComputeZone:   stringValue(event.ComputeZone),
		ComputeZoneID: stringValue(event.ComputeZoneId),

		EventID:   event.EventId,
		NodeUUID:  event.NodeUUID,
		Component: stringValue(event.Component),
		Name:      stringValue(event.Name),
		Type:      stringValue(event.Type),
		Message:   stringValue(event.Message),
		Timestamp: stringValue(event.Timestamp),
		CreatedAt: stringValue(event.CreatedAt),
	}
	if event.SuggestedActions != nil {
		out.SuggestedActions = make([]SuggestedAction, 0, len(*event.SuggestedActions))
		for _, action := range *event.SuggestedActions {
			if mapped := suggestedActionFromGenerated(&action); mapped != nil {
				out.SuggestedActions = append(out.SuggestedActions, *mapped)
			}
		}
	}
	return out
}

// Maps an event bucket API model into SDK values
func eventBucketFromGenerated(bucket fleetapi.ModelsEventBucket) EventBucket {
	return EventBucket{
		StartTime:      stringValue(bucket.StartTime),
		EndTime:        stringValue(bucket.EndTime),
		FirstEventTime: stringValue(bucket.FirstEventTime),
		Count:          cloneInt(bucket.Count),
	}
}

// Validate checks filters before a request. Time ranges are checked by ListEvents.
func (opts EventListOptions) Validate() error {
	if err := validateDataSources(opts.DataSources); err != nil {
		return err
	}
	if strings.TrimSpace(opts.Component) != "" && len(opts.Components) > 0 {
		return fmt.Errorf("component and components filters cannot be combined")
	}
	if len(opts.NodeGroupIDs) > 0 && len(opts.ExcludeNodeGroupIDs) > 0 {
		return fmt.Errorf("node group include and exclude filters cannot be combined")
	}
	if len(opts.ComputeZoneIDs) > 0 && len(opts.ExcludeComputeZoneIDs) > 0 {
		return fmt.Errorf("compute zone include and exclude filters cannot be combined")
	}
	if opts.SortBy != "" && !opts.SortBy.Valid() {
		return invalidOption("sortBy", "event sort field", string(opts.SortBy), "")
	}
	if opts.SortOrder != "" && !opts.SortOrder.Valid() {
		return invalidOption("sortOrder", "event sort order", string(opts.SortOrder), "asc or desc")
	}
	return nil
}

// Validate checks shared filters before a bucket request.
func (opts EventBucketsOptions) Validate() error {
	return (EventListOptions{
		Component: opts.Component, Components: opts.Components, DataSources: opts.DataSources,
		NodeGroupIDs: opts.NodeGroupIDs, ExcludeNodeGroupIDs: opts.ExcludeNodeGroupIDs,
		ComputeZoneIDs: opts.ComputeZoneIDs, ExcludeComputeZoneIDs: opts.ExcludeComputeZoneIDs,
	}).Validate()
}

// EventComponentOption pairs a raw filter value with its display name.
type EventComponentOption struct {
	Component            string `json:"component"`
	ComponentDisplayName string `json:"componentDisplayName"`
}

// EventFilterOptions contains catalog values, not counts or time-scoped results.
type EventFilterOptions struct {
	Components  []EventComponentOption `json:"components,omitempty"`
	DataSources []string               `json:"dataSources,omitempty"`
	EventNames  []string               `json:"eventNames,omitempty"`
	GPUTypes    []string               `json:"gpuTypes,omitempty"`
	Severities  []string               `json:"severities,omitempty"`
	SortFields  []string               `json:"sortFields,omitempty"`
	RawJSON     []byte                 `json:"-"`
}

// GetEventFilterOptions discovers supported event filters and list sort fields.
// Options are independent of time/inventory filters. Others matches uncatalogued
// event names; GPU types reflect current resolved node inventory.
func (c *Client) GetEventFilterOptions(ctx context.Context) (EventFilterOptions, error) {
	ctx, cancel := c.requestContext(ctx)
	defer cancel()
	resp, err := c.api.GetV1EventsOptionsWithResponse(ctx)
	if err != nil {
		return EventFilterOptions{}, err
	}
	if resp.StatusCode() != http.StatusOK {
		return EventFilterOptions{}, newAPIError(resp.StatusCode(), resp.Status(), resp.Body)
	}
	var options EventFilterOptions
	if err := json.Unmarshal(resp.Body, &options); err != nil {
		return EventFilterOptions{}, err
	}
	options.RawJSON = append([]byte(nil), resp.Body...)
	return options, nil
}
