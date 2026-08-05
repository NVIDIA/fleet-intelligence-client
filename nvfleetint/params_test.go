// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// Verifies identifier validation, including the dot segments that survive
// percent-escaping and would otherwise re-target the request
func TestValidateResourceID(t *testing.T) {
	cases := []struct {
		name    string
		value   string
		want    string
		wantErr string
	}{
		{name: "uuid", value: "1e9c0d2a-0000-4a1b-9c3d-000000000001", want: "1e9c0d2a-0000-4a1b-9c3d-000000000001"},
		{name: "trims surrounding space", value: "  node-1  ", want: "node-1"},
		{name: "dotted identifier is fine", value: "node.example.1", want: "node.example.1"},
		{name: "empty", value: "", wantErr: "node UUID is required"},
		{name: "whitespace only", value: "   ", wantErr: "node UUID is required"},
		{name: "parent dot segment", value: "..", wantErr: "different API path"},
		{name: "padded parent dot segment", value: " .. ", wantErr: "different API path"},
		{name: "current dot segment", value: ".", wantErr: "different API path"},
		{name: "forward slash", value: "../../v1/tags", wantErr: "single path segment"},
		{name: "backslash", value: `a\b`, wantErr: "single path segment"},
		{name: "control character", value: "node\x00id", wantErr: "control character"},
		{name: "newline", value: "node\nid", wantErr: "control character"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := ValidateResourceID("node UUID", testCase.value)
			if testCase.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got != testCase.want {
					t.Fatalf("got %q, want %q", got, testCase.want)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected an error, got %q", got)
			}
			if !strings.Contains(err.Error(), testCase.wantErr) {
				t.Fatalf("error %q does not mention %q", err, testCase.wantErr)
			}
		})
	}
}

// Verifies a rejected control character is not echoed back in the message
func TestValidateResourceIDDoesNotEchoControlCharacters(t *testing.T) {
	_, err := ValidateResourceID("node UUID", "node\x1b[31mid")
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.ContainsRune(err.Error(), 0x1b) {
		t.Fatalf("error echoed a control character: %q", err.Error())
	}
}

// Verifies paging bounds, including the unset case that defers to the backend
func TestValidatePagination(t *testing.T) {
	page := func(v int) *int { return &v }

	cases := []struct {
		name     string
		page     *int
		pageSize *int
		wantErr  string
	}{
		{name: "both unset"},
		{name: "first page", page: page(0), pageSize: page(50)},
		{name: "minimum page size", pageSize: page(MinPageSize)},
		{name: "maximum page size", pageSize: page(MaxPageSize)},
		{name: "negative page", page: page(-1), wantErr: "non-negative"},
		{name: "zero page size", pageSize: page(0), wantErr: "expected 1-100"},
		{name: "oversized page size", pageSize: page(MaxPageSize + 1), wantErr: "expected 1-100"},
		{name: "negative page size", pageSize: page(-5), wantErr: "expected 1-100"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			err := validatePagination(testCase.page, testCase.pageSize)
			switch {
			case testCase.wantErr == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case testCase.wantErr != "" && err == nil:
				t.Fatal("expected an error")
			case testCase.wantErr != "" && !strings.Contains(err.Error(), testCase.wantErr):
				t.Fatalf("error %q does not mention %q", err, testCase.wantErr)
			}
		})
	}
}

