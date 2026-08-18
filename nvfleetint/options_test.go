// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A node options payload exercising both the string and nested-object forms.
const nodeOptionsBody = `{"filters":{"fields":[` +
	`{"name":"computeZones","options":[{"id":"cz-1","value":"East","options":[{"id":"ng-1","value":"Training"}]}]},` +
	`{"name":"gpuTypes","options":["NVIDIA-H100"]}` +
	`]},"sorting":{"fields":["hostname","nodeGroup"],"orders":["asc","desc"],"defaults":{"field":"hostname","order":"asc"}}}`

// Verifies node filter options send the agent type and decode nested options.
func TestGetNodeFilterOptions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/nodes/options" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("agentType"); got != "oob" {
			t.Fatalf("unexpected agentType: %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(nodeOptionsBody))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}
	options, err := client.GetNodeFilterOptions(context.Background(), NodeAgentTypeOOB)
	if err != nil {
		t.Fatalf("get node filter options failed: %v", err)
	}

	if len(options.Filters.Fields) != 2 {
		t.Fatalf("unexpected fields: %#v", options.Filters.Fields)
	}
	zones := options.Filters.Fields[0]
	if zones.Name != "computeZones" || len(zones.Options) != 1 {
		t.Fatalf("unexpected compute zone field: %#v", zones)
	}
	if zones.Options[0].ID != "cz-1" || zones.Options[0].Value != "East" {
		t.Fatalf("unexpected compute zone option: %#v", zones.Options[0])
	}
	nested := zones.Options[0].Options
	if len(nested) != 1 || nested[0].ID != "ng-1" || nested[0].Value != "Training" {
		t.Fatalf("unexpected nested options: %#v", nested)
	}
	gpuTypes := options.Filters.Fields[1]
	if len(gpuTypes.Options) != 1 || gpuTypes.Options[0].ID != "" || gpuTypes.Options[0].Value != "NVIDIA-H100" {
		t.Fatalf("unexpected gpu type options: %#v", gpuTypes.Options)
	}
	if options.Sorting.Defaults.Field != "hostname" || options.Sorting.Defaults.Order != "asc" {
		t.Fatalf("unexpected sorting defaults: %#v", options.Sorting.Defaults)
	}
	if string(options.RawJSON) != nodeOptionsBody {
		t.Fatalf("raw JSON not preserved: %s", options.RawJSON)
	}
}

// Verifies an empty agent type omits the query parameter entirely.
func TestGetNodeFilterOptionsOmitsEmptyAgentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := r.URL.Query()["agentType"]; ok {
			t.Fatalf("agentType should be omitted, got %q", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(nodeOptionsBody))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}
	if _, err := client.GetNodeFilterOptions(context.Background(), ""); err != nil {
		t.Fatalf("get node filter options failed: %v", err)
	}
}

// Verifies an unsupported agent type is rejected before any request is made.
func TestGetNodeFilterOptionsRejectsAgentType(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests++
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}
	_, err = client.GetNodeFilterOptions(context.Background(), "bmc")
	if err == nil || !strings.Contains(err.Error(), "invalid node agent type") {
		t.Fatalf("unexpected error: %v", err)
	}
	if requests != 0 {
		t.Fatalf("expected no requests, got %d", requests)
	}
}

// Verifies node group filter options decode from the shared envelope.
func TestGetNodeGroupFilterOptions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/nodegroups/options" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"filters":{"fields":[{"name":"healthStatuses","options":["Healthy"]}]},"sorting":{"fields":["health"],"orders":["asc"],"defaults":{"field":"health","order":"desc"}}}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}
	options, err := client.GetNodeGroupFilterOptions(context.Background())
	if err != nil {
		t.Fatalf("get node group filter options failed: %v", err)
	}
	if len(options.Filters.Fields) != 1 || options.Filters.Fields[0].Options[0].Value != "Healthy" {
		t.Fatalf("unexpected fields: %#v", options.Filters.Fields)
	}
	if len(options.RawJSON) == 0 {
		t.Fatal("raw JSON not preserved")
	}
}

// Verifies a non-2xx options response surfaces as an APIError.
func TestFilterOptionsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"denied"}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}
	if _, err := client.GetNodeGroupFilterOptions(context.Background()); err == nil {
		t.Fatal("expected an error")
	} else if !strings.Contains(err.Error(), "403") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Verifies a malformed options body reports a decode error.
func TestFilterOptionsDecodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"filters":{"fields":[{"name":"gpuTypes","options":[12]}]}}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}
	_, err = client.GetNodeGroupFilterOptions(context.Background())
	if err == nil || !strings.Contains(err.Error(), "must be a string or object") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Verifies marshaling restores the string, object, and nested object forms.
func TestFilterOptionMarshalRoundTrip(t *testing.T) {
	for _, want := range []string{
		`"NVIDIA-H100"`,
		`{"id":"cz-1","value":"East"}`,
		`{"id":"cz-1","value":"East","options":[{"id":"ng-1","value":"Training"}]}`,
	} {
		var option FilterOption
		if err := json.Unmarshal([]byte(want), &option); err != nil {
			t.Fatalf("unmarshal %s failed: %v", want, err)
		}
		got, err := json.Marshal(option)
		if err != nil {
			t.Fatalf("marshal %s failed: %v", want, err)
		}
		if string(got) != want {
			t.Fatalf("round trip changed %s into %s", want, got)
		}
	}
}
