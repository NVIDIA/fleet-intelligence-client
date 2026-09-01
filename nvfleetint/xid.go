// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/NVIDIA/fleet-intelligence-client/internal/generated/fleetapi"
)

const (
	XIDBurstSortByJobDisruption                   XIDBurstSortBy = "jobDisruption"
	XIDBurstSortByJobDisruptionDueToPlatformIssue XIDBurstSortBy = "jobDisruptionDueToPlatformIssue"
	XIDBurstSortByCategory                        XIDBurstSortBy = "category"
	XIDBurstSortBySubcategory                     XIDBurstSortBy = "subcategory"
	XIDBurstSortByXIDNumbers                      XIDBurstSortBy = "xidNumbers"
	XIDBurstSortByXIDCount                        XIDBurstSortBy = "xidCount"
	XIDBurstSortByBurstDuration                   XIDBurstSortBy = "burstDurationSeconds"
	// XIDBurstSortByNodeUUID spells the value "nodeUuid" to match the API contract,
	// which differs from the nodeUUID casing used elsewhere in the spec.
	XIDBurstSortByNodeUUID             XIDBurstSortBy = "nodeUuid"
	XIDBurstSortByHostname             XIDBurstSortBy = "hostname"
	XIDBurstSortByNodeGroup            XIDBurstSortBy = "nodeGroup"
	XIDBurstSortByComputeZone          XIDBurstSortBy = "computeZone"
	XIDBurstSortByStartTime            XIDBurstSortBy = "startTime"
	XIDBurstSortByDCAdminAction        XIDBurstSortBy = "dcAdminAction"
	XIDBurstSortByDCAdminInvestigation XIDBurstSortBy = "dcAdminInvestigation"
	XIDBurstSortByTenantAction         XIDBurstSortBy = "tenantAction"
	XIDBurstSortByTenantInvestigation  XIDBurstSortBy = "tenantInvestigation"

	XIDBurstOrderAsc  XIDBurstSortOrder = "asc"
	XIDBurstOrderDesc XIDBurstSortOrder = "desc"
)

// XIDBurstSortBy represents a sortable XID burst column
type XIDBurstSortBy string

// Valid reports whether the sort field is accepted by the API
func (sortBy XIDBurstSortBy) Valid() bool {
	return fleetapi.GetV1XIDBurstsParamsSortBy(sortBy).Valid()
}

// XIDBurstSortOrder represents a sort direction for XID burst listings
type XIDBurstSortOrder string

// Valid reports whether the sort order is accepted by the API
func (order XIDBurstSortOrder) Valid() bool {
	return fleetapi.GetV1XIDBurstsParamsSortOrder(order).Valid()
}

// ListXIDBurstsOptions represents request options for listing finalized XID
// bursts. A time range is required: supply either Window (relative) or
// StartTime and EndTime (absolute), but not both. Values within each slice
// filter are OR-combined; filters of different kinds are AND-combined.
type ListXIDBurstsOptions struct {
	Window    string
	StartTime string
	EndTime   string

	NodeUUID       string
	NodeGroupIDs   []string
	ComputeZoneIDs []string

	// Exclusion filters select every assignment except the supplied IDs. Each
	// cannot be combined with the inclusive filter for the same dimension.
	ExcludeNodeGroupIDs   []string
	ExcludeComputeZoneIDs []string

	JobDisruption                   *bool
	JobDisruptionDueToPlatformIssue *bool
	XIDNumbers                      []int

	HostnameSearch             string
	CategorySearch             string
	SubcategorySearch          string
	TenantActionSearch         string
	TenantInvestigationSearch  string
	DCAdminActionSearch        string
	DCAdminInvestigationSearch string

	Categories            []string
	Subcategories         []string
	TenantActions         []string
	TenantInvestigations  []string
	DCAdminActions        []string
	DCAdminInvestigations []string

	SortBy    XIDBurstSortBy
	SortOrder XIDBurstSortOrder
	Page      *int
	PageSize  *int
}

// XIDBurstsPage represents a paginated XID burst list with the raw backend payload
type XIDBurstsPage struct {
	Bursts   []XIDBurst `json:"items"`
	Page     int        `json:"page"`
	PageSize int        `json:"pageSize"`
	Total    int        `json:"total"`
	RawJSON  []byte     `json:"-"`
}

