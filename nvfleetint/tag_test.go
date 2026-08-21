// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

// Verifies tag list request construction and decoding
func TestListTagsSendsParamsAndDecodes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/tags" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		query := r.URL.Query()
		if got := query.Get("prefix"); got != "gpu" {
			t.Fatalf("unexpected prefix: %q", got)
		}
		if got := query.Get("nodeGroupId"); got != "ng-1" {
			t.Fatalf("unexpected nodeGroupId: %q", got)
		}
		if query.Has("nodeUUID") || query.Has("computeZoneId") {
			t.Fatalf("did not expect other resource filters: %q", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tags":["gpu-health","gpu-burn"]}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	got, err := client.ListTags(context.Background(), TagListOptions{Prefix: "gpu", NodeGroupID: "ng-1"})
	if err != nil {
		t.Fatalf("list tags failed: %v", err)
	}
	if !slices.Equal(got.Tags, []string{"gpu-health", "gpu-burn"}) {
		t.Fatalf("unexpected tags: %#v", got.Tags)
	}
	if !strings.Contains(string(got.RawJSON), `"tags"`) {
		t.Fatalf("raw JSON not preserved: %q", string(got.RawJSON))
	}
}

// Verifies more than one resource filter is rejected before any request
func TestListTagsRejectsMultipleResourceFilters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("did not expect a request for invalid input")
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	_, err = client.ListTags(context.Background(), TagListOptions{NodeUUID: "node-1", ComputeZoneID: "cz-1"})
	if err == nil {
		t.Fatal("expected error for multiple resource filters")
	}
	if !strings.Contains(err.Error(), "at most one") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Verifies tag list API errors are structured
func TestListTagsReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad","details":"invalid prefix"}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	_, err = client.ListTags(context.Background(), TagListOptions{Prefix: "!"})
	if err == nil {
		t.Fatal("expected API error")
	}
	if !strings.Contains(err.Error(), "400") || !strings.Contains(err.Error(), "invalid prefix") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Verifies set node tags request construction and decoding
func TestSetNodeTagsSendsBodyAndDecodes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/v1/nodes/node-1/tags" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body failed: %v", err)
		}
		if got := strings.TrimSpace(string(body)); got != `{"tags":["gpu-health","burn_in"]}` {
			t.Fatalf("unexpected body: %s", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodeUUID":"node-1","tags":["burn_in","gpu-health"]}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	got, err := client.SetNodeTags(context.Background(), " node-1 ", SetNodeTagsOptions{
		Tags: []string{"gpu-health", " burn_in "},
	})
	if err != nil {
		t.Fatalf("set node tags failed: %v", err)
	}
	if got.NodeUUID != "node-1" {
		t.Fatalf("unexpected node UUID: %q", got.NodeUUID)
	}
	if !slices.Equal(got.Tags, []string{"burn_in", "gpu-health"}) {
		t.Fatalf("unexpected tags: %#v", got.Tags)
	}
	if !strings.Contains(string(got.RawJSON), `"nodeUUID"`) {
		t.Fatalf("raw JSON not preserved: %q", string(got.RawJSON))
	}
}

// Verifies clearing a node's tags sends an empty list rather than null
func TestSetNodeTagsClearsWithEmptyList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body failed: %v", err)
		}
		if got := strings.TrimSpace(string(body)); got != `{"tags":[]}` {
			t.Fatalf("unexpected body: %s", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodeUUID":"node-1","tags":[]}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	got, err := client.SetNodeTags(context.Background(), "node-1", SetNodeTagsOptions{})
	if err != nil {
		t.Fatalf("set node tags failed: %v", err)
	}
	if len(got.Tags) != 0 {
		t.Fatalf("unexpected tags: %#v", got.Tags)
	}
}

// Verifies the node UUID is echoed from the request when the backend omits it
func TestSetNodeTagsFallsBackToRequestedNodeUUID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tags":["gpu-health"]}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	got, err := client.SetNodeTags(context.Background(), "node-1", SetNodeTagsOptions{
		Tags: []string{"gpu-health"},
	})
	if err != nil {
		t.Fatalf("set node tags failed: %v", err)
	}
	if got.NodeUUID != "node-1" {
		t.Fatalf("unexpected node UUID: %q", got.NodeUUID)
	}
}

// Verifies invalid input is rejected before any request is made
func TestSetNodeTagsRejectsInvalidInput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("did not expect a request for invalid input")
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	cases := []struct {
		name     string
		nodeUUID string
		tags     []string
		want     string
	}{
		{name: "missing node UUID", nodeUUID: "  ", tags: []string{"gpu-health"}, want: "node UUID is required"},
		{name: "uppercase", nodeUUID: "node-1", tags: []string{"GPU"}, want: "lowercase letters"},
		{name: "space", nodeUUID: "node-1", tags: []string{"gpu health"}, want: "lowercase letters"},
		{name: "non-ascii", nodeUUID: "node-1", tags: []string{"gpu-héalth"}, want: "lowercase letters"},
		{name: "empty", nodeUUID: "node-1", tags: []string{"   "}, want: "must not be empty"},
		{name: "too long", nodeUUID: "node-1", tags: []string{strings.Repeat("a", MaxTagLength+1)}, want: "50-character maximum"},
		{name: "leading separator", nodeUUID: "node-1", tags: []string{"-gpu"}, want: "start and end"},
		{name: "trailing separator", nodeUUID: "node-1", tags: []string{"gpu_"}, want: "start and end"},
		{name: "consecutive separators", nodeUUID: "node-1", tags: []string{"gpu-_health"}, want: "cannot be consecutive"},
		{name: "reserved", nodeUUID: "node-1", tags: []string{"none"}, want: "reserved"},
		{name: "duplicate", nodeUUID: "node-1", tags: []string{"gpu", "gpu"}, want: "duplicate tag"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := client.SetNodeTags(context.Background(), testCase.nodeUUID, SetNodeTagsOptions{
				Tags: testCase.tags,
			})
			if err == nil {
				t.Fatal("expected error for invalid input")
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// Verifies a tag at the length limit is accepted
func TestSetNodeTagsAcceptsMaximumLengthTag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodeUUID":"node-1"}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	tag := strings.Repeat("a", MaxTagLength)
	result, err := client.SetNodeTags(context.Background(), "node-1", SetNodeTagsOptions{
		Tags: []string{tag},
	})
	if err != nil {
		t.Fatalf("set node tags failed: %v", err)
	}
	// The response omits tags, so the result falls back to what was requested.
	if len(result.Tags) != 1 || result.Tags[0] != tag {
		t.Fatalf("unexpected tags: %v", result.Tags)
	}
}

// Verifies set node tags API errors are structured
func TestSetNodeTagsReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found","details":"unknown node"}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	_, err = client.SetNodeTags(context.Background(), "node-1", SetNodeTagsOptions{Tags: []string{"gpu"}})
	if err == nil {
		t.Fatal("expected API error")
	}
	if !strings.Contains(err.Error(), "404") || !strings.Contains(err.Error(), "unknown node") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Verifies a failed write is not replayed: the retrying doer covers reads only,
// so a 500 on PUT must surface after exactly one attempt.
func TestSetNodeTagsDoesNotRetry(t *testing.T) {
	var attempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	if _, err := client.SetNodeTags(context.Background(), "node-1", SetNodeTagsOptions{
		Tags: []string{"gpu"},
	}); err == nil {
		t.Fatal("expected API error")
	}
	if attempts != 1 {
		t.Fatalf("expected exactly one attempt, got %d", attempts)
	}
}
