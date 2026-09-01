// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"context"
	"errors"
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
		_, _ = w.Write([]byte(`{"computezones":[{"id":"cz-1","name":"East","type":"datacenter","location":{"region":"us-east-1"},"nodesCount":7}],"hasMore":true,"page":2,"pageSize":50,"total":99}`))
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
	if zone.Location == nil || zone.Location.Region != "us-east-1" {
		t.Fatalf("unexpected location: %#v", zone.Location)
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

// computeZoneUpdateServer serves the read-modify-write pair, recording the
// request the update issued.
func computeZoneUpdateServer(t *testing.T, zone string, method *string, body *[]byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/computezones" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("unexpected auth header: %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			if got := r.URL.Query().Get("view"); got != "basic" {
				t.Errorf("unexpected view: %q", got)
			}
			if got := r.URL.Query()["computeZoneIds"]; !slices.Equal(got, []string{"cz-1"}) {
				t.Errorf("unexpected computeZoneIds: %#v", got)
			}
			_, _ = w.Write([]byte(`{"computezones":[` + zone + `],"page":0,"pageSize":20,"total":1}`))
			return
		}

		*method = r.Method
		read, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body failed: %v", err)
		}
		*body = read
		_, _ = w.Write([]byte(`{"id":"cz-1"}`))
	}))
}

// The stored zone the update tests merge over
const storedComputeZone = `{"id":"cz-1","name":"East","type":"datacenter",` +
	`"contact":{"email":"ops@example.com","pic":"Jane Doe"},` +
	`"location":{"city":"Baltimore","country":"United States","region":"us-east-1","latitude":39.04581234,"longitude":-76.64131111}}`

// Verifies that an update of one field echoes every other stored field back,
// and that coordinates survive the round trip byte for byte rather than being
// rewritten at the float32 precision of the generated model.
func TestUpdateComputeZoneMergesOverStoredZone(t *testing.T) {
	var method string
	var body []byte
	server := computeZoneUpdateServer(t, storedComputeZone, &method, &body)
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	email := "new@example.com"
	got, err := client.UpdateComputeZone(context.Background(), "cz-1", UpdateComputeZoneOptions{ContactEmail: &email})
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}

	if method != http.MethodPut {
		t.Fatalf("unexpected method: %q", method)
	}
	want := `{"id":"cz-1","type":"datacenter","contact":{"email":"new@example.com","pic":"Jane Doe"},` +
		`"location":{"city":"Baltimore","country":"United States","region":"us-east-1","latitude":39.04581234,"longitude":-76.64131111}}`
	if string(body) != want {
		t.Fatalf("unexpected body:\n got %s\nwant %s", body, want)
	}

	if got.ID != "cz-1" || got.Type != "datacenter" || got.ContactEmail != "new@example.com" || got.ContactPIC != "Jane Doe" {
		t.Fatalf("unexpected result: %#v", got)
	}
	if got.Latitude != "39.04581234" || got.Longitude != "-76.64131111" {
		t.Fatalf("unexpected coordinates: %#v", got)
	}
	if string(got.RawJSON) != `{"id":"cz-1"}` {
		t.Fatalf("unexpected raw JSON: %s", got.RawJSON)
	}
}

// Verifies that coordinate text accepted by strconv.ParseFloat is normalized
// before it is stored in json.Number and marshaled into the request body.
func TestUpdateComputeZoneNormalizesCoordinateTextBeforeMarshal(t *testing.T) {
	cases := []struct {
		name string
		lat  string
		lon  string
		want string
	}{
		{
			name: "leading plus and leading decimal point",
			lat:  "+39.0",
			lon:  ".5",
			want: `{"id":"cz-1","type":"datacenter","contact":{"email":"ops@example.com","pic":"Jane Doe"},` +
				`"location":{"city":"Baltimore","country":"United States","region":"us-east-1","latitude":39,"longitude":0.5}}`,
		},
		{
			name: "trailing decimal point",
			lat:  "1.",
			lon:  "-1.",
			want: `{"id":"cz-1","type":"datacenter","contact":{"email":"ops@example.com","pic":"Jane Doe"},` +
				`"location":{"city":"Baltimore","country":"United States","region":"us-east-1","latitude":1,"longitude":-1}}`,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var method string
			var body []byte
			server := computeZoneUpdateServer(t, storedComputeZone, &method, &body)
			defer server.Close()

			client, err := NewClient(server.URL, "test-key")
			if err != nil {
				t.Fatalf("new client failed: %v", err)
			}

			if _, err := client.UpdateComputeZone(context.Background(), "cz-1", UpdateComputeZoneOptions{
				LocationLatitude:  &testCase.lat,
				LocationLongitude: &testCase.lon,
			}); err != nil {
				t.Fatalf("update failed: %v", err)
			}

			if method != http.MethodPut {
				t.Fatalf("unexpected method: %q", method)
			}
			if string(body) != testCase.want {
				t.Fatalf("unexpected body:\n got %s\nwant %s", body, testCase.want)
			}
		})
	}
}