// PageInfo reports the pagination envelope of the response. The XID burst
// endpoint sends no hasMore field, so it is derived from the counters.
func (page XIDBurstsPage) PageInfo() PageInfo {
	hasMore := hasMoreFromCounts(page.Page, page.PageSize, page.Total)
	return PageInfo{
		Page:     page.Page,
		PageSize: page.PageSize,
		Total:    page.Total,
		HasMore:  &hasMore,
		RawJSON:  page.RawJSON,
	}
}

// XIDBurst represents one finalized XID burst. Fields are shaped server-side by
// the caller's persona: category, subcategory, platform-attributed disruption,
// XID descriptions, and DC-admin actions are omitted for tenant callers.
type XIDBurst struct {
	BurstID                         string            `json:"burstId,omitempty"`
	NodeUUID                        string            `json:"nodeUuid,omitempty"`
	Hostname                        string            `json:"hostname,omitempty"`
	NodeGroup                       string            `json:"nodeGroup,omitempty"`
	NodeGroupID                     string            `json:"nodeGroupId,omitempty"`
	ComputeZone                     string            `json:"computeZone,omitempty"`
	ComputeZoneID                   string            `json:"computeZoneId,omitempty"`
	StartTime                       string            `json:"startTime,omitempty"`
	EndTime                         string            `json:"endTime,omitempty"`
	BurstDurationSeconds            *int              `json:"burstDurationSeconds,omitempty"`
	XIDCount                        *int              `json:"xidCount,omitempty"`
	XIDNumbers                      []XIDBurstXID     `json:"xidNumbers,omitempty"`
	DeviceIDs                       map[string][]int  `json:"deviceIds,omitempty"`
	JobDisruption                   *bool             `json:"jobDisruption,omitempty"`
	JobDisruptionDueToPlatformIssue *bool             `json:"jobDisruptionDueToPlatformIssue,omitempty"`
	Category                        string            `json:"category,omitempty"`
	Subcategory                     string            `json:"subcategory,omitempty"`
	StickyXIDsSuppressed            *int              `json:"stickyXidsSuppressed,omitempty"`
	SuggestedActions                []SuggestedAction `json:"suggestedActions,omitempty"`
}

// XIDBurstDetails represents one XID burst with the raw backend payload
type XIDBurstDetails struct {
	XIDBurst
	RawJSON []byte `json:"-"`
}

// XIDBurstXID represents one XID number observed in a burst
type XIDBurstXID struct {
	XIDNumber   *int   `json:"xidNumber,omitempty"`
	Mnemonic    string `json:"mnemonic,omitempty"`
	Description string `json:"description,omitempty"`
}

