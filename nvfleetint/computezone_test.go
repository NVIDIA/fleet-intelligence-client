// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

// Verifies detail list request construction and decoding
func TestListComputeZonesDetailSendsAuthAndParams(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/computezones" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		query := r.URL.Query()
		if got := query.Get("view"); got != "detail" {
			t.Fatalf("unexpected view: %q", got)
		}
		if got := query.Get("includeMetrics"); got != "false" {
			t.Fatalf("unexpected includeMetrics: %q", got)
		}
		if got := query["computeZoneIds"]; !slices.Equal(got, []string{"cz-1", "cz-2"}) {
			t.Fatalf("unexpected computeZoneIds: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query.Get("page"); got != "2" {
			t.Fatalf("unexpected page: %q", got)
		}
		if got := query.Get("pageSize"); got != "50" {
			t.Fatalf("unexpected pageSize: %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"computezones":[{"id":"cz-1","name":"East","type":"datacenter","geoLocation":{"region":"us-east-1"},"nodesCount":7}],"hasMore":true,"page":2,"pageSize":50,"total":99}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	page := 2
	pageSize := 50
	includeMetrics := false
	got, err := client.ListComputeZones(context.Background(), ListComputeZonesOptions{
		View:           ComputeZoneViewDetail,
		IncludeMetrics: &includeMetrics,
		ZoneIDs:        []string{"cz-1", "cz-2"},
		Page:           &page,
		PageSize:       &pageSize,
	})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if !got.HasMore || got.Page != 2 || got.PageSize != 50 || got.Total != 99 {
		t.Fatalf("unexpected page metadata: %#v", got)
	}
	if len(got.ComputeZones) != 1 {
		t.Fatalf("unexpected zone count: %d", len(got.ComputeZones))
	}
	zone := got.ComputeZones[0]
	if zone.ID != "cz-1" || zone.Name != "East" || zone.Type != "datacenter" {
		t.Fatalf("unexpected zone: %#v", zone)
	}
	if zone.NodeCount == nil || *zone.NodeCount != 7 {
		t.Fatalf("unexpected node count: %#v", zone.NodeCount)
	}
	if zone.GeoLocation == nil || zone.GeoLocation.Region != "us-east-1" {
		t.Fatalf("unexpected geolocation: %#v", zone.GeoLocation)
	}
	if !strings.Contains(string(got.RawJSON), `"computezones"`) {
		t.Fatalf("raw JSON not preserved: %q", string(got.RawJSON))
	}
}

// Verifies basic view decoding
func TestListComputeZonesBasic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("view"); got != "basic" {
			t.Fatalf("unexpected view: %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"computezones":[{"id":"cz-1","name":"East"}],"hasMore":false,"page":0,"pageSize":20,"total":1}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	got, err := client.ListComputeZones(context.Background(), ListComputeZonesOptions{View: ComputeZoneViewBasic})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(got.ComputeZones) != 1 || got.ComputeZones[0].ID != "cz-1" || got.ComputeZones[0].Name != "East" {
		t.Fatalf("unexpected zones: %#v", got.ComputeZones)
	}
	if got.ComputeZones[0].NodeCount != nil {
		t.Fatalf("basic view should not set node count: %#v", got.ComputeZones[0].NodeCount)
	}
}

// Verifies updates preserve backend fields that were not explicitly changed
func TestUpdateComputeZonePreservesBackendFields(t *testing.T) {
	requests := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected auth header: %q", got)
		}

		switch r.Method {
		case http.MethodGet:
			if r.URL.Path != "/v1/computezones" {
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
			query := r.URL.Query()
			if got := query.Get("view"); got != "detail" {
				t.Fatalf("unexpected view: %q", got)
			}
			if got := query.Get("includeMetrics"); got != "false" {
				t.Fatalf("unexpected includeMetrics: %q", got)
			}
			if got := query["computeZoneIds"]; !slices.Equal(got, []string{"cz-1"}) {
				t.Fatalf("unexpected computeZoneIds: %#v", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"computezones":[{"id":"cz-1","name":"East","type":"datacenter","contact":{"email":"old@example.com","pic":"Ada"},"geoLocation":{"city":"Santa Clara","country":"US","region":"us-west","latitude":37.774929,"longitude":-122.419416}}],"hasMore":false,"page":0,"pageSize":20,"total":1}`))
		case http.MethodPut:
			if r.URL.Path != "/v1/computezones" {
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
			var body struct {
				ID      string `json:"id"`
				Type    string `json:"type"`
				Contact struct {
					Email string `json:"email"`
					PIC   string `json:"pic"`
				} `json:"contact"`
				GeoLocation struct {
					City      string      `json:"city"`
					Country   string      `json:"country"`
					Region    string      `json:"region"`
					Latitude  json.Number `json:"latitude"`
					Longitude json.Number `json:"longitude"`
				} `json:"geoLocation"`
			}
			data, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read body failed: %v", err)
			}
			if err := json.Unmarshal(data, &body); err != nil {
				t.Fatalf("decode body failed: %v\n%s", err, string(data))
			}
			if body.ID != "cz-1" || body.Type != "datacenter" {
				t.Fatalf("backend type or ID was not preserved: %#v", body)
			}
			if body.Contact.Email != "new@example.com" || body.Contact.PIC != "Ada" {
				t.Fatalf("contact fields were not merged: %#v", body.Contact)
			}
			if body.GeoLocation.City != "Austin" || body.GeoLocation.Country != "US" || body.GeoLocation.Region != "us-west" {
				t.Fatalf("geo fields were not merged: %#v", body.GeoLocation)
			}
			// Untouched coordinates must be echoed back byte-for-byte; decoding
			// them through the generated float32 model would rewrite them.
			if body.GeoLocation.Latitude.String() != "37.774929" || body.GeoLocation.Longitude.String() != "-122.419416" {
				t.Fatalf("coordinates lost precision: %s / %s", body.GeoLocation.Latitude, body.GeoLocation.Longitude)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"cz-1"}`))
		default:
			t.Fatalf("unexpected method: %s", r.Method)
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	email := "new@example.com"
	city := "Austin"
	got, err := client.UpdateComputeZone(context.Background(), UpdateComputeZoneOptions{
		ID:           "cz-1",
		ContactEmail: &email,
		GeoCity:      &city,
	})
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if got.ID != "cz-1" || !strings.Contains(string(got.RawJSON), `"id":"cz-1"`) {
		t.Fatalf("unexpected result: %#v raw %q", got, string(got.RawJSON))
	}
	if !slices.Equal(requests, []string{"GET /v1/computezones", "PUT /v1/computezones"}) {
		t.Fatalf("unexpected requests: %#v", requests)
	}
}

// Verifies coordinates are sent as the caller's own text and can be cleared
func TestUpdateComputeZoneCoordinates(t *testing.T) {
	tests := []struct {
		name      string
		latitude  string
		longitude string
		want      string
	}{
		{name: "set", latitude: "37.774929", longitude: "-122.419416", want: `"latitude":37.774929,"longitude":-122.419416`},
		{name: "high precision", latitude: "37.7749295361", longitude: "-122.4194155008", want: `"latitude":37.7749295361,"longitude":-122.4194155008`},
		{name: "clear", latitude: "", longitude: "", want: `"geoLocation":{"city":"Santa Clara"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"computezones":[{"id":"cz-1","geoLocation":{"city":"Santa Clara","latitude":1.5,"longitude":2.5}}],"hasMore":false,"page":0,"pageSize":20,"total":1}`))
					return
				}
				data, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatalf("read body failed: %v", err)
				}
				body = string(data)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":"cz-1"}`))
			}))
			defer server.Close()

			client, err := NewClient(server.URL, "test-key")
			if err != nil {
				t.Fatalf("new client failed: %v", err)
			}

			latitude := tt.latitude
			longitude := tt.longitude
			if _, err := client.UpdateComputeZone(context.Background(), UpdateComputeZoneOptions{
				ID:           "cz-1",
				GeoLatitude:  &latitude,
				GeoLongitude: &longitude,
			}); err != nil {
				t.Fatalf("update failed: %v", err)
			}
			if !strings.Contains(body, tt.want) {
				t.Fatalf("body missing %q: %s", tt.want, body)
			}
		})
	}
}

