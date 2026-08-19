// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/NVIDIA/fleet-intelligence-client/internal/generated/fleetapi"
)

const (
	ComputeZoneViewDetail ComputeZoneView = "detail"
	ComputeZoneViewBasic  ComputeZoneView = "basic"
)

const (
	ComputeZoneTypeDatacenter    ComputeZoneType = "datacenter"
	ComputeZoneTypeCloudProvider ComputeZoneType = "cloud provider"
)

// Represents supported response shapes for listing compute zones
type ComputeZoneView string

// Represents supported compute zone types
type ComputeZoneType string

// Reports whether the view is accepted by the API
func (view ComputeZoneView) Valid() bool {
	return fleetapi.GetV1ComputezonesParamsView(view).Valid()
}

// Reports whether the type is accepted by the API
func (zoneType ComputeZoneType) Valid() bool {
	return fleetapi.ModelsComputeZoneType(zoneType).Valid()
}

// Represents request options for listing compute zones
type ListComputeZonesOptions struct {
	View           ComputeZoneView
	IncludeMetrics *bool
	ZoneIDs        []string
	Page           *int
	PageSize       *int
}

// Represents a paginated compute zone response with the raw backend payload
type ComputeZonesPage struct {
	ComputeZones []ComputeZone `json:"computezones"`
	HasMore      bool          `json:"hasMore"`
	Page         int           `json:"page"`
	PageSize     int           `json:"pageSize"`
	Total        int           `json:"total"`
	RawJSON      []byte        `json:"-"`
}

// Represents contact metadata for a compute zone
type Contact struct {
	Email string `json:"email,omitempty"`
	PIC   string `json:"pic,omitempty"`
}

// Represents a compute zone
type ComputeZone struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Type        string       `json:"type,omitempty"`
	Contact     *Contact     `json:"contact,omitempty"`
	GeoLocation *GeoLocation `json:"geoLocation,omitempty"`
	NodeCount   *int         `json:"nodeCount,omitempty"`
}

// Represents request options for updating a compute zone. Pointer fields are
// values the caller wants to change; nil fields preserve the backend value.
// Coordinates are text so an untouched value round-trips verbatim and an empty
// value can clear one; validate them with ValidateLatitude/ValidateLongitude.
type UpdateComputeZoneOptions struct {
	ID           string
	Type         *string
	ContactEmail *string
	ContactPIC   *string
	GeoCity      *string
	GeoCountry   *string
	GeoRegion    *string
	GeoLatitude  *string
	GeoLongitude *string
}

// Represents an update response with the raw backend payload
type UpdateComputeZoneResult struct {
	ID      string `json:"id"`
	RawJSON []byte `json:"-"`
}

// Lists compute zones using the configured API client
func (c *Client) ListComputeZones(ctx context.Context, opts ListComputeZonesOptions) (ComputeZonesPage, error) {
	ctx, cancel := c.requestContext(ctx)
	defer cancel()

	view, err := normalizeComputeZoneView(opts.View)
	if err != nil {
		return ComputeZonesPage{}, err
	}

	params := fleetapi.GetV1ComputezonesParams{
		View: computeZoneViewParam(view),
	}
	if view == ComputeZoneViewBasic && opts.IncludeMetrics != nil {
		return ComputeZonesPage{}, fmt.Errorf("basic compute zone view is incompatible with include metrics")
	}
	if opts.IncludeMetrics != nil {
		params.IncludeMetrics = cloneBool(opts.IncludeMetrics)
	}
	if len(opts.ZoneIDs) > 0 {
		zoneIDs := append([]string(nil), opts.ZoneIDs...)
		params.ComputeZoneIds = &zoneIDs
	}
	if opts.Page != nil {
		params.Page = cloneInt(opts.Page)
	}
	if opts.PageSize != nil {
		params.PageSize = cloneInt(opts.PageSize)
	}

	resp, err := c.api.GetV1ComputezonesWithResponse(ctx, &params)
	if err != nil {
		return ComputeZonesPage{}, err
	}
	if resp.StatusCode() != http.StatusOK {
		return ComputeZonesPage{}, newAPIError(resp.StatusCode(), resp.Status(), resp.Body)
	}

	if view == ComputeZoneViewBasic {
		return decodeBasicComputeZones(resp.Body)
	}

	return decodeDetailComputeZones(resp.Body)
}