// ListXIDBursts retrieves finalized XID bursts for the selected time range
func (c *Client) ListXIDBursts(ctx context.Context, opts ListXIDBurstsOptions) (XIDBurstsPage, error) {
	ctx, cancel := c.requestContext(ctx)
	defer cancel()

	if err := validateListXIDBurstsOptions(opts); err != nil {
		return XIDBurstsPage{}, err
	}
	timeRange, err := normalizeEventTimeRange(opts.Window, opts.StartTime, opts.EndTime)
	if err != nil {
		return XIDBurstsPage{}, err
	}

	mode := fleetapi.GetV1XIDBurstsParamsTimeMode(timeRange.timeMode)
	params := fleetapi.GetV1XIDBurstsParams{TimeMode: &mode}
	params.Window = optionalString(timeRange.window)
	params.StartTime = optionalString(timeRange.startTime)
	params.EndTime = optionalString(timeRange.endTime)
	params.NodeUUID = optionalTrimmedString(opts.NodeUUID)
	params.NodeGroupIds = optionalSlice(opts.NodeGroupIDs)
	params.ComputeZoneIds = optionalSlice(opts.ComputeZoneIDs)
	params.ExcludeNodeGroupIds = optionalSlice(opts.ExcludeNodeGroupIDs)
	params.ExcludeComputeZoneIds = optionalSlice(opts.ExcludeComputeZoneIDs)
	params.JobDisruption = cloneBool(opts.JobDisruption)
	params.JobDisruptionDueToPlatformIssue = cloneBool(opts.JobDisruptionDueToPlatformIssue)
	params.XidNumbers = optionalSlice(opts.XIDNumbers)
	params.HostnameSearch = optionalTrimmedString(opts.HostnameSearch)
	params.CategorySearch = optionalTrimmedString(opts.CategorySearch)
	params.SubcategorySearch = optionalTrimmedString(opts.SubcategorySearch)
	params.TenantActionSearch = optionalTrimmedString(opts.TenantActionSearch)
	params.TenantInvestigationSearch = optionalTrimmedString(opts.TenantInvestigationSearch)
	params.DcAdminActionSearch = optionalTrimmedString(opts.DCAdminActionSearch)
	params.DcAdminInvestigationSearch = optionalTrimmedString(opts.DCAdminInvestigationSearch)
	params.Categories = optionalSlice(opts.Categories)
	params.Subcategories = optionalSlice(opts.Subcategories)
	params.TenantActions = optionalSlice(opts.TenantActions)
	params.TenantInvestigations = optionalSlice(opts.TenantInvestigations)
	params.DcAdminActions = optionalSlice(opts.DCAdminActions)
	params.DcAdminInvestigations = optionalSlice(opts.DCAdminInvestigations)
	params.SortBy = optionalEnum[fleetapi.GetV1XIDBurstsParamsSortBy](opts.SortBy)
	params.SortOrder = optionalEnum[fleetapi.GetV1XIDBurstsParamsSortOrder](opts.SortOrder)
	params.Page = cloneInt(opts.Page)
	params.PageSize = cloneInt(opts.PageSize)

	resp, err := c.api.GetV1XIDBurstsWithResponse(ctx, &params)
	if err != nil {
		return XIDBurstsPage{}, err
	}
	if resp.StatusCode() != http.StatusOK {
		return XIDBurstsPage{}, newAPIError(resp.StatusCode(), resp.Status(), resp.Body)
	}

	var data fleetapi.ModelsXIDBurstsListResponse
	if err := json.Unmarshal(resp.Body, &data); err != nil {
		return XIDBurstsPage{}, err
	}

	page := XIDBurstsPage{
		Page:     intValue(data.Page),
		PageSize: intValue(data.PageSize),
		Total:    intValue(data.Total),
		RawJSON:  append([]byte(nil), resp.Body...),
	}
	if data.Items != nil {
		page.Bursts = make([]XIDBurst, 0, len(*data.Items))
		for _, item := range *data.Items {
			page.Bursts = append(page.Bursts, xidBurstFromGenerated(item))
		}
	}
	return page, nil
}

// DescribeXIDBurst retrieves the details of one finalized XID burst
func (c *Client) DescribeXIDBurst(ctx context.Context, burstID string) (XIDBurstDetails, error) {
	ctx, cancel := c.requestContext(ctx)
	defer cancel()

	burstID = strings.TrimSpace(burstID)
	if burstID == "" {
		return XIDBurstDetails{}, fmt.Errorf("burst ID is required")
	}

	resp, err := c.api.GetV1XIDBurstDetailWithResponse(ctx, burstID)
	if err != nil {
		return XIDBurstDetails{}, err
	}
	if resp.StatusCode() != http.StatusOK {
		return XIDBurstDetails{}, newAPIError(resp.StatusCode(), resp.Status(), resp.Body)
	}

	var data fleetapi.ModelsXIDBurstDetail
	if err := json.Unmarshal(resp.Body, &data); err != nil {
		return XIDBurstDetails{}, err
	}

	return XIDBurstDetails{
		XIDBurst: xidBurstDetailFromGenerated(data),
		RawJSON:  append([]byte(nil), resp.Body...),
	}, nil
}

// Checks XID burst list options before making the request
func validateListXIDBurstsOptions(opts ListXIDBurstsOptions) error {
	if opts.SortBy != "" && !opts.SortBy.Valid() {
		return fmt.Errorf("invalid XID burst sort %q", opts.SortBy)
	}
	if opts.SortOrder != "" && !opts.SortOrder.Valid() {
		return fmt.Errorf("invalid XID burst sort order %q: expected asc or desc", opts.SortOrder)
	}
	if len(opts.NodeGroupIDs) > 0 && len(opts.ExcludeNodeGroupIDs) > 0 {
		return errors.New("node group include and exclude filters cannot be combined")
	}
	if len(opts.ComputeZoneIDs) > 0 && len(opts.ExcludeComputeZoneIDs) > 0 {
		return errors.New("compute zone include and exclude filters cannot be combined")
	}
	for _, xid := range opts.XIDNumbers {
		if xid < 0 {
			return fmt.Errorf("invalid XID number %d: expected a non-negative integer", xid)
		}
	}
	return nil
}

