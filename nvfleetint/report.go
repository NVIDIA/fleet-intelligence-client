// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/NVIDIA/fleet-intelligence-client/internal/generated/fleetapi"
)

// Default filename for signed inventory report downloads
const defaultSignedInventoryFilename = "inventory-report.zip"

const reportDurationUnitsMessage = "expected a positive duration using units ns, us, µs, ms, s, m, or h"

var (
	maxReportWindow       = time.Duration(1<<63 - 1)
	reportDurationPattern = regexp.MustCompile(`^\+?(?:(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:ns|us|µs|ms|s|m|h))+$`)
)

const (
	ReportFormatJSON ReportFormat = "json"
	ReportFormatCSV  ReportFormat = "csv"

	ErrorReportViewList     ErrorReportView = "list"
	ErrorReportViewGraph    ErrorReportView = "graph"
	ErrorReportViewOverview ErrorReportView = "overview"

	ErrorReportGroupByError ErrorReportGroupBy = "error"
	ErrorReportGroupByNode  ErrorReportGroupBy = "node"

	ErrorReportTimeModeAbsolute ErrorReportTimeMode = "absolute"
	ErrorReportTimeModeRelative ErrorReportTimeMode = "relative"

	InventoryReportSortByHostname       InventoryReportSortBy = "hostname"
	InventoryReportSortByNodeUUID       InventoryReportSortBy = "nodeUUID"
	InventoryReportSortByNodeGroup      InventoryReportSortBy = "nodegroup"
	InventoryReportSortByComputeZone    InventoryReportSortBy = "computezone"
	InventoryReportSortByGPUType        InventoryReportSortBy = "gpuType"
	InventoryReportSortByGPUCount       InventoryReportSortBy = "gpuCount"
	InventoryReportSortByPublicIP       InventoryReportSortBy = "publicIP"
	InventoryReportSortByPrivateIP      InventoryReportSortBy = "privateIP"
	InventoryReportSortByIntegrityCheck InventoryReportSortBy = "integrityCheck"
	InventoryReportSortByGeoLocation    InventoryReportSortBy = "geoLocation"

	InventoryReportOrderAsc  InventoryReportSortOrder = "asc"
	InventoryReportOrderDesc InventoryReportSortOrder = "desc"
)

// Represents supported report response formats
type ReportFormat string

// Reports whether the report format is accepted by the API
func (format ReportFormat) Valid() bool {
	switch format {
	case ReportFormatJSON, ReportFormatCSV:
		return true
	default:
		return false
	}
}

// Represents supported error report views
type ErrorReportView string

// Reports whether the error report view is accepted by the API
func (view ErrorReportView) Valid() bool {
	return fleetapi.GetV1ReportsErrorParamsView(view).Valid()
}

// Represents supported groupings for error list reports
type ErrorReportGroupBy string

// Reports whether the error report grouping is accepted by the API
func (groupBy ErrorReportGroupBy) Valid() bool {
	return fleetapi.GetV1ReportsErrorParamsGroupBy(groupBy).Valid()
}

// Represents supported time filter modes for error reports
type ErrorReportTimeMode string

// Reports whether the error report time mode is accepted by the API
func (mode ErrorReportTimeMode) Valid() bool {
	switch mode {
	case ErrorReportTimeModeAbsolute, ErrorReportTimeModeRelative:
		return true
	default:
		return false
	}
}

// Represents supported inventory report sort fields
type InventoryReportSortBy string

// Reports whether the inventory report sort field is accepted by the API
func (sortBy InventoryReportSortBy) Valid() bool {
	return fleetapi.GetV1ReportsInventoryParamsSortBy(sortBy).Valid()
}

// Represents supported inventory report sort orders
type InventoryReportSortOrder string

// Reports whether the inventory report sort order is accepted by the API
func (order InventoryReportSortOrder) Valid() bool {
	return fleetapi.GetV1ReportsInventoryParamsOrder(order).Valid()
}

// Represents request options for inventory reports
type InventoryReportOptions struct {
	Format         ReportFormat
	Signed         bool
	ComputeZoneIDs []string
	NodeGroupIDs   []string
	Tags           []string
	StartTime      string
	EndTime        string
	SortBy         InventoryReportSortBy
	Order          InventoryReportSortOrder
	Page           *int
	PageSize       *int
}

