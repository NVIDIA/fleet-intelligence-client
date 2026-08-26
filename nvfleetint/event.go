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

// EventListOptions represents request options for listing events. A time range
// is required: supply either Window (relative) or StartTime and EndTime
// (absolute), but not both.
type EventListOptions struct {
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

// Event represents a single fleet event
type Event struct {
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

// ListEvents retrieves events filtered by node, component, and time range
func (c *Client) ListEvents(ctx context.Context, opts EventListOptions) (EventsPage, error) {
	ctx, cancel := c.requestContext(ctx)
	defer cancel()

	timeRange, err := normalizeEventTimeRange(opts.Window, opts.StartTime, opts.EndTime)
	if err != nil {
		return EventsPage{}, err
	}

	mode := fleetapi.GetV1EventsParamsTimeMode(timeRange.timeMode)
	params := fleetapi.GetV1EventsParams{
		TimeMode:  &mode,
		Window:    optionalString(timeRange.window),
		StartTime: optionalString(timeRange.startTime),
		EndTime:   optionalString(timeRange.endTime),
		NodeUUID:  optionalTrimmedString(opts.NodeUUID),
		Component: optionalTrimmedString(opts.Component),
		Page:      cloneInt(opts.Page),
		PageSize:  cloneInt(opts.PageSize),
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

	timeRange, err := normalizeEventTimeRange(opts.Window, opts.StartTime, opts.EndTime)
	if err != nil {
		return EventBuckets{}, err
	}
	if opts.MaxBuckets != nil && (*opts.MaxBuckets < 1 || *opts.MaxBuckets > MaxEventBuckets) {
		return EventBuckets{}, fmt.Errorf("max buckets must be between 1 and %d", MaxEventBuckets)
	}

	mode := fleetapi.GetV1EventsBucketsParamsTimeMode(timeRange.timeMode)
	params := fleetapi.GetV1EventsBucketsParams{TimeMode: &mode}
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
		EventID:   stringValue(event.EventId),
		NodeUUID:  stringValue(event.NodeUUID),
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