// Maps XID burst list items into SDK values
func xidBurstFromGenerated(item fleetapi.ModelsXIDBurstListItem) XIDBurst {
	return XIDBurst{
		BurstID:                         stringValue(item.BurstId),
		NodeUUID:                        stringValue(item.NodeUuid),
		Hostname:                        stringValue(item.Hostname),
		NodeGroup:                       stringValue(item.NodeGroup),
		NodeGroupID:                     stringValue(item.NodeGroupId),
		ComputeZone:                     stringValue(item.ComputeZone),
		ComputeZoneID:                   stringValue(item.ComputeZoneId),
		StartTime:                       timeValue(item.StartTime),
		EndTime:                         timeValue(item.EndTime),
		BurstDurationSeconds:            cloneInt(item.BurstDurationSeconds),
		XIDCount:                        cloneInt(item.XidCount),
		XIDNumbers:                      xidBurstXIDsFromGenerated(item.XidNumbers),
		DeviceIDs:                       xidBurstDeviceIDsFromGenerated(item.DeviceIds),
		JobDisruption:                   cloneBool(item.JobDisruption),
		JobDisruptionDueToPlatformIssue: cloneBool(item.JobDisruptionDueToPlatformIssue),
		Category:                        stringValue(item.Category),
		Subcategory:                     stringValue(item.Subcategory),
		StickyXIDsSuppressed:            cloneInt(item.StickyXidsSuppressed),
		SuggestedActions:                suggestedActionsFromGenerated(item.SuggestedActions),
	}
}

// Maps XID burst details into SDK values. The generated detail and list-item
// models are field-identical, so the conversion below reuses the list-item
// mapper; if the two ever diverge in the spec, this stops compiling instead of
// silently dropping a field.
func xidBurstDetailFromGenerated(detail fleetapi.ModelsXIDBurstDetail) XIDBurst {
	return xidBurstFromGenerated(fleetapi.ModelsXIDBurstListItem(detail))
}

// Maps burst XID entries into SDK values
func xidBurstXIDsFromGenerated(xids *[]fleetapi.ModelsXIDBurstXID) []XIDBurstXID {
	if xids == nil {
		return nil
	}
	out := make([]XIDBurstXID, 0, len(*xids))
	for _, xid := range *xids {
		out = append(out, XIDBurstXID{
			XIDNumber:   cloneInt(xid.XidNumber),
			Mnemonic:    stringValue(xid.Mnemonic),
			Description: stringValue(xid.Description),
		})
	}
	return out
}

// Copies the impacted-device map without sharing backing storage
func xidBurstDeviceIDsFromGenerated(devices *map[string][]int) map[string][]int {
	if devices == nil {
		return nil
	}
	out := make(map[string][]int, len(*devices))
	for device, xids := range *devices {
		out[device] = append([]int(nil), xids...)
	}
	return out
}

// Personas a suggested action can target. The API omits the persona when it has
// already reduced actions to one, which for a tenant key means tenant.
const (
	ActionPersonaTenant  = "tenant"
	ActionPersonaDCAdmin = "dc_admin"
)

// Types a suggested action can carry.
const (
	ActionTypeImmediate     = "immediate"
	ActionTypeInvestigatory = "investigatory"
)

// XIDBurstFilterOptions represents the filter values available when listing XID
// bursts. The XID endpoint returns per-field value lists rather than the shared
// filters/sorting envelope the node and alert endpoints use, and publishes no
// sorting metadata.
type XIDBurstFilterOptions struct {
	XIDNumbers                      []int             `json:"xidNumbers"`
	Categories                      []string          `json:"categories"`
	Subcategories                   []string          `json:"subcategories"`
	JobDisruption                   []bool            `json:"jobDisruption"`
	JobDisruptionDueToPlatformIssue []bool            `json:"jobDisruptionDueToPlatformIssue"`
	SuggestedActions                []SuggestedAction `json:"suggestedActions"`
	RawJSON                         []byte            `json:"-"`
}