// Updates a compute zone by first reading its current backend state and then
// preserving fields the caller left nil. The API has no conditional-update
// mechanism, so this read-modify-write flow is last-write-wins and can
// overwrite concurrent changes made after the read.
func (c *Client) UpdateComputeZone(ctx context.Context, opts UpdateComputeZoneOptions) (UpdateComputeZoneResult, error) {
	body, err := c.buildUpdateComputeZoneRequest(ctx, opts)
	if err != nil {
		return UpdateComputeZoneResult{}, err
	}
	data, err := json.Marshal(body)
	if err != nil {
		return UpdateComputeZoneResult{}, err
	}

	ctx, cancel := c.requestContext(ctx)
	defer cancel()

	resp, err := c.api.PutV1ComputezonesWithBodyWithResponse(ctx, "application/json", bytes.NewReader(data))
	if err != nil {
		return UpdateComputeZoneResult{}, err
	}
	if resp.StatusCode() != http.StatusOK {
		return UpdateComputeZoneResult{}, newAPIError(resp.StatusCode(), resp.Status(), resp.Body)
	}

	result := UpdateComputeZoneResult{
		RawJSON: append([]byte(nil), resp.Body...),
	}
	if resp.JSON200 != nil {
		result.ID = stringValue(resp.JSON200.Id)
	}

	return result, nil
}

// Defaults an omitted view and rejects unsupported values
func normalizeComputeZoneView(view ComputeZoneView) (ComputeZoneView, error) {
	if view == "" {
		return ComputeZoneViewDetail, nil
	}
	if !view.Valid() {
		return "", fmt.Errorf("invalid compute zone view %q: expected basic or detail", view)
	}

	return view, nil
}

// Converts a normalized view into the generated parameter type
func computeZoneViewParam(view ComputeZoneView) *fleetapi.GetV1ComputezonesParamsView {
	param := fleetapi.GetV1ComputezonesParamsView(view)
	return &param
}

// Represents the update request body. This mirrors
// fleetapi.ModelsUpdateComputeZoneRequest but carries coordinates as
// json.Number: the generated model decodes them as float32, so echoing an
// untouched location back through it would rewrite the stored value at
// float32 precision on every unrelated edit.
type updateComputeZoneBody struct {
	Contact     *computeZoneContactBody     `json:"contact,omitempty"`
	GeoLocation *computeZoneGeoLocationBody `json:"geoLocation,omitempty"`
	ID          string                      `json:"id"`
	Type        *string                     `json:"type,omitempty"`
}

// Mirrors fleetapi.ModelsContact for the request body
type computeZoneContactBody struct {
	Email *string `json:"email,omitempty"`
	Pic   *string `json:"pic,omitempty"`
}

// Mirrors fleetapi.ModelsGeoLocation for the request body, keeping coordinates
// as the backend's own number text
type computeZoneGeoLocationBody struct {
	City      *string      `json:"city,omitempty"`
	Country   *string      `json:"country,omitempty"`
	Latitude  *json.Number `json:"latitude,omitempty"`
	Longitude *json.Number `json:"longitude,omitempty"`
	Region    *string      `json:"region,omitempty"`
}

// Represents the stored compute zone fields the update merge reads back,
// decoded straight from the backend payload so coordinate text survives
type currentComputeZone struct {
	ID          string                      `json:"id"`
	Type        *string                     `json:"type"`
	Contact     *computeZoneContactBody     `json:"contact"`
	GeoLocation *computeZoneGeoLocationBody `json:"geoLocation"`
}

// Represents the list envelope currentComputeZone is read from
type currentComputeZonesResponse struct {
	ComputeZones []currentComputeZone `json:"computezones"`
}