// Represents an inventory report response
type InventoryReport struct {
	Nodes    []InventoryNode `json:"nodes"`
	HasMore  bool            `json:"hasMore"`
	Page     int             `json:"page"`
	PageSize int             `json:"pageSize"`
	Total    int             `json:"total"`
	RawJSON  []byte          `json:"-"`
	RawCSV   []byte          `json:"-"`

	// RawSigned holds the signed CSV bundle (a zip) when Signed is requested.
	RawSigned []byte `json:"-"`
	// Filename is the server-suggested filename for a signed bundle download.
	Filename string `json:"-"`
}

// Represents one inventory row in an inventory report
type InventoryNode struct {
	NodeUUID               string       `json:"nodeUUID"`
	Hostname               string       `json:"hostname,omitempty"`
	ComputeZone            string       `json:"computeZone,omitempty"`
	NodeGroup              string       `json:"nodeGroup,omitempty"`
	GPUType                string       `json:"gpuType,omitempty"`
	GPUCount               *int         `json:"gpuCount,omitempty"`
	PublicIP               string       `json:"publicIP,omitempty"`
	PrivateIP              string       `json:"privateIP,omitempty"`
	IntegrityCheck         string       `json:"integrityCheck,omitempty"`
	IntegrityCheckReason   string       `json:"integrityCheckReason,omitempty"`
	LastIntegrityCheckTime string       `json:"lastIntegrityCheckTS,omitempty"`
	FirmwareCheck          string       `json:"firmwareCheck,omitempty"`
	EnrolledAt             string       `json:"enrolledAt,omitempty"`
	RemovedAt              string       `json:"removedAt,omitempty"`
	GeoLocation            *GeoLocation `json:"geoLocation,omitempty"`
	SerialNumbers          []string     `json:"serialNumbers,omitempty"`
}

// Represents request options for error reports
type ErrorReportOptions struct {
	View           ErrorReportView
	GroupBy        ErrorReportGroupBy
	Format         ReportFormat
	Page           *int
	PageSize       *int
	Step           string
	ComputeZoneIDs []string
	NodeGroupIDs   []string
	Tags           []string
	Errors         []string
	TimeMode       ErrorReportTimeMode
	Window         string
	StartTime      string
	EndTime        string
}

// Represents an error report response
type ErrorReport struct {
	View     ErrorReportView      `json:"view"`
	GroupBy  ErrorReportGroupBy   `json:"groupBy,omitempty"`
	Errors   []ErrorReportError   `json:"errors,omitempty"`
	Nodes    []ErrorReportNode    `json:"nodes,omitempty"`
	Overview *ErrorReportOverview `json:"overview,omitempty"`
	Graph    *ErrorReportGraph    `json:"graph,omitempty"`
	HasMore  bool                 `json:"hasMore"`
	Page     int                  `json:"page"`
	PageSize int                  `json:"pageSize"`
	Total    int                  `json:"total"`
	RawJSON  []byte               `json:"-"`
	RawCSV   []byte               `json:"-"`
}

// Represents one error grouping row in an error list report
type ErrorReportError struct {
	Name            string           `json:"name,omitempty"`
	Count           *int             `json:"count,omitempty"`
	NodeCount       *int             `json:"nodeCount,omitempty"`
	SuggestedAction *SuggestedAction `json:"suggestedAction,omitempty"`
}

// Represents one node grouping row in an error list report
type ErrorReportNode struct {
	NodeUUID string   `json:"nodeUUID,omitempty"`
	Hostname string   `json:"hostname,omitempty"`
	Errors   []string `json:"errors,omitempty"`
}

// Represents aggregate error report statistics
type ErrorReportOverview struct {
	TotalErrorNodes *int `json:"totalErrorNodes,omitempty"`
	TotalErrorTypes *int `json:"totalErrorTypes,omitempty"`
	TotalErrors     *int `json:"totalErrors,omitempty"`
}

// Represents time-series error report data
type ErrorReportGraph struct {
	Result    []ErrorTimeSeries `json:"result,omitempty"`
	TimeRange *TimeRange        `json:"timeRange,omitempty"`
}

// Represents one error time-series result
type ErrorTimeSeries struct {
	Error  string `json:"error,omitempty"`
	Values string `json:"values,omitempty"`
}

