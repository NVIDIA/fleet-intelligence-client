// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

const xidBurstBody = `{"items":[{"burstId":"burst-1","nodeUuid":"node-1","hostname":"gpu-01","nodeGroup":"ng","nodeGroupId":"ng-1","computeZone":"cz","computeZoneId":"cz-1","startTime":"2026-05-01T00:00:00.123456789Z","endTime":"2026-05-01T00:05:00.987654321Z","burstDurationSeconds":300,"xidCount":2,"xidNumbers":[{"xidNumber":48,"mnemonic":"DBE","description":"Double Bit ECC"},{"xidNumber":94,"mnemonic":"CE"}],"deviceIds":{"0000:0f:00.0":[48,94]},"jobDisruption":true,"jobDisruptionDueToPlatformIssue":false,"category":"GPU","subcategory":"Memory","stickyXidsSuppressed":1,"suggestedActions":[{"action":"Drain the node","code":"DRAIN","persona":"tenant","type":"immediate"}]}],"page":0,"pageSize":20,"total":1}`

// Verifies XID burst list request construction and decoding
func TestListXIDBurstsSendsParamsAndDecodes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/xid/bursts" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		query := r.URL.Query()
		for _, want := range []struct{ key, value string }{
			{"timeMode", "relative"},
			{"window", "24h"},
			{"nodeUUID", "node-1"},
			{"hostnameSearch", "gpu-0"},
			{"jobDisruption", "true"},
			{"jobDisruptionDueToPlatformIssue", "false"},
			{"sortBy", "startTime"},
			{"sortOrder", "desc"},
			{"page", "1"},
			{"pageSize", "20"},
			{"tenantActionSearch", "DRAIN"},
			{"dcAdminInvestigationSearch", "LOGS"},
		} {
			if got := query.Get(want.key); got != want.value {
				t.Fatalf("unexpected %s: %q", want.key, got)
			}
		}
		for _, want := range []struct {
			key    string
			values []string
		}{
			{"xidNumbers", []string{"48", "94"}},
			{"nodeGroupIds", []string{"ng-1"}},
			{"computeZoneIds", []string{"cz-1"}},
			{"categories", []string{"GPU"}},
			{"tenantActions", []string{"DRAIN", "RESET"}},
		} {
			if got := query[want.key]; !reflect.DeepEqual(got, want.values) {
				t.Fatalf("unexpected %s: %#v", want.key, got)
			}
		}
		if query.Has("startTime") || query.Has("endTime") {
			t.Fatalf("did not expect start/end in relative mode: %q", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(xidBurstBody))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	page := 1
	pageSize := 20
	jobDisruption := true
	platformIssue := false
	result, err := client.ListXIDBursts(context.Background(), ListXIDBurstsOptions{
		Window:                          "24h",
		NodeUUID:                        "node-1",
		NodeGroupIDs:                    []string{"ng-1"},
		ComputeZoneIDs:                  []string{"cz-1"},
		JobDisruption:                   &jobDisruption,
		JobDisruptionDueToPlatformIssue: &platformIssue,
		XIDNumbers:                      []int{48, 94},
		HostnameSearch:                  "gpu-0",
		TenantActionSearch:              "DRAIN",
		DCAdminInvestigationSearch:      "LOGS",
		Categories:                      []string{"GPU"},
		TenantActions:                   []string{"DRAIN", "RESET"},
		SortBy:                          XIDBurstSortByStartTime,
		SortOrder:                       XIDBurstOrderDesc,
		Page:                            &page,
		PageSize:                        &pageSize,
	})
	if err != nil {
		t.Fatalf("list XID bursts failed: %v", err)
	}

	if len(result.Bursts) != 1 {
		t.Fatalf("unexpected bursts: %#v", result.Bursts)
	}
	burst := result.Bursts[0]
	if burst.BurstID != "burst-1" || burst.NodeUUID != "node-1" || burst.Hostname != "gpu-01" {
		t.Fatalf("unexpected burst identity: %#v", burst)
	}
	if burst.StartTime != "2026-05-01T00:00:00.123456789Z" || burst.EndTime != "2026-05-01T00:05:00.987654321Z" {
		t.Fatalf("unexpected burst times: %#v", burst)
	}
	if burst.JobDisruption == nil || !*burst.JobDisruption {
		t.Fatalf("unexpected jobDisruption: %#v", burst.JobDisruption)
	}
	if burst.JobDisruptionDueToPlatformIssue == nil || *burst.JobDisruptionDueToPlatformIssue {
		t.Fatalf("unexpected platform disruption: %#v", burst.JobDisruptionDueToPlatformIssue)
	}
	if len(burst.XIDNumbers) != 2 || burst.XIDNumbers[0].XIDNumber == nil || *burst.XIDNumbers[0].XIDNumber != 48 ||
		burst.XIDNumbers[0].Mnemonic != "DBE" || burst.XIDNumbers[0].Description != "Double Bit ECC" {
		t.Fatalf("unexpected XID numbers: %#v", burst.XIDNumbers)
	}
	if !reflect.DeepEqual(burst.DeviceIDs, map[string][]int{"0000:0f:00.0": {48, 94}}) {
		t.Fatalf("unexpected device IDs: %#v", burst.DeviceIDs)
	}
	if len(burst.SuggestedActions) != 1 || burst.SuggestedActions[0].Code != "DRAIN" ||
		burst.SuggestedActions[0].Persona != "tenant" || burst.SuggestedActions[0].Type != "immediate" {
		t.Fatalf("unexpected suggested actions: %#v", burst.SuggestedActions)
	}
	if result.Page != 0 || result.PageSize != 20 || result.Total != 1 {
		t.Fatalf("unexpected pagination: %#v", result)
	}
	if !strings.Contains(string(result.RawJSON), `"burstId"`) {
		t.Fatalf("raw JSON not preserved: %q", string(result.RawJSON))
	}
}