// Builds the request body shared by UpdateComputeZone and
// PreviewUpdateComputeZone so a dry run can never disagree with the write.
// The request body includes values read from the backend for fields the caller
// left nil; because the API has no ETag, version, or If-Match equivalent,
// preview-then-update and direct update flows are last-write-wins and can
// overwrite concurrent changes made after the read.
func (c *Client) buildUpdateComputeZoneRequest(ctx context.Context, opts UpdateComputeZoneOptions) (updateComputeZoneBody, error) {
	opts.ID = strings.TrimSpace(opts.ID)
	if err := validateUpdateComputeZoneOptions(opts); err != nil {
		return updateComputeZoneBody{}, err
	}

	current, err := c.currentComputeZone(ctx, opts.ID)
	if err != nil {
		return updateComputeZoneBody{}, err
	}

	req := updateComputeZoneBody{ID: opts.ID, Type: cloneString(current.Type)}
	if opts.Type != nil {
		zoneType := strings.TrimSpace(*opts.Type)
		req.Type = &zoneType
	}

	contact := cloneComputeZoneContact(current.Contact)
	if opts.ContactEmail != nil {
		if contact == nil {
			contact = &computeZoneContactBody{}
		}
		contact.Email = trimmedStringPointer(*opts.ContactEmail)
	}
	if opts.ContactPIC != nil {
		if contact == nil {
			contact = &computeZoneContactBody{}
		}
		contact.Pic = trimmedStringPointer(*opts.ContactPIC)
	}
	req.Contact = contact

	location := cloneComputeZoneGeoLocation(current.GeoLocation)
	if opts.GeoCity != nil {
		if location == nil {
			location = &computeZoneGeoLocationBody{}
		}
		location.City = trimmedStringPointer(*opts.GeoCity)
	}
	if opts.GeoCountry != nil {
		if location == nil {
			location = &computeZoneGeoLocationBody{}
		}
		location.Country = trimmedStringPointer(*opts.GeoCountry)
	}
	if opts.GeoRegion != nil {
		if location == nil {
			location = &computeZoneGeoLocationBody{}
		}
		location.Region = trimmedStringPointer(*opts.GeoRegion)
	}
	if opts.GeoLatitude != nil {
		if location == nil {
			location = &computeZoneGeoLocationBody{}
		}
		location.Latitude = coordinateNumber(*opts.GeoLatitude)
	}
	if opts.GeoLongitude != nil {
		if location == nil {
			location = &computeZoneGeoLocationBody{}
		}
		location.Longitude = coordinateNumber(*opts.GeoLongitude)
	}
	req.GeoLocation = location

	return req, nil
}

func validateUpdateComputeZoneOptions(opts UpdateComputeZoneOptions) error {
	if strings.TrimSpace(opts.ID) == "" {
		return fmt.Errorf("compute zone ID is required")
	}
	if len(opts.ID) > 255 {
		return fmt.Errorf("compute zone ID must be at most 255 characters")
	}
	if opts.Type != nil {
		zoneType := strings.TrimSpace(*opts.Type)
		if zoneType == "" {
			return fmt.Errorf("compute zone type cannot be empty")
		}
		if !ComputeZoneType(zoneType).Valid() {
			return fmt.Errorf("invalid compute zone type %q: expected datacenter or cloud provider", zoneType)
		}
	}
	// An empty coordinate clears the value, so only real values are checked.
	if opts.GeoLatitude != nil && strings.TrimSpace(*opts.GeoLatitude) != "" {
		if err := ValidateLatitude(*opts.GeoLatitude); err != nil {
			return err
		}
	}
	if opts.GeoLongitude != nil && strings.TrimSpace(*opts.GeoLongitude) != "" {
		if err := ValidateLongitude(*opts.GeoLongitude); err != nil {
			return err
		}
	}
	return nil
}

// Reads the stored compute zone the update merges over. There is no read-one
// endpoint, so the list is filtered to the requested ID.
func (c *Client) currentComputeZone(ctx context.Context, id string) (currentComputeZone, error) {
	includeMetrics := false
	page, err := c.ListComputeZones(ctx, ListComputeZonesOptions{
		View:           ComputeZoneViewDetail,
		IncludeMetrics: &includeMetrics,
		ZoneIDs:        []string{id},
	})
	if err != nil {
		return currentComputeZone{}, err
	}

	var resp currentComputeZonesResponse
	if err := json.Unmarshal(page.RawJSON, &resp); err != nil {
		return currentComputeZone{}, err
	}
	for _, zone := range resp.ComputeZones {
		if zone.ID == id {
			return zone, nil
		}
	}

	return currentComputeZone{}, fmt.Errorf("compute zone %q not found", id)
}