// Represents an inclusive report time range
type TimeRange struct {
	Start string `json:"start,omitempty"`
	End   string `json:"end,omitempty"`
}

// Represents the backend graph response with flexible time-series values
type errorReportGraphResponse struct {
	Result    []errorTimeSeriesResponse `json:"result,omitempty"`
	TimeRange *TimeRange                `json:"timeRange,omitempty"`
}

// Represents one backend graph time-series entry
type errorTimeSeriesResponse struct {
	Label  errorTimeSeriesLabel `json:"label,omitempty"`
	Values json.RawMessage      `json:"values,omitempty"`
}

// Represents a backend graph time-series label
type errorTimeSeriesLabel struct {
	Error string `json:"error,omitempty"`
}

// Retrieves an inventory report using the configured API client
func (c *Client) GetInventoryReport(ctx context.Context, opts InventoryReportOptions) (InventoryReport, error) {
	ctx, cancel := c.requestContext(ctx)
	defer cancel()

	format, err := normalizeReportFormat(opts.Format)
	if err != nil {
		return InventoryReport{}, err
	}
	if err := validateInventoryReportOptions(opts); err != nil {
		return InventoryReport{}, err
	}
	if opts.Signed && format != ReportFormatCSV {
		return InventoryReport{}, fmt.Errorf("signed inventory reports require csv format")
	}

	params := inventoryReportParams(opts, format)
	resp, err := c.api.GetV1ReportsInventoryWithResponse(ctx, &params, acceptReportFormat(format, opts.Signed))
	if err != nil {
		return InventoryReport{}, err
	}
	if resp.StatusCode() != http.StatusOK {
		return InventoryReport{}, newAPIError(resp.StatusCode(), resp.Status(), resp.Body)
	}
	if opts.Signed {
		if err := validateSignedReportContentType(resp.HTTPResponse); err != nil {
			return InventoryReport{}, err
		}
		return InventoryReport{
			RawSigned: append([]byte(nil), resp.Body...),
			Filename:  signedReportFilename(resp.HTTPResponse),
		}, nil
	}
	if format == ReportFormatCSV {
		return InventoryReport{RawCSV: append([]byte(nil), resp.Body...)}, nil
	}

	return decodeInventoryReport(resp.Body)
}

// Retrieves an error report using the configured API client
func (c *Client) GetErrorReport(ctx context.Context, opts ErrorReportOptions) (ErrorReport, error) {
	ctx, cancel := c.requestContext(ctx)
	defer cancel()

	normalized, err := normalizeErrorReportOptions(opts)
	if err != nil {
		return ErrorReport{}, err
	}

	params := errorReportParams(normalized)
	resp, err := c.api.GetV1ReportsErrorWithResponse(ctx, &params, acceptReportFormat(normalized.Format, false))
	if err != nil {
		return ErrorReport{}, err
	}
	if resp.StatusCode() != http.StatusOK {
		return ErrorReport{}, newAPIError(resp.StatusCode(), resp.Status(), resp.Body)
	}
	if normalized.Format == ReportFormatCSV {
		return ErrorReport{
			View:    normalized.View,
			GroupBy: normalized.GroupBy,
			RawCSV:  append([]byte(nil), resp.Body...),
		}, nil
	}

	return decodeErrorReport(resp.Body, normalized.View, normalized.GroupBy)
}

// Defaults an omitted report format and rejects unsupported values
func normalizeReportFormat(format ReportFormat) (ReportFormat, error) {
	if format == "" {
		return ReportFormatJSON, nil
	}
	if !format.Valid() {
		return "", fmt.Errorf("invalid report format %q: expected json or csv", format)
	}
	return format, nil
}