// Verifies no SDK call that interpolates an identifier into the URL path can be
// steered off its endpoint by a hostile identifier. Each case must fail before
// a request is issued, so the server counter stays at zero.
func TestPathParamsCannotRetargetRequests(t *testing.T) {
	var requests atomic.Int32
	var observed []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		observed = append(observed, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	calls := map[string]func(id string) error{
		"DescribeNode": func(id string) error {
			_, err := client.DescribeNode(context.Background(), id)
			return err
		},
		"NodeHealthHistory": func(id string) error {
			_, err := client.NodeHealthHistory(context.Background(), id, NodeHealthHistoryOptions{
				StartTime: "2026-04-07T00:00:00Z",
				EndTime:   "2026-04-14T00:00:00Z",
			})
			return err
		},
		"ListNodeAlertTimeline": func(id string) error {
			_, err := client.ListNodeAlertTimeline(context.Background(), ListNodeAlertTimelineOptions{NodeUUID: id})
			return err
		},
		"DescribeAlertTimeline/node": func(id string) error {
			_, err := client.DescribeAlertTimeline(context.Background(), id, "alert-1")
			return err
		},
		"DescribeAlertTimeline/alert": func(id string) error {
			_, err := client.DescribeAlertTimeline(context.Background(), "node-1", id)
			return err
		},
	}

	hostile := []string{"", "   ", ".", "..", "../..", "../../v1/tags", `a\b`, "node\nid"}

	for name, call := range calls {
		for _, id := range hostile {
			t.Run(name+"/"+strings.ReplaceAll(id, "\n", "\\n"), func(t *testing.T) {
				if err := call(id); err == nil {
					t.Fatalf("hostile identifier %q was accepted", id)
				}
			})
		}
	}

	if requests.Load() != 0 {
		t.Fatalf("expected no requests to be issued, server saw %d: %v", requests.Load(), observed)
	}
}

// Verifies a legitimate identifier still reaches the intended endpoint, so the
// validation above is not simply rejecting everything
func TestPathParamsReachIntendedEndpoint(t *testing.T) {
	var path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodeUUID":"node-1"}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	if _, err := client.DescribeNode(context.Background(), "  node-1  "); err != nil {
		t.Fatalf("describe node failed: %v", err)
	}
	if path != "/v1/nodes/node-1" {
		t.Fatalf("unexpected request path: %q", path)
	}
}

// Verifies every paginated list call rejects out-of-range paging before issuing
// a request
func TestListCallsRejectOutOfRangePagination(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	badPage := -1
	badPageSize := MaxPageSize + 1

	calls := map[string]func(page, pageSize *int) error{
		"ListAlerts": func(page, pageSize *int) error {
			_, err := client.ListAlerts(context.Background(), ListAlertsOptions{Page: page, PageSize: pageSize})
			return err
		},
		"ListAlertTimelineNodes": func(page, pageSize *int) error {
			_, err := client.ListAlertTimelineNodes(context.Background(), ListAlertTimelineNodesOptions{Page: page, PageSize: pageSize})
			return err
		},
		"ListNodeAlertTimeline": func(page, pageSize *int) error {
			_, err := client.ListNodeAlertTimeline(context.Background(), ListNodeAlertTimelineOptions{
				NodeUUID: "node-1", Page: page, PageSize: pageSize,
			})
			return err
		},
		"ListComputeZones": func(page, pageSize *int) error {
			_, err := client.ListComputeZones(context.Background(), ListComputeZonesOptions{Page: page, PageSize: pageSize})
			return err
		},
		"ListEvents": func(page, pageSize *int) error {
			_, err := client.ListEvents(context.Background(), EventListOptions{Window: "24h", Page: page, PageSize: pageSize})
			return err
		},
		"ListNodes": func(page, pageSize *int) error {
			_, err := client.ListNodes(context.Background(), ListNodesOptions{Page: page, PageSize: pageSize})
			return err
		},
		"ListNodeGroups": func(page, pageSize *int) error {
			_, err := client.ListNodeGroups(context.Background(), ListNodeGroupsOptions{Page: page, PageSize: pageSize})
			return err
		},
		"GetInventoryReport": func(page, pageSize *int) error {
			_, err := client.GetInventoryReport(context.Background(), InventoryReportOptions{Page: page, PageSize: pageSize})
			return err
		},
		"GetErrorReport": func(page, pageSize *int) error {
			_, err := client.GetErrorReport(context.Background(), ErrorReportOptions{
				View: ErrorReportViewOverview, TimeMode: ErrorReportTimeModeRelative, Window: "24h",
				Page: page, PageSize: pageSize,
			})
			return err
		},
	}

	for name, call := range calls {
		t.Run(name+"/page", func(t *testing.T) {
			if err := call(&badPage, nil); err == nil {
				t.Fatal("negative page was accepted")
			}
		})
		t.Run(name+"/pageSize", func(t *testing.T) {
			if err := call(nil, &badPageSize); err == nil {
				t.Fatal("oversized page size was accepted")
			}
		})
	}

	if requests.Load() != 0 {
		t.Fatalf("expected no requests to be issued, server saw %d", requests.Load())
	}
}