func cloneComputeZoneContact(contact *computeZoneContactBody) *computeZoneContactBody {
	if contact == nil {
		return nil
	}
	return &computeZoneContactBody{
		Email: cloneString(contact.Email),
		Pic:   cloneString(contact.Pic),
	}
}

func cloneComputeZoneGeoLocation(location *computeZoneGeoLocationBody) *computeZoneGeoLocationBody {
	if location == nil {
		return nil
	}
	return &computeZoneGeoLocationBody{
		City:      cloneString(location.City),
		Country:   cloneString(location.Country),
		Latitude:  cloneJSONNumber(location.Latitude),
		Longitude: cloneJSONNumber(location.Longitude),
		Region:    cloneString(location.Region),
	}
}

// Converts a coordinate option into wire text. An empty value clears the
// coordinate by omitting it from the replacement document.
func coordinateNumber(value string) *json.Number {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	number := json.Number(trimmed)
	return &number
}

func trimmedStringPointer(value string) *string {
	trimmed := strings.TrimSpace(value)
	return &trimmed
}

// Decodes detail responses and preserves the original payload
func decodeDetailComputeZones(data []byte) (ComputeZonesPage, error) {
	var resp fleetapi.ModelsComputeZonesResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return ComputeZonesPage{}, err
	}

	page := ComputeZonesPage{
		HasMore:  boolValue(resp.HasMore),
		Page:     intValue(resp.Page),
		PageSize: intValue(resp.PageSize),
		Total:    intValue(resp.Total),
		RawJSON:  append([]byte(nil), data...),
	}
	if resp.Computezones != nil {
		page.ComputeZones = make([]ComputeZone, 0, len(*resp.Computezones))
		for _, zone := range *resp.Computezones {
			page.ComputeZones = append(page.ComputeZones, computeZoneFromOverview(zone))
		}
	}

	return page, nil
}

// Decodes basic responses and preserves the original payload
func decodeBasicComputeZones(data []byte) (ComputeZonesPage, error) {
	var resp fleetapi.ModelsListComputeZonesResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return ComputeZonesPage{}, err
	}

	page := ComputeZonesPage{
		HasMore:  boolValue(resp.HasMore),
		Page:     intValue(resp.Page),
		PageSize: intValue(resp.PageSize),
		Total:    intValue(resp.Total),
		RawJSON:  append([]byte(nil), data...),
	}
	if resp.Computezones != nil {
		page.ComputeZones = make([]ComputeZone, 0, len(*resp.Computezones))
		for _, zone := range *resp.Computezones {
			page.ComputeZones = append(page.ComputeZones, computeZoneFromSimple(zone))
		}
	}

	return page, nil
}

func contactFromGenerated(contact *fleetapi.ModelsContact) *Contact {
	if contact == nil {
		return nil
	}
	return &Contact{
		Email: stringValue(contact.Email),
		PIC:   stringValue(contact.Pic),
	}
}

// Maps detail API models into SDK values
func computeZoneFromOverview(zone fleetapi.ModelsComputeZoneOverview) ComputeZone {
	return ComputeZone{
		ID:          stringValue(zone.Id),
		Name:        stringValue(zone.Name),
		Type:        enumStringValue(zone.Type),
		Contact:     contactFromGenerated(zone.Contact),
		GeoLocation: geoLocationFromGenerated(zone.GeoLocation),
		NodeCount:   cloneInt(zone.NodesCount),
	}
}

// Maps basic API models into SDK values
func computeZoneFromSimple(zone fleetapi.ModelsSimpleComputeZone) ComputeZone {
	return ComputeZone{
		ID:          stringValue(zone.Id),
		Name:        stringValue(zone.Name),
		Type:        enumStringValue(zone.Type),
		Contact:     contactFromGenerated(zone.Contact),
		GeoLocation: geoLocationFromGenerated(zone.GeoLocation),
	}
}