// Defaults and checks error report options before making the request
func normalizeErrorReportOptions(opts ErrorReportOptions) (ErrorReportOptions, error) {
	if opts.View == "" {
		return ErrorReportOptions{}, fmt.Errorf("error report view is required")
	}
	if !opts.View.Valid() {
		return ErrorReportOptions{}, fmt.Errorf("invalid error report view %q: expected list, graph, or overview", opts.View)
	}

	format, err := normalizeReportFormat(opts.Format)
	if err != nil {
		return ErrorReportOptions{}, err
	}
	opts.Format = format

	if opts.GroupBy != "" && !opts.GroupBy.Valid() {
		return ErrorReportOptions{}, fmt.Errorf("invalid error report group-by %q: expected error or node", opts.GroupBy)
	}
	if opts.View == ErrorReportViewList && opts.GroupBy == "" {
		return ErrorReportOptions{}, fmt.Errorf("error report group-by is required for list view")
	}
	if opts.View == ErrorReportViewGraph && opts.GroupBy == "" {
		opts.GroupBy = ErrorReportGroupByError
	}
	if opts.View == ErrorReportViewGraph && opts.GroupBy != ErrorReportGroupByError {
		return ErrorReportOptions{}, fmt.Errorf("error report graph view only supports group-by error")
	}
	if opts.View == ErrorReportViewOverview && opts.GroupBy != "" {
		return ErrorReportOptions{}, fmt.Errorf("error report overview view does not support group-by")
	}
	if opts.Format == ReportFormatCSV && opts.View != ErrorReportViewList {
		return ErrorReportOptions{}, fmt.Errorf("csv format is only supported for error report list view")
	}
	if opts.TimeMode != "" && !opts.TimeMode.Valid() {
		return ErrorReportOptions{}, fmt.Errorf("invalid error report time mode %q: expected absolute or relative", opts.TimeMode)
	}

	opts.Window = strings.TrimSpace(opts.Window)
	opts.StartTime = strings.TrimSpace(opts.StartTime)
	opts.EndTime = strings.TrimSpace(opts.EndTime)
	if err := validateErrorReportTime(opts); err != nil {
		return ErrorReportOptions{}, err
	}

	return opts, nil
}

// Rejects conflicting or malformed error report time options
func validateErrorReportTime(opts ErrorReportOptions) error {
	switch opts.TimeMode {
	case ErrorReportTimeModeRelative:
		if opts.Window == "" {
			return fmt.Errorf("error report relative time mode requires window")
		}
		if opts.StartTime != "" || opts.EndTime != "" {
			return fmt.Errorf("error report relative time mode does not support start time or end time")
		}
	case ErrorReportTimeModeAbsolute:
		if opts.StartTime == "" || opts.EndTime == "" {
			return fmt.Errorf("error report absolute time mode requires start time and end time")
		}
		if opts.Window != "" {
			return fmt.Errorf("error report absolute time mode does not support window")
		}
	}

	if opts.Window != "" {
		if err := validateReportWindow(opts.Window); err != nil {
			return err
		}
	}
	if opts.StartTime != "" {
		if err := validateReportRFC3339("start time", opts.StartTime); err != nil {
			return err
		}
	}
	if opts.EndTime != "" {
		if err := validateReportRFC3339("end time", opts.EndTime); err != nil {
			return err
		}
	}
	return nil
}

// Checks a relative window value is a positive Go duration
func validateReportWindow(window string) error {
	if !reportDurationPattern.MatchString(window) {
		return fmt.Errorf("invalid window %q: %s", window, reportDurationUnitsMessage)
	}
	duration, err := time.ParseDuration(window)
	if err != nil {
		return fmt.Errorf("invalid window %q: duration is too large; maximum is %s", window, maxReportWindow)
	}
	if duration <= 0 {
		return fmt.Errorf("invalid window %q: %s", window, reportDurationUnitsMessage)
	}
	return nil
}

// Checks a timestamp value is RFC3339
func validateReportRFC3339(name, value string) error {
	if _, err := time.Parse(time.RFC3339, value); err != nil {
		return fmt.Errorf("error report %s must be RFC3339", name)
	}
	return nil
}

// Checks inventory report options before making the request
func validateInventoryReportOptions(opts InventoryReportOptions) error {
	if opts.SortBy != "" && !opts.SortBy.Valid() {
		return fmt.Errorf("invalid inventory report sort %q: expected hostname, nodeUUID, nodegroup, computezone, gpuType, gpuCount, publicIP, privateIP, integrityCheck, or geoLocation", opts.SortBy)
	}
	if opts.Order != "" && !opts.Order.Valid() {
		return fmt.Errorf("invalid inventory report order %q: expected asc or desc", opts.Order)
	}
	return nil
}

