package fleetintelligence

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"gitlab-master.nvidia.com/gpu-health/fleet-intelligence-client-go/internal/generated/fleetapi"
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
	View     ComputeZoneView
	ZoneIDs  []string
	Page     *int
	PageSize *int
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

// Represents a compute zone
type ComputeZone struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Type        string       `json:"type,omitempty"`
	GeoLocation *GeoLocation `json:"geoLocation,omitempty"`
	NodeCount   *int         `json:"nodeCount,omitempty"`
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
		ID:          stringValue(zone.Id),
		Name:        stringValue(zone.Name),
		Type:        enumStringValue(zone.Type),
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
		GeoLocation: geoLocationFromGenerated(zone.GeoLocation),
	}
}