// Verifies out-of-range and non-numeric coordinates never reach the backend
func TestUpdateComputeZoneRejectsInvalidCoordinates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		t.Fatalf("invalid coordinates should not reach the backend: %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "latitude out of range", value: "1000", want: `invalid latitude "1000": must be between -90 and 90`},
		{name: "latitude not a number", value: "north", want: "expected a decimal number"},
		{name: "latitude quoted", value: `"37.4"`, want: "expected a decimal number"},
		{name: "latitude infinity", value: "Inf", want: "expected a decimal number"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value := tt.value
			if _, err := client.UpdateComputeZone(context.Background(), UpdateComputeZoneOptions{
				ID:          "cz-1",
				GeoLatitude: &value,
			}); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("unexpected error: got %v want %q", err, tt.want)
			}
		})
	}

	longitude := "-400"
	if _, err := client.UpdateComputeZone(context.Background(), UpdateComputeZoneOptions{
		ID:           "cz-1",
		GeoLongitude: &longitude,
	}); err == nil || !strings.Contains(err.Error(), `invalid longitude "-400": must be between -180 and 180`) {
		t.Fatalf("unexpected longitude error: %v", err)
	}
}

// Verifies dry-run previews apply the same backend-preserving merge
func TestPreviewUpdateComputeZone(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("preview should not write, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"computezones":[{"id":"cz-1","type":"datacenter","contact":{"email":"old@example.com","pic":"Ada"}}],"hasMore":false,"page":0,"pageSize":20,"total":1}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL+"/api", "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	email := "new@example.com"
	preview, err := client.PreviewUpdateComputeZone(context.Background(), UpdateComputeZoneOptions{
		ID:           "cz-1",
		ContactEmail: &email,
	})
	if err != nil {
		t.Fatalf("preview failed: %v", err)
	}
	if preview.Method != http.MethodPut || preview.URL != server.URL+"/api/v1/computezones" {
		t.Fatalf("unexpected preview target: %#v", preview)
	}
	if !strings.Contains(string(preview.Body), `"email":"new@example.com"`) || !strings.Contains(string(preview.Body), `"pic":"Ada"`) {
		t.Fatalf("preview body did not preserve contact: %s", string(preview.Body))
	}
}

// Verifies API errors are structured
func TestListComputeZonesReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"Invalid filter parameters","details":"bad zone id"}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	_, err = client.ListComputeZones(context.Background(), ListComputeZonesOptions{})
	if err == nil {
		t.Fatal("expected API error")
	}
	if !strings.Contains(err.Error(), "400 Bad Request") || !strings.Contains(err.Error(), "bad zone id") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Verifies local option validation
func TestListComputeZonesRejectsInvalidOptions(t *testing.T) {
	client, err := NewClient("https://fleet.example.com", "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	includeMetrics := false
	tests := []struct {
		name string
		opts ListComputeZonesOptions
		want string
	}{
		{name: "view", opts: ListComputeZonesOptions{View: "wide"}, want: "invalid compute zone view"},
		{name: "basic include metrics", opts: ListComputeZonesOptions{View: ComputeZoneViewBasic, IncludeMetrics: &includeMetrics}, want: "basic compute zone view is incompatible with include metrics"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.ListComputeZones(context.Background(), tt.opts)
			if err == nil {
				t.Fatal("expected invalid options error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("unexpected error: got %v want %q", err, tt.want)
			}
		})
	}
}
