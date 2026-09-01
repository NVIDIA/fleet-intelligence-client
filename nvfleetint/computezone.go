// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/NVIDIA/fleet-intelligence-client/internal/generated/fleetapi"
)

const (
	ComputeZoneViewDetail ComputeZoneView = "detail"
	ComputeZoneViewBasic  ComputeZoneView = "basic"
)

// Represents supported response shapes for listing compute zones
type ComputeZoneView string

// Reports whether the view is accepted by the API
func (view ComputeZoneView) Valid() bool {
	return fleetapi.GetV1ComputezonesParamsView(view).Valid()
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

// PageInfo reports the pagination envelope of the response.
func (page ComputeZonesPage) PageInfo() PageInfo {
	hasMore := page.HasMore
	return PageInfo{
		Page:     page.Page,
		PageSize: page.PageSize,
		Total:    page.Total,
		HasMore:  &hasMore,
		RawJSON:  page.RawJSON,
	}
}

// Represents a compute zone
type ComputeZone struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type,omitempty"`
	Location  *Location `json:"location,omitempty"`
	NodeCount *int      `json:"nodeCount,omitempty"`
}

// Lists compute zones using the configured API client
func (c *Client) ListComputeZones(ctx context.Context, opts ListComputeZonesOptions) (ComputeZonesPage, error) {
	ctx, cancel := c.requestContext(ctx)
	defer cancel()

	view, err := opts.normalize()
	if err != nil {
		return ComputeZonesPage{}, err
	}

	params := fleetapi.GetV1ComputezonesParams{
		View: computeZoneViewParam(view),
	}
	params.IncludeMetrics = cloneBool(opts.IncludeMetrics)
	params.ComputeZoneIds = optionalSlice(opts.ZoneIDs)
	params.Page = cloneInt(opts.Page)
	params.PageSize = cloneInt(opts.PageSize)

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

// The accepted values named in each compute zone option's error
const computeZoneViewValues = "basic or detail"

// Validate reports whether the options describe a request the API accepts.
// ListComputeZones calls it, and a caller can call it first to reject a bad
// request without opening a connection.
func (opts ListComputeZonesOptions) Validate() error {
	_, err := opts.normalize()
	return err
}

// Defaults an omitted view and checks every option against it
func (opts ListComputeZonesOptions) normalize() (ComputeZoneView, error) {
	view := opts.View
	if view == "" {
		view = ComputeZoneViewDetail
	} else if !view.Valid() {
		return "", invalidOption("view", "compute zone view", string(view), computeZoneViewValues)
	}

	if view == ComputeZoneViewBasic && opts.IncludeMetrics != nil {
		return "", errors.New("basic compute zone view is incompatible with include metrics")
	}

	return view, nil
}

// Converts a normalized view into the generated parameter type
func computeZoneViewParam(view ComputeZoneView) *fleetapi.GetV1ComputezonesParamsView {
	param := fleetapi.GetV1ComputezonesParamsView(view)
	return &param
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

// Maps detail API models into SDK values
func computeZoneFromOverview(zone fleetapi.ModelsComputeZoneOverview) ComputeZone {
	return ComputeZone{
		ID:        stringValue(zone.Id),
		Name:      stringValue(zone.Name),
		Type:      enumStringValue(zone.Type),
		Location:  locationFromGenerated(zone.Location),
		NodeCount: cloneInt(zone.NodesCount),
	}
}

// Maps basic API models into SDK values
func computeZoneFromSimple(zone fleetapi.ModelsSimpleComputeZone) ComputeZone {
	return ComputeZone{
		ID:       stringValue(zone.Id),
		Name:     stringValue(zone.Name),
		Type:     enumStringValue(zone.Type),
		Location: locationFromGenerated(zone.Location),
	}
}

// UpdateComputeZoneOptions represents request options for updating a compute
// zone's metadata. Every field is optional: a nil field leaves the stored value
// alone, and a non-nil pointer to an empty string clears it. Compute zone names
// are agent-managed and cannot be set through the customer API, so there is no
// name option.
//
// Coordinates are text rather than numbers for two reasons. The generated model
// decodes them as float32, so round-tripping a stored coordinate through it
// would rewrite the backend's value at float32 precision on every unrelated
// edit; and a numeric field has no value meaning "clear this".
type UpdateComputeZoneOptions struct {
	Type              *string
	ContactEmail      *string
	ContactPIC        *string
	LocationCity      *string
	LocationCountry   *string
	LocationRegion    *string
	LocationLatitude  *string
	LocationLongitude *string
}

// ComputeZoneUpdate reports the compute zone metadata written by
// UpdateComputeZone, together with the raw backend payload.
//
// The fields describe the document that was sent: PUT /v1/computezones replaces
// the zone's metadata, and the backend's own response carries only the zone ID.
// Coordinates are the text that was written, for the reason given on
// UpdateComputeZoneOptions.
type ComputeZoneUpdate struct {
	ID           string `json:"id"`
	Type         string `json:"type,omitempty"`
	ContactEmail string `json:"contactEmail,omitempty"`
	ContactPIC   string `json:"contactPIC,omitempty"`
	City         string `json:"city,omitempty"`
	Country      string `json:"country,omitempty"`
	Region       string `json:"region,omitempty"`
	Latitude     string `json:"latitude,omitempty"`
	Longitude    string `json:"longitude,omitempty"`
	RawJSON      []byte `json:"-"`
}

// The accepted values named in a rejected compute zone type
const computeZoneTypeValues = `"datacenter" or "cloud provider"`

// Wire representation of a compute zone's contact. Sent and received as a
// whole object, so an omitted field is one the zone does not carry.
type computeZoneContact struct {
	Email *string `json:"email,omitempty"`
	Pic   *string `json:"pic,omitempty"`
}

// Wire representation of a compute zone's location. Coordinates are
// json.Number so the backend's own text is preserved byte for byte through a
// read-modify-write.
type computeZoneLocation struct {
	City      *string      `json:"city,omitempty"`
	Country   *string      `json:"country,omitempty"`
	Region    *string      `json:"region,omitempty"`
	Latitude  *json.Number `json:"latitude,omitempty"`
	Longitude *json.Number `json:"longitude,omitempty"`
}

// Wire representation of the update request body and of the stored zone it is
// merged over. The generated model is bypassed on both sides because it decodes
// coordinates as float32; see UpdateComputeZoneOptions.
type computeZoneDocument struct {
	ID       string               `json:"id"`
	Type     *string              `json:"type,omitempty"`
	Contact  *computeZoneContact  `json:"contact,omitempty"`
	Location *computeZoneLocation `json:"location,omitempty"`
}

// UpdateComputeZone updates a compute zone's type, contact, and location.
//
// The update is read-modify-write: PUT /v1/computezones replaces the zone's
// metadata rather than patching it, so the zone is read first and the supplied
// options are merged over what it already stores. Fields the caller did not
// name are echoed back unchanged, which makes setting a contact safe for a
// location the caller never mentioned. There is no ETag, so a concurrent edit
// between the read and the write is lost.
func (c *Client) UpdateComputeZone(ctx context.Context, zoneID string, opts UpdateComputeZoneOptions) (ComputeZoneUpdate, error) {
	ctx, cancel := c.requestContext(ctx)
	defer cancel()

	body, payload, err := c.buildComputeZoneUpdateRequest(ctx, zoneID, opts)
	if err != nil {
		return ComputeZoneUpdate{}, err
	}

	resp, err := c.api.PutV1ComputezonesWithBodyWithResponse(ctx, "application/json", bytes.NewReader(payload))
	if err != nil {
		return ComputeZoneUpdate{}, err
	}
	if resp.StatusCode() != http.StatusOK {
		return ComputeZoneUpdate{}, newAPIError(resp.StatusCode(), resp.Status(), resp.Body)
	}

	var data fleetapi.ModelsUpdateComputeZoneResponse
	if err := json.Unmarshal(resp.Body, &data); err != nil {
		return ComputeZoneUpdate{}, err
	}

	result := computeZoneUpdateResult(body)
	// The backend echoes the zone it wrote; fall back to the requested ID so
	// the result always names the zone even if that field is omitted.
	if id := stringValue(data.Id); id != "" {
		result.ID = id
	}
	result.RawJSON = append([]byte(nil), resp.Body...)

	return result, nil
}

// PreviewUpdateComputeZone builds the request UpdateComputeZone would send,
// without sending it. It still performs the read the merge depends on
// (getComputeZone), since the request body cannot be known without it, but
// issues no write. Both PreviewUpdateComputeZone and UpdateComputeZone build
// the body through buildComputeZoneUpdateRequest, so a preview can never
// disagree with the request that is actually issued.
func (c *Client) PreviewUpdateComputeZone(ctx context.Context, zoneID string, opts UpdateComputeZoneOptions) (RequestPreview, error) {
	ctx, cancel := c.requestContext(ctx)
	defer cancel()

	_, payload, err := c.buildComputeZoneUpdateRequest(ctx, zoneID, opts)
	if err != nil {
		return RequestPreview{}, err
	}

	req, err := fleetapi.NewPutV1ComputezonesRequestWithBody(c.generatedServerURL(), "application/json", bytes.NewReader(payload))
	if err != nil {
		return RequestPreview{}, err
	}

	return RequestPreview{Method: req.Method, URL: req.URL.String(), Body: payload}, nil
}

// Validates the options, reads the zone being updated, and merges the
// supplied fields into the document that will be sent. UpdateComputeZone and
// PreviewUpdateComputeZone both call this, so the executed request and the
// --dry-run preview of it are built by exactly the same code.
func (c *Client) buildComputeZoneUpdateRequest(ctx context.Context, zoneID string, opts UpdateComputeZoneOptions) (computeZoneDocument, []byte, error) {
	zoneID = strings.TrimSpace(zoneID)
	if zoneID == "" {
		return computeZoneDocument{}, nil, errors.New("compute zone ID is required")
	}
	if err := opts.Validate(); err != nil {
		return computeZoneDocument{}, nil, err
	}

	current, err := c.getComputeZone(ctx, zoneID)
	if err != nil {
		return computeZoneDocument{}, nil, err
	}

	body := buildComputeZoneUpdate(zoneID, current, opts)
	payload, err := json.Marshal(body)
	if err != nil {
		return computeZoneDocument{}, nil, err
	}

	return body, payload, nil
}

// Validate reports whether the options describe a request the API accepts.
// UpdateComputeZone calls it, and a caller can call it first to reject a bad
// request without opening a connection.
func (opts UpdateComputeZoneOptions) Validate() error {
	if opts.empty() {
		return errors.New("no compute zone changes were requested")
	}
	if opts.Type != nil {
		zoneType := strings.TrimSpace(*opts.Type)
		if !fleetapi.ModelsComputeZoneType(zoneType).Valid() {
			return invalidOption("type", "compute zone type", *opts.Type, computeZoneTypeValues)
		}
	}
	if err := validateCoordinate(opts.LocationLatitude, "latitude", 90); err != nil {
		return err
	}
	return validateCoordinate(opts.LocationLongitude, "longitude", 180)
}

// Reports whether the options ask for nothing at all. An update with no fields
// is rejected rather than sent: the request would rewrite the zone with what it
// already holds, which reads as a successful edit having changed nothing.
func (opts UpdateComputeZoneOptions) empty() bool {
	return opts.Type == nil &&
		opts.ContactEmail == nil && opts.ContactPIC == nil &&
		opts.LocationCity == nil && opts.LocationCountry == nil &&
		opts.LocationRegion == nil &&
		opts.LocationLatitude == nil && opts.LocationLongitude == nil
}

// Checks a coordinate that was supplied as text. An empty value clears the
// coordinate and so has nothing to check.
func validateCoordinate(value *string, option string, limit float64) error {
	if value == nil {
		return nil
	}
	text := strings.TrimSpace(*value)
	if text == "" {
		return nil
	}

	degrees, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return invalidOption(option, "compute zone "+option, *value, "a decimal number")
	}
	if math.IsNaN(degrees) || math.IsInf(degrees, 0) {
		return invalidOption(option, "compute zone "+option, *value, "a finite decimal number")
	}
	if degrees < -limit || degrees > limit {
		return invalidOption(option, "compute zone "+option, *value,
			fmt.Sprintf("a value between -%g and %g", limit, limit))
	}
	return nil
}

// Reads the zone being updated so the supplied fields can be merged over what
// it already stores. There is no read-one endpoint, so this filters the list to
// the requested ID; the basic view is used because it carries every field the
// update writes and none of the metrics it does not.
func (c *Client) getComputeZone(ctx context.Context, zoneID string) (computeZoneDocument, error) {
	page, err := c.ListComputeZones(ctx, ListComputeZonesOptions{
		View:    ComputeZoneViewBasic,
		ZoneIDs: []string{zoneID},
	})
	if err != nil {
		return computeZoneDocument{}, err
	}

	// Decoded from the raw payload rather than from page.ComputeZones: the
	// domain type drops the contact and holds coordinates as float32.
	var envelope struct {
		ComputeZones []computeZoneDocument `json:"computezones"`
	}
	if err := json.Unmarshal(page.RawJSON, &envelope); err != nil {
		return computeZoneDocument{}, err
	}

	for _, zone := range envelope.ComputeZones {
		if zone.ID == zoneID {
			return zone, nil
		}
	}

	return computeZoneDocument{}, fmt.Errorf("compute zone %q not found", zoneID)
}

// Merges the supplied options over the stored zone. Both the executed request
// and any caller inspecting what would be sent go through here, so the two can
// never disagree.
func buildComputeZoneUpdate(zoneID string, current computeZoneDocument, opts UpdateComputeZoneOptions) computeZoneDocument {
	body := computeZoneDocument{
		ID:       zoneID,
		Type:     current.Type,
		Contact:  current.Contact,
		Location: current.Location,
	}

	if opts.Type != nil {
		body.Type = optionalString(strings.TrimSpace(*opts.Type))
	}

	contact := computeZoneContact{}
	if body.Contact != nil {
		contact = *body.Contact
	}
	applyStringField(&contact.Email, opts.ContactEmail)
	applyStringField(&contact.Pic, opts.ContactPIC)
	body.Contact = optionalContact(contact)

	location := computeZoneLocation{}
	if body.Location != nil {
		location = *body.Location
	}
	applyStringField(&location.City, opts.LocationCity)
	applyStringField(&location.Country, opts.LocationCountry)
	applyStringField(&location.Region, opts.LocationRegion)
	applyCoordinateField(&location.Latitude, opts.LocationLatitude)
	applyCoordinateField(&location.Longitude, opts.LocationLongitude)
	body.Location = optionalLocation(location)

	return body
}

// Applies one supplied string over a stored one. A nil option keeps the stored
// value; an empty option clears the field, which the whole-object replace
// expresses by omitting it.
func applyStringField(field **string, value *string) {
	if value == nil {
		return
	}
	*field = optionalString(strings.TrimSpace(*value))
}

// Applies one supplied coordinate over a stored one. The caller's text is
// normalized into JSON number syntax, and an empty option clears the coordinate.
func applyCoordinateField(field **json.Number, value *string) {
	if value == nil {
		return
	}
	text := strings.TrimSpace(*value)
	if text == "" {
		*field = nil
		return
	}
	degrees, _ := strconv.ParseFloat(text, 64)
	number := json.Number(strconv.FormatFloat(degrees, 'g', -1, 64))
	*field = &number
}

// Omits a contact that carries nothing, so an update never sends an empty
// object where the zone has no contact at all.
func optionalContact(contact computeZoneContact) *computeZoneContact {
	if contact.Email == nil && contact.Pic == nil {
		return nil
	}
	return &contact
}

// Omits a location that carries nothing, for the same reason as optionalContact
func optionalLocation(location computeZoneLocation) *computeZoneLocation {
	if location.City == nil && location.Country == nil && location.Region == nil &&
		location.Latitude == nil && location.Longitude == nil {
		return nil
	}
	return &location
}

// Flattens the document that was written into the reported result
func computeZoneUpdateResult(body computeZoneDocument) ComputeZoneUpdate {
	result := ComputeZoneUpdate{
		ID:   body.ID,
		Type: stringValue(body.Type),
	}
	if body.Contact != nil {
		result.ContactEmail = stringValue(body.Contact.Email)
		result.ContactPIC = stringValue(body.Contact.Pic)
	}
	if body.Location != nil {
		result.City = stringValue(body.Location.City)
		result.Country = stringValue(body.Location.Country)
		result.Region = stringValue(body.Location.Region)
		result.Latitude = numberValue(body.Location.Latitude)
		result.Longitude = numberValue(body.Location.Longitude)
	}
	return result
}

// Converts an optional JSON number into its text, empty when absent
func numberValue(value *json.Number) string {
	if value == nil {
		return ""
	}
	return string(*value)
}
