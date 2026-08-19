// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/NVIDIA/fleet-intelligence-client/internal/generated/fleetapi"
)

// Represents physical location metadata for a fleet resource.
// This retains the backend "geoLocation" vocabulary; the CLI surfaces it
// to users as "location".
type GeoLocation struct {
	City      string   `json:"city,omitempty"`
	Country   string   `json:"country,omitempty"`
	Region    string   `json:"region,omitempty"`
	Latitude  *float32 `json:"latitude,omitempty"`
	Longitude *float32 `json:"longitude,omitempty"`
}

// Reports whether value is a latitude the API accepts: a decimal number
// between -90 and 90. Coordinates are validated as text so a stored value can
// be echoed back to the backend verbatim.
func ValidateLatitude(value string) error {
	return validateCoordinate("latitude", value, 90)
}

// Reports whether value is a longitude the API accepts: a decimal number
// between -180 and 180. See ValidateLatitude on why coordinates stay text.
func ValidateLongitude(value string) error {
	return validateCoordinate("longitude", value, 180)
}

// Checks that value is a JSON number literal within the given symmetric range
func validateCoordinate(name, value string, limit float64) error {
	trimmed := strings.TrimSpace(value)
	invalid := fmt.Errorf("invalid %s %q: expected a decimal number", name, value)
	if trimmed == "" {
		return fmt.Errorf("%s cannot be empty", name)
	}
	// Reject anything that is not a bare JSON number before decoding, so
	// quoted values and literals like null never reach the backend.
	if first := trimmed[0]; first != '-' && (first < '0' || first > '9') {
		return invalid
	}
	var number json.Number
	if err := json.Unmarshal([]byte(trimmed), &number); err != nil {
		return invalid
	}
	parsed, err := number.Float64()
	if err != nil {
		return invalid
	}
	if parsed < -limit || parsed > limit {
		return fmt.Errorf("invalid %s %q: must be between %g and %g", name, value, -limit, limit)
	}

	return nil
}

// Represents a non-success API response
type APIError struct {
	StatusCode int
	Status     string
	Message    string
	Details    string
}

// Formats API failures with status and response details
func (e *APIError) Error() string {
	status := e.Status
	if strings.TrimSpace(status) == "" {
		status = http.StatusText(e.StatusCode)
	}
	if strings.TrimSpace(status) == "" {
		status = fmt.Sprintf("HTTP %d", e.StatusCode)
	}

	parts := []string{"request failed: " + status}
	if strings.TrimSpace(e.Message) != "" {
		parts = append(parts, e.Message)
	}
	if strings.TrimSpace(e.Details) != "" {
		parts = append(parts, e.Details)
	}

	return strings.Join(parts, ": ")
}

// Builds a structured error from a failed backend response
func newAPIError(statusCode int, status string, data []byte) error {
	apiErr := &APIError{
		StatusCode: statusCode,
		Status:     status,
	}

	var response fleetapi.ModelsErrorResponse
	if err := json.Unmarshal(data, &response); err == nil {
		apiErr.Message = stringValue(response.Error)
		apiErr.Details = stringValue(response.Details)
		if response.RequestStatus != nil {
			if apiErr.Message == "" {
				apiErr.Message = stringValue(response.RequestStatus.StatusCode)
			}
			if apiErr.Details == "" {
				apiErr.Details = stringValue(response.RequestStatus.StatusDescription)
			}
		}
	}
	if apiErr.Message == "" && len(data) > 0 {
		apiErr.Message = strings.TrimSpace(string(data))
	}

	return apiErr
}

// Maps generated location metadata into SDK values
func geoLocationFromGenerated(location *fleetapi.ModelsGeoLocation) *GeoLocation {
	if location == nil {
		return nil
	}
	return &GeoLocation{
		City:      stringValue(location.City),
		Country:   stringValue(location.Country),
		Region:    stringValue(location.Region),
		Latitude:  cloneFloat32(location.Latitude),
		Longitude: cloneFloat32(location.Longitude),
	}
}

// Converts optional strings into empty strings
func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// Converts optional string enum values into empty strings
func enumStringValue[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

// Converts optional integers into zero values
func intValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

// Converts optional booleans into false values
func boolValue(value *bool) bool {
	if value == nil {
		return false
	}
	return *value
}

// Copies optional integers without sharing pointers
func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	out := *value
	return &out
}

// Copies optional strings without sharing pointers
func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	out := *value
	return &out
}

// Copies optional JSON numbers without sharing pointers
func cloneJSONNumber(value *json.Number) *json.Number {
	if value == nil {
		return nil
	}
	out := *value
	return &out
}

// Copies optional float values without sharing pointers
func cloneFloat32(value *float32) *float32 {
	if value == nil {
		return nil
	}
	out := *value
	return &out
}

// Copies optional booleans without sharing pointers
func cloneBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	out := *value
	return &out
}

// Converts optional timestamps into RFC3339 strings without dropping fractional
// seconds, matching the string timestamps the other SDK models expose.
func timeValue(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.Format(time.RFC3339Nano)
}

// Copies optional string slices without sharing backing arrays
func cloneStringSlice(values *[]string) []string {
	if values == nil {
		return nil
	}
	return append([]string(nil), (*values)...)
}

// Converts a slice into an optional query parameter, omitting it when empty
func optionalStringSlice(values []string) *[]string {
	if len(values) == 0 {
		return nil
	}
	out := append([]string(nil), values...)
	return &out
}

// Converts a string into an optional query parameter, omitting it when blank
func optionalTrimmedString(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// Returns a pointer to a copied boolean value
func boolPointer(value bool) *bool {
	out := value
	return &out
}