// Builds generated inventory report query parameters
func inventoryReportParams(opts InventoryReportOptions, format ReportFormat) fleetapi.GetV1ReportsInventoryParams {
	params := fleetapi.GetV1ReportsInventoryParams{}
	if format != "" {
		param := fleetapi.GetV1ReportsInventoryParamsFormat(format)
		params.Format = &param
	}
	if opts.Signed {
		signed := true
		params.Signed = &signed
	}
	if opts.Page != nil {
		params.Page = cloneInt(opts.Page)
	}
	if opts.PageSize != nil {
		params.PageSize = cloneInt(opts.PageSize)
	}
	if len(opts.ComputeZoneIDs) > 0 {
		values := append([]string(nil), opts.ComputeZoneIDs...)
		params.ComputeZoneIds = &values
	}
	if len(opts.NodeGroupIDs) > 0 {
		values := append([]string(nil), opts.NodeGroupIDs...)
		params.NodeGroupIds = &values
	}
	if len(opts.Tags) > 0 {
		values := append([]string(nil), opts.Tags...)
		params.Tags = &values
	}
	if opts.StartTime != "" {
		value := opts.StartTime
		params.StartTime = &value
	}
	if opts.EndTime != "" {
		value := opts.EndTime
		params.EndTime = &value
	}
	if opts.SortBy != "" {
		value := fleetapi.GetV1ReportsInventoryParamsSortBy(opts.SortBy)
		params.SortBy = &value
	}
	if opts.Order != "" {
		value := fleetapi.GetV1ReportsInventoryParamsOrder(opts.Order)
		params.Order = &value
	}
	return params
}

// Builds generated error report query parameters
func errorReportParams(opts ErrorReportOptions) fleetapi.GetV1ReportsErrorParams {
	params := fleetapi.GetV1ReportsErrorParams{
		View: fleetapi.GetV1ReportsErrorParamsView(opts.View),
	}
	if opts.GroupBy != "" {
		value := fleetapi.GetV1ReportsErrorParamsGroupBy(opts.GroupBy)
		params.GroupBy = &value
	}
	if opts.Format != "" {
		value := fleetapi.GetV1ReportsErrorParamsFormat(opts.Format)
		params.Format = &value
	}
	if opts.Page != nil {
		params.Page = cloneInt(opts.Page)
	}
	if opts.PageSize != nil {
		params.PageSize = cloneInt(opts.PageSize)
	}
	if opts.Step != "" {
		value := opts.Step
		params.Step = &value
	}
	if len(opts.ComputeZoneIDs) > 0 {
		values := append([]string(nil), opts.ComputeZoneIDs...)
		params.ComputeZoneIds = &values
	}
	if len(opts.NodeGroupIDs) > 0 {
		values := append([]string(nil), opts.NodeGroupIDs...)
		params.NodeGroupIds = &values
	}
	if len(opts.Tags) > 0 {
		values := append([]string(nil), opts.Tags...)
		params.Tags = &values
	}
	if len(opts.Errors) > 0 {
		values := append([]string(nil), opts.Errors...)
		params.Errors = &values
	}
	if opts.TimeMode != "" {
		value := string(opts.TimeMode)
		params.TimeMode = &value
	}
	if opts.Window != "" {
		value := opts.Window
		params.Window = &value
	}
	if opts.StartTime != "" {
		value := opts.StartTime
		params.StartTime = &value
	}
	if opts.EndTime != "" {
		value := opts.EndTime
		params.EndTime = &value
	}
	return params
}

// Overrides the request Accept header for CSV and signed report downloads
func acceptReportFormat(format ReportFormat, signed bool) fleetapi.RequestEditorFn {
	return func(_ context.Context, req *http.Request) error {
		switch {
		case signed:
			req.Header.Set("Accept", "application/zip")
		case format == ReportFormatCSV:
			req.Header.Set("Accept", "text/csv")
		}
		return nil
	}
}

// Validates that a signed report response contains a zip bundle
func validateSignedReportContentType(resp *http.Response) error {
	if resp == nil {
		return fmt.Errorf("signed inventory report response missing content type")
	}
	contentType := resp.Header.Get("Content-Type")
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil || !strings.EqualFold(mediaType, "application/zip") {
		if strings.TrimSpace(contentType) == "" {
			return fmt.Errorf("signed inventory report response missing content type")
		}
		return fmt.Errorf("signed inventory report response has content type %q, expected application/zip", contentType)
	}
	return nil
}