// Verifies an absolute range sets absolute mode and omits the window
func TestListXIDBurstsAbsoluteRange(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if got := query.Get("timeMode"); got != "absolute" {
			t.Fatalf("unexpected timeMode: %q", got)
		}
		if got := query.Get("startTime"); got != "2026-05-01T00:00:00Z" {
			t.Fatalf("unexpected startTime: %q", got)
		}
		if got := query.Get("endTime"); got != "2026-05-08T00:00:00Z" {
			t.Fatalf("unexpected endTime: %q", got)
		}
		if query.Has("window") {
			t.Fatalf("did not expect window in absolute mode: %q", r.URL.RawQuery)
		}
		// Filters the caller left unset must not be sent at all.
		for _, key := range []string{"jobDisruption", "xidNumbers", "categories", "sortBy", "hostnameSearch"} {
			if query.Has(key) {
				t.Fatalf("did not expect %s: %q", key, r.URL.RawQuery)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[],"page":0,"pageSize":20,"total":0}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	if _, err := client.ListXIDBursts(context.Background(), ListXIDBurstsOptions{
		StartTime: "2026-05-01T00:00:00Z",
		EndTime:   "2026-05-08T00:00:00Z",
	}); err != nil {
		t.Fatalf("list XID bursts failed: %v", err)
	}
}

// Verifies invalid list options are rejected before any request
func TestListXIDBurstsValidation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("did not expect a request for invalid input")
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	tests := []struct {
		name string
		opts ListXIDBurstsOptions
		want string
	}{
		{"no time range", ListXIDBurstsOptions{}, "a time range is required"},
		{"window with start", ListXIDBurstsOptions{Window: "24h", StartTime: "2026-05-01T00:00:00Z"}, "window cannot be combined"},
		{"malformed start", ListXIDBurstsOptions{StartTime: "yesterday", EndTime: "2026-05-08T00:00:00Z"}, "start time must be RFC3339"},
		{"bad sort", ListXIDBurstsOptions{Window: "24h", SortBy: "burstId"}, "invalid XID burst sort"},
		{"bad order", ListXIDBurstsOptions{Window: "24h", SortOrder: "sideways"}, "invalid XID burst sort order"},
		{"negative XID", ListXIDBurstsOptions{Window: "24h", XIDNumbers: []int{-1}}, "invalid XID number"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := client.ListXIDBursts(context.Background(), tt.opts); err == nil {
				t.Fatal("expected error")
			} else if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// Verifies XID burst list API errors are structured
func TestListXIDBurstsReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"forbidden","details":"NCP-only filter requested by tenant"}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	_, err = client.ListXIDBursts(context.Background(), ListXIDBurstsOptions{Window: "24h"})
	if err == nil {
		t.Fatal("expected API error")
	}
	if !strings.Contains(err.Error(), "403") || !strings.Contains(err.Error(), "NCP-only filter") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Verifies XID burst describe request construction and decoding
func TestDescribeXIDBurstSendsRequestAndDecodes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/xid/bursts/burst-1" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"burstId":"burst-1","nodeUuid":"node-1","hostname":"gpu-01","startTime":"2026-05-01T00:00:00Z","endTime":"2026-05-01T00:05:00Z","burstDurationSeconds":300,"xidCount":1,"xidNumbers":[{"xidNumber":48,"mnemonic":"DBE"}],"deviceIds":{"0000:0f:00.0":[48]},"jobDisruption":true,"suggestedActions":[{"action":"Drain the node","code":"DRAIN","persona":"tenant","type":"immediate"}]}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	burst, err := client.DescribeXIDBurst(context.Background(), "burst-1")
	if err != nil {
		t.Fatalf("describe XID burst failed: %v", err)
	}
	if burst.BurstID != "burst-1" || burst.Hostname != "gpu-01" || burst.StartTime != "2026-05-01T00:00:00Z" {
		t.Fatalf("unexpected burst: %#v", burst)
	}
	if burst.JobDisruption == nil || !*burst.JobDisruption {
		t.Fatalf("unexpected jobDisruption: %#v", burst.JobDisruption)
	}
	// A field the backend omits for this persona stays nil rather than false.
	if burst.JobDisruptionDueToPlatformIssue != nil {
		t.Fatalf("expected omitted platform disruption, got %#v", burst.JobDisruptionDueToPlatformIssue)
	}
	if len(burst.SuggestedActions) != 1 || burst.SuggestedActions[0].Action != "Drain the node" {
		t.Fatalf("unexpected suggested actions: %#v", burst.SuggestedActions)
	}
	if !strings.Contains(string(burst.RawJSON), `"deviceIds"`) {
		t.Fatalf("raw JSON not preserved: %q", string(burst.RawJSON))
	}
}

// Verifies describe requires a burst ID and reports API errors
func TestDescribeXIDBurstErrors(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found","details":"finalized burst not found"}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	if _, err := client.DescribeXIDBurst(context.Background(), "   "); err == nil {
		t.Fatal("expected error for empty burst ID")
	} else if !strings.Contains(err.Error(), "burst ID is required") {
		t.Fatalf("unexpected error: %v", err)
	}
	if requests != 0 {
		t.Fatalf("expected no requests for invalid input, got %d", requests)
	}

	_, err = client.DescribeXIDBurst(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected API error")
	}
	if !strings.Contains(err.Error(), "404") || !strings.Contains(err.Error(), "finalized burst not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}