// Verifies that an empty value clears just that field
func TestUpdateComputeZoneClearsFields(t *testing.T) {
	var method string
	var body []byte
	server := computeZoneUpdateServer(t, storedComputeZone, &method, &body)
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	empty := ""
	got, err := client.UpdateComputeZone(context.Background(), "cz-1", UpdateComputeZoneOptions{
		LocationRegion:   &empty,
		LocationLatitude: &empty,
	})
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}

	want := `{"id":"cz-1","type":"datacenter","contact":{"email":"ops@example.com","pic":"Jane Doe"},` +
		`"location":{"city":"Baltimore","country":"United States","longitude":-76.64131111}}`
	if string(body) != want {
		t.Fatalf("unexpected body:\n got %s\nwant %s", body, want)
	}
	if got.Region != "" || got.Latitude != "" {
		t.Fatalf("cleared fields still reported: %#v", got)
	}
}

// Verifies that clearing the only contact field drops the whole object rather
// than sending an empty one
func TestUpdateComputeZoneDropsEmptyContact(t *testing.T) {
	var method string
	var body []byte
	server := computeZoneUpdateServer(t, `{"id":"cz-1","name":"East","contact":{"email":"ops@example.com"}}`, &method, &body)
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	empty := ""
	if _, err := client.UpdateComputeZone(context.Background(), "cz-1", UpdateComputeZoneOptions{ContactEmail: &empty}); err != nil {
		t.Fatalf("update failed: %v", err)
	}

	if string(body) != `{"id":"cz-1"}` {
		t.Fatalf("unexpected body: %s", body)
	}
}

// Verifies that a zone the backend does not return is reported rather than
// written blind
func TestUpdateComputeZoneRejectsMissingZone(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("unexpected write request: %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"computezones":[],"page":0,"pageSize":20,"total":0}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	zoneType := "datacenter"
	_, err = client.UpdateComputeZone(context.Background(), "cz-1", UpdateComputeZoneOptions{Type: &zoneType})
	if err == nil || !strings.Contains(err.Error(), `compute zone "cz-1" not found`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Verifies that a non-2xx write is reported as an APIError
func TestUpdateComputeZoneAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"computezones":[{"id":"cz-1"}],"page":0,"pageSize":20,"total":1}`))
			return
		}
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"forbidden","details":"no admin role"}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	zoneType := "datacenter"
	_, err = client.UpdateComputeZone(context.Background(), "cz-1", UpdateComputeZoneOptions{Type: &zoneType})
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("unexpected error: %v", err)
	}
	if apiErr.StatusCode != http.StatusForbidden || apiErr.Message != "forbidden" || apiErr.Details != "no admin role" {
		t.Fatalf("unexpected api error: %#v", apiErr)
	}
}

// Verifies option validation, which rejects a bad request before a connection
// is opened
func TestUpdateComputeZoneOptionsValidate(t *testing.T) {
	value := func(text string) *string { return &text }

	cases := []struct {
		name string
		opts UpdateComputeZoneOptions
		want string
	}{
		{"no changes", UpdateComputeZoneOptions{}, "no compute zone changes were requested"},
		{"bad type", UpdateComputeZoneOptions{Type: value("bogus")}, `invalid compute zone type "bogus"`},
		{"empty type", UpdateComputeZoneOptions{Type: value("")}, `invalid compute zone type ""`},
		{"latitude text", UpdateComputeZoneOptions{LocationLatitude: value("north")}, `invalid compute zone latitude "north"`},
		{"latitude nan", UpdateComputeZoneOptions{LocationLatitude: value("NaN")}, `invalid compute zone latitude "NaN"`},
		{"latitude range", UpdateComputeZoneOptions{LocationLatitude: value("90.1")}, `invalid compute zone latitude "90.1"`},
		{"longitude range", UpdateComputeZoneOptions{LocationLongitude: value("-180.5")}, `invalid compute zone longitude "-180.5"`},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			err := testCase.opts.Validate()
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}

	accepted := UpdateComputeZoneOptions{
		Type:              value("cloud provider"),
		LocationLatitude:  value("1."),
		LocationLongitude: value(".5"),
	}
	if err := accepted.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Verifies that a missing zone ID is rejected before any request
func TestUpdateComputeZoneRequiresZoneID(t *testing.T) {
	client, err := NewClient("https://api.example.com", "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	zoneType := "datacenter"
	_, err = client.UpdateComputeZone(context.Background(), "  ", UpdateComputeZoneOptions{Type: &zoneType})
	if err == nil || !strings.Contains(err.Error(), "compute zone ID is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}