// Derives the filename for a signed report download from the response headers
func signedReportFilename(resp *http.Response) string {
	if resp == nil {
		return defaultSignedInventoryFilename
	}
	disposition := resp.Header.Get("Content-Disposition")
	if disposition == "" {
		return defaultSignedInventoryFilename
	}
	_, params, err := mime.ParseMediaType(disposition)
	if err != nil {
		return defaultSignedInventoryFilename
	}
	// Strip any path components a server might include to avoid traversal.
	if name := filepath.Base(strings.TrimSpace(params["filename"])); name != "" && name != "." && name != string(filepath.Separator) {
		return name
	}
	return defaultSignedInventoryFilename
}

// Decodes inventory report responses and preserves the original payload
func decodeInventoryReport(data []byte) (InventoryReport, error) {
	var resp fleetapi.ModelsInventoryReportResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return InventoryReport{}, err
	}

	report := InventoryReport{
		HasMore:  boolValue(resp.HasMore),
		Page:     intValue(resp.Page),
		PageSize: intValue(resp.PageSize),
		Total:    intValue(resp.Total),
		RawJSON:  append([]byte(nil), data...),
	}
	if resp.Nodes != nil {
		report.Nodes = make([]InventoryNode, 0, len(*resp.Nodes))
		for _, node := range *resp.Nodes {
			report.Nodes = append(report.Nodes, inventoryNodeFromGenerated(node))
		}
	}

	return report, nil
}

// Decodes error report responses and preserves the original payload
func decodeErrorReport(data []byte, view ErrorReportView, groupBy ErrorReportGroupBy) (ErrorReport, error) {
	switch view {
	case ErrorReportViewList:
		if groupBy == ErrorReportGroupByNode {
			return decodeErrorReportByNode(data, view, groupBy)
		}
		return decodeErrorReportByError(data, view, groupBy)
	case ErrorReportViewGraph:
		return decodeErrorReportGraph(data, view, groupBy)
	case ErrorReportViewOverview:
		return decodeErrorReportOverview(data, view)
	default:
		return ErrorReport{}, fmt.Errorf("invalid error report view %q", view)
	}
}

// Decodes list error reports grouped by error
func decodeErrorReportByError(data []byte, view ErrorReportView, groupBy ErrorReportGroupBy) (ErrorReport, error) {
	var resp fleetapi.ModelsByErrorResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return ErrorReport{}, err
	}

	report := ErrorReport{
		View:     view,
		GroupBy:  groupBy,
		HasMore:  boolValue(resp.HasMore),
		Page:     intValue(resp.Page),
		PageSize: intValue(resp.PageSize),
		Total:    intValue(resp.Total),
		RawJSON:  append([]byte(nil), data...),
	}
	if resp.Errors != nil {
		report.Errors = make([]ErrorReportError, 0, len(*resp.Errors))
		for _, item := range *resp.Errors {
			report.Errors = append(report.Errors, errorReportErrorFromGenerated(item))
		}
	}

	return report, nil
}

// Decodes list error reports grouped by node
func decodeErrorReportByNode(data []byte, view ErrorReportView, groupBy ErrorReportGroupBy) (ErrorReport, error) {
	var resp fleetapi.ModelsByNodeResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return ErrorReport{}, err
	}

	report := ErrorReport{
		View:     view,
		GroupBy:  groupBy,
		HasMore:  boolValue(resp.HasMore),
		Page:     intValue(resp.Page),
		PageSize: intValue(resp.PageSize),
		Total:    intValue(resp.Total),
		RawJSON:  append([]byte(nil), data...),
	}
	if resp.Nodes != nil {
		report.Nodes = make([]ErrorReportNode, 0, len(*resp.Nodes))
		for _, item := range *resp.Nodes {
			report.Nodes = append(report.Nodes, errorReportNodeFromGenerated(item))
		}
	}

	return report, nil
}

// Decodes graph error reports
func decodeErrorReportGraph(data []byte, view ErrorReportView, groupBy ErrorReportGroupBy) (ErrorReport, error) {
	var resp errorReportGraphResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return ErrorReport{}, err
	}

	return ErrorReport{
		View:    view,
		GroupBy: groupBy,
		Graph:   errorReportGraphFromResponse(resp),
		RawJSON: append([]byte(nil), data...),
	}, nil
}