// XIDBurstFilterOptionsScope narrows the values an options lookup returns to
// those present in a time range and inventory scope. Every field is optional;
// the zero value asks for every value available to the caller. The time range
// follows the same rules as the burst list: a window selects a relative range,
// start and end select an absolute one, and the two cannot be combined.
//
// The backend applies no table column filters here, so narrowing by anything
// other than these fields is not available.
type XIDBurstFilterOptionsScope struct {
	Window    string
	StartTime string
	EndTime   string

	NodeGroupIDs   []string
	ComputeZoneIDs []string

	// Exclusion filters select every assignment except the supplied IDs. Each
	// cannot be combined with the inclusive filter for the same dimension.
	ExcludeNodeGroupIDs   []string
	ExcludeComputeZoneIDs []string
}

// Reports whether any time filter was supplied. The endpoint treats the range
// as optional, so an empty scope must send no time parameters at all rather
// than defaulting to a mode.
func (scope XIDBurstFilterOptionsScope) hasTimeRange() bool {
	return strings.TrimSpace(scope.Window) != "" ||
		strings.TrimSpace(scope.StartTime) != "" ||
		strings.TrimSpace(scope.EndTime) != ""
}

// GetXIDBurstFilterOptions gets the filter values available when listing XID
// bursts. Categories, subcategories, platform-disruption values, and DC-admin
// actions are returned only to cloud-provider/NCP callers, so a tenant key sees
// a reduced set. The scope optionally narrows the values to a time range and
// inventory selection; pass the zero value for everything available.
func (c *Client) GetXIDBurstFilterOptions(ctx context.Context, scope XIDBurstFilterOptionsScope) (XIDBurstFilterOptions, error) {
	ctx, cancel := c.requestContext(ctx)
	defer cancel()

	params, err := buildXIDBurstFilterOptionsParams(scope)
	if err != nil {
		return XIDBurstFilterOptions{}, err
	}

	resp, err := c.api.GetV1XIDBurstOptions(ctx, params)
	if err != nil {
		return XIDBurstFilterOptions{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return XIDBurstFilterOptions{}, fmt.Errorf("read XID burst filter options: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return XIDBurstFilterOptions{}, newAPIError(resp.StatusCode, resp.Status, body)
	}

	var options XIDBurstFilterOptions
	if err := json.Unmarshal(body, &options); err != nil {
		return XIDBurstFilterOptions{}, fmt.Errorf("decode XID burst filter options: %w", err)
	}
	options.RawJSON = append([]byte(nil), body...)
	return options, nil
}

// Validates a filter-options scope and builds the query parameters. The time
// range is optional here, unlike the burst list, so it is normalized only when
// the caller supplied one.
func buildXIDBurstFilterOptionsParams(scope XIDBurstFilterOptionsScope) (*fleetapi.GetV1XIDBurstOptionsParams, error) {
	if len(scope.NodeGroupIDs) > 0 && len(scope.ExcludeNodeGroupIDs) > 0 {
		return nil, errors.New("node group include and exclude filters cannot be combined")
	}
	if len(scope.ComputeZoneIDs) > 0 && len(scope.ExcludeComputeZoneIDs) > 0 {
		return nil, errors.New("compute zone include and exclude filters cannot be combined")
	}

	params := fleetapi.GetV1XIDBurstOptionsParams{
		NodeGroupIds:          optionalSlice(scope.NodeGroupIDs),
		ComputeZoneIds:        optionalSlice(scope.ComputeZoneIDs),
		ExcludeNodeGroupIds:   optionalSlice(scope.ExcludeNodeGroupIDs),
		ExcludeComputeZoneIds: optionalSlice(scope.ExcludeComputeZoneIDs),
	}
	if !scope.hasTimeRange() {
		return &params, nil
	}

	timeRange, err := normalizeEventTimeRange(scope.Window, scope.StartTime, scope.EndTime)
	if err != nil {
		return nil, err
	}
	mode := fleetapi.GetV1XIDBurstOptionsParamsTimeMode(timeRange.timeMode)
	params.TimeMode = &mode
	params.Window = optionalString(timeRange.window)
	params.StartTime = optionalString(timeRange.startTime)
	params.EndTime = optionalString(timeRange.endTime)
	return &params, nil
}
