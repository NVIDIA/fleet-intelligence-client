package fleetintelligence

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

// Verifies status, message, and detail formatting
func TestAPIErrorFormatting(t *testing.T) {
	tests := []struct {
		name string
		err  *APIError
		want string
	}{
		{name: "status text", err: &APIError{StatusCode: http.StatusBadRequest, Message: "bad request", Details: "bad filter"}, want: "request failed: Bad Request: bad request: bad filter"},
		{name: "explicit status", err: &APIError{StatusCode: http.StatusTeapot, Status: "418 I'm a teapot"}, want: "request failed: 418 I'm a teapot"},
		{name: "unknown status", err: &APIError{StatusCode: 599}, want: "request failed: HTTP 599"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Fatalf("unexpected error: got %q want %q", got, tt.want)
			}
		})
	}
}

// Verifies standard backend error payloads
func TestNewAPIErrorParsesBackendError(t *testing.T) {
	err := newAPIError(http.StatusBadRequest, "400 Bad Request", []byte(`{"error":"Invalid filter parameters","details":"bad zone id"}`))

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T", err)
	}
	if apiErr.StatusCode != http.StatusBadRequest || apiErr.Message != "Invalid filter parameters" || apiErr.Details != "bad zone id" {
		t.Fatalf("unexpected API error: %#v", apiErr)
	}
}

// Verifies KAS-shaped error payloads
func TestNewAPIErrorParsesKASRequestStatus(t *testing.T) {
	err := newAPIError(http.StatusUnauthorized, "401 Unauthorized", []byte(`{"requestStatus":{"statusCode":"Unauthorized","statusDescription":"token expired"}}`))

	if !strings.Contains(err.Error(), "Unauthorized") || !strings.Contains(err.Error(), "token expired") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Verifies non-JSON error payloads are still useful
func TestNewAPIErrorFallsBackToRawBody(t *testing.T) {
	err := newAPIError(http.StatusServiceUnavailable, "503 Service Unavailable", []byte(" backend unavailable\n"))

	if !strings.Contains(err.Error(), "503 Service Unavailable") || !strings.Contains(err.Error(), "backend unavailable") {
		t.Fatalf("unexpected error: %v", err)
	}
}