// Decodes overview error reports
func decodeErrorReportOverview(data []byte, view ErrorReportView) (ErrorReport, error) {
	var resp fleetapi.ModelsErrorOverviewResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return ErrorReport{}, err
	}

	return ErrorReport{
		View: view,
		Overview: &ErrorReportOverview{
			TotalErrorNodes: cloneInt(resp.TotalErrorNodes),
			TotalErrorTypes: cloneInt(resp.TotalErrorTypes),
			TotalErrors:     cloneInt(resp.TotalErrors),
		},
		RawJSON: append([]byte(nil), data...),
	}, nil
}

// Maps inventory node API models into SDK values
func inventoryNodeFromGenerated(node fleetapi.ModelsInventoryNode) InventoryNode {
	return InventoryNode{
		NodeUUID:               stringValue(node.NodeUUID),
		Hostname:               stringValue(node.Hostname),
		ComputeZone:            stringValue(node.ComputeZone),
		NodeGroup:              stringValue(node.NodeGroup),
		GPUType:                stringValue(node.GpuType),
		GPUCount:               cloneInt(node.GpuCount),
		PublicIP:               stringValue(node.PublicIP),
		PrivateIP:              stringValue(node.PrivateIP),
		IntegrityCheck:         enumStringValue(node.IntegrityCheck),
		IntegrityCheckReason:   stringValue(node.IntegrityCheckReason),
		LastIntegrityCheckTime: stringValue(node.LastIntegrityCheckTS),
		FirmwareCheck:          enumStringValue(node.FirmwareCheck),
		EnrolledAt:             stringValue(node.EnrolledAt),
		RemovedAt:              stringValue(node.RemovedAt),
		GeoLocation:            geoLocationFromGenerated(node.GeoLocation),
		SerialNumbers:          cloneStringSlice(node.SerialNumbers),
	}
}

// Maps error report error API models into SDK values
func errorReportErrorFromGenerated(item fleetapi.ModelsByError) ErrorReportError {
	return ErrorReportError{
		Name:            stringValue(item.Name),
		Count:           cloneInt(item.Count),
		NodeCount:       cloneInt(item.NodeCount),
		SuggestedAction: suggestedActionFromGenerated(item.SuggestedAction),
	}
}

// Maps error report node API models into SDK values
func errorReportNodeFromGenerated(item fleetapi.ModelsByNode) ErrorReportNode {
	return ErrorReportNode{
		NodeUUID: stringValue(item.NodeUUID),
		Hostname: stringValue(item.Hostname),
		Errors:   cloneStringSlice(item.Errors),
	}
}

// Maps graph error report API responses into SDK values
func errorReportGraphFromResponse(resp errorReportGraphResponse) *ErrorReportGraph {
	graph := &ErrorReportGraph{
		TimeRange: resp.TimeRange,
	}
	if len(resp.Result) > 0 {
		graph.Result = make([]ErrorTimeSeries, 0, len(resp.Result))
	}
	for _, item := range resp.Result {
		graph.Result = append(graph.Result, errorTimeSeriesFromResponse(item))
	}
	return graph
}

// Maps error report time-series responses into SDK values
func errorTimeSeriesFromResponse(item errorTimeSeriesResponse) ErrorTimeSeries {
	return ErrorTimeSeries{
		Error:  item.Label.Error,
		Values: reportRawValueString(item.Values),
	}
}

// Converts a raw graph values field into a displayable string
func reportRawValueString(value json.RawMessage) string {
	value = bytes.TrimSpace(value)
	if len(value) == 0 || bytes.Equal(value, []byte("null")) {
		return ""
	}

	var text string
	if err := json.Unmarshal(value, &text); err == nil {
		return text
	}

	var compact bytes.Buffer
	if err := json.Compact(&compact, value); err == nil {
		return compact.String()
	}
	return string(value)
}

// Maps one generated suggested action into an SDK value
func suggestedActionFromGenerated(action *fleetapi.ModelsSuggestedAction) *SuggestedAction {
	if action == nil {
		return nil
	}
	return &SuggestedAction{
		Action:  stringValue(action.Action),
		Code:    stringValue(action.Code),
		Persona: enumStringValue(action.Persona),
		Type:    enumStringValue(action.Type),
	}
}
