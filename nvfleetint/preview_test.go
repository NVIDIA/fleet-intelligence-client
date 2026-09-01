// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Verifies that PreviewUpdateComputeZone reports the same method, URL, and
// body that UpdateComputeZone would send, and that it issues no PUT.
func TestPreviewUpdateComputeZoneMatchesIssuedRequest(t *testing.T) {
	var method string
	var body []byte
	server := computeZoneUpdateServer(t, storedComputeZone, &method, &body)
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	email := "new@example.com"
	preview, err := client.PreviewUpdateComputeZone(context.Background(), "cz-1", UpdateComputeZoneOptions{ContactEmail: &email})
	if err != nil {
		t.Fatalf("preview failed: %v", err)
	}

	// The read-modify-write merge still needs to read the zone, but the
	// server-observed method stays empty: only GET reaches computeZoneUpdateServer's
	// GET branch, and the PUT branch is never hit.
	if method != "" {
		t.Fatalf("preview issued a write: method=%q body=%s", method, body)
	}

	if preview.Method != http.MethodPut {
		t.Fatalf("unexpected preview method: %q", preview.Method)
	}
	if preview.URL != server.URL+"/v1/computezones" {
		t.Fatalf("unexpected preview URL: %q", preview.URL)
	}

	want := `{"id":"cz-1","type":"datacenter","contact":{"email":"new@example.com","pic":"Jane Doe"},` +
		`"location":{"city":"Baltimore","country":"United States","region":"us-east-1","latitude":39.04581234,"longitude":-76.64131111}}`
	if string(preview.Body) != want {
		t.Fatalf("unexpected preview body:\n got %s\nwant %s", preview.Body, want)
	}

	// Now issue the real request and confirm it matches the preview exactly.
	got, err := client.UpdateComputeZone(context.Background(), "cz-1", UpdateComputeZoneOptions{ContactEmail: &email})
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if method != http.MethodPut {
		t.Fatalf("unexpected method: %q", method)
	}
	if string(body) != string(preview.Body) {
		t.Fatalf("issued body disagrees with preview:\n issued  %s\n preview %s", body, preview.Body)
	}
	if got.ContactEmail != "new@example.com" {
		t.Fatalf("unexpected result: %#v", got)
	}
}

// Verifies that a preview surfaces the same not-found error UpdateComputeZone
// would return when the merge cannot find the zone being updated.
func TestPreviewUpdateComputeZoneRejectsMissingZone(t *testing.T) {
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
	if _, err := client.PreviewUpdateComputeZone(context.Background(), "cz-1", UpdateComputeZoneOptions{Type: &zoneType}); err == nil ||
		!strings.Contains(err.Error(), `compute zone "cz-1" not found`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Verifies that PreviewSetNodeTags reports the same method, URL, and body
// that SetNodeTags would send, and that it issues no request at all.
func TestPreviewSetNodeTagsMatchesIssuedRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("preview issued a request")
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	preview, err := client.PreviewSetNodeTags(context.Background(), " node-1 ", SetNodeTagsOptions{
		Tags: []string{"gpu-health", " burn_in "},
	})
	if err != nil {
		t.Fatalf("preview failed: %v", err)
	}

	if preview.Method != http.MethodPut {
		t.Fatalf("unexpected preview method: %q", preview.Method)
	}
	if preview.URL != server.URL+"/v1/nodes/node-1/tags" {
		t.Fatalf("unexpected preview URL: %q", preview.URL)
	}
	if string(preview.Body) != `{"tags":["gpu-health","burn_in"]}` {
		t.Fatalf("unexpected preview body: %s", preview.Body)
	}
}

// Verifies a preview surfaces the same tag validation SetNodeTags applies
func TestPreviewSetNodeTagsRejectsInvalidTag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("preview issued a request")
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	if _, err := client.PreviewSetNodeTags(context.Background(), "node-1", SetNodeTagsOptions{Tags: []string{"Not Valid"}}); err == nil {
		t.Fatal("expected an error for an invalid tag")
	}
}
