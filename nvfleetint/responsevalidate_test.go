// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Verifies the field checks the OpenAPI contract declares, and that payload
// shapes which are not JSON objects of interest are left alone
func TestValidateResponsePayload(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		wantErr string
	}{
		{name: "ordinary node", payload: `{"nodeUUID":"n-1","hostname":"gpu-001","publicIP":"10.0.0.1","privateIP":"192.168.1.1"}`},
		{name: "ipv6", payload: `{"publicIP":"2001:db8::1"}`},
		{name: "absent address", payload: `{"publicIP":"","privateIP":""}`},
		{name: "nested object", payload: `{"node":{"publicIP":"10.0.0.1"}}`},
		{name: "array of objects", payload: `{"nodes":[{"hostname":"gpu-1"},{"hostname":"gpu-2"}]}`},
		{name: "valid alert", payload: `{"severity":"Critical","state":"Resolved"}`},
		{name: "alert message may wrap", payload: `{"message":"line one\nline two\ttabbed"}`},

		{name: "falsified ip", payload: `{"publicIP":"10.0.0.1; rm -rf /"}`, wantErr: "not a valid IP address"},
		{name: "hostname as ip", payload: `{"privateIP":"gpu-001.example.com"}`, wantErr: "not a valid IP address"},
		{name: "nested falsified ip", payload: `{"node":{"publicIP":"nope"}}`, wantErr: "not a valid IP address"},
		{name: "falsified ip in array", payload: `{"nodes":[{"hostname":"ok"},{"publicIP":"nope"}]}`, wantErr: "not a valid IP address"},
		{name: "hostname with markup", payload: `{"hostname":"<script>alert(1)</script>"}`, wantErr: "invalid hostname character"},
		{name: "hostname with space", payload: `{"hostname":"gpu 001"}`, wantErr: "invalid hostname character"},
		{name: "overlong hostname", payload: `{"hostname":"` + strings.Repeat("a", 254) + `"}`, wantErr: "DNS limit"},
		{name: "unknown severity", payload: `{"severity":"Catastrophic"}`, wantErr: "not a valid alert severity"},
		{name: "unknown state", payload: `{"state":"Exploded"}`, wantErr: "not a valid alert state"},

		// A field name appearing as someone else's value must not be validated.
		{name: "field name used as a value", payload: `{"note":"publicIP","other":"hostname"}`},
		// Payloads that are not JSON at all are the CSV and ZIP report bodies.
		{name: "csv body", payload: "hostname,publicIP\ngpu-001,not-an-ip\n"},
		{name: "malformed json", payload: `{"publicIP":`},
		{name: "empty body", payload: ``},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			err := validateResponsePayload([]byte(testCase.payload))
			if testCase.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected an error")
			}
			if !errors.Is(err, ErrInvalidResponse) {
				t.Fatalf("error is not ErrInvalidResponse: %v", err)
			}
			if !strings.Contains(err.Error(), testCase.wantErr) {
				t.Fatalf("error %q does not mention %q", err, testCase.wantErr)
			}
		})
	}
}

// Verifies control characters are refused in any response string, whatever the
// field, since they are what would let a tampered payload rewrite an operator's
// terminal through rendered output
func TestValidateResponseRejectsControlCharacters(t *testing.T) {
	hostile := map[string]string{
		"ansi escape":      "{\"hostname\":\"gpu-001\\u001b[31mFAKE\"}",
		"null byte":        "{\"message\":\"a\\u0000b\"}",
		"bell":             "{\"component\":\"\\u0007\"}",
		"c1 control":       "{\"message\":\"a\\u0085b\"}",
		"escape in nested": "{\"node\":{\"error\":\"\\u001b]0;title\\u0007\"}}",
	}

	for name, payload := range hostile {
		t.Run(name, func(t *testing.T) {
			err := validateResponsePayload([]byte(payload))
			if err == nil {
				t.Fatal("expected a control character to be rejected")
			}
			if !errors.Is(err, ErrInvalidResponse) {
				t.Fatalf("error is not ErrInvalidResponse: %v", err)
			}
			if !strings.Contains(err.Error(), "control character") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// Verifies the rejection message cannot itself carry escape sequences into the
// terminal that is about to print it
func TestResponseValidationErrorDoesNotEmitControlBytes(t *testing.T) {
	err := validateResponsePayload([]byte("{\"hostname\":\"gpu\\u001b[31m\"}"))
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.ContainsRune(err.Error(), 0x1b) {
		t.Fatalf("error echoed a raw escape byte: %q", err.Error())
	}
}

// Verifies a tampered payload fails the SDK call rather than being decoded and
// rendered as fleet state
func TestTamperedResponseFailsTheCall(t *testing.T) {
	cases := map[string]string{
		"falsified ip":     `{"nodeUUID":"node-1","hostname":"gpu-001","publicIP":"attacker-controlled"}`,
		"escaped hostname": "{\"nodeUUID\":\"node-1\",\"hostname\":\"gpu-001\\u001b[2K\"}",
	}

	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(payload))
			}))
			defer server.Close()

			client, err := NewClient(server.URL, "test-key")
			if err != nil {
				t.Fatalf("new client failed: %v", err)
			}

			if _, err := client.DescribeNode(context.Background(), "node-1"); !errors.Is(err, ErrInvalidResponse) {
				t.Fatalf("expected ErrInvalidResponse, got %v", err)
			}
		})
	}
}

// Verifies an untampered response still decodes, so the validation is not
// simply failing everything
func TestValidResponseStillDecodes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodeUUID":"node-1","hostname":"gpu-001","publicIP":"10.0.0.1","privateIP":"192.168.1.1"}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	node, err := client.DescribeNode(context.Background(), "node-1")
	if err != nil {
		t.Fatalf("describe node failed: %v", err)
	}
	if node.Hostname != "gpu-001" || node.PublicIP != "10.0.0.1" {
		t.Fatalf("unexpected node: %#v", node.Node)
	}
}

// Verifies validation runs on every SDK entry point that decodes JSON, so a
// tampered field cannot slip through whichever endpoint happens to carry it
func TestEveryJSONEndpointValidates(t *testing.T) {
	// A hostname carrying an ANSI escape is accepted by every response shape,
	// since the check is keyed on the field name wherever it appears.
	const tampered = "{\"hostname\":\"gpu-001\\u001b[31m\"}"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(tampered))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}
	ctx := context.Background()

	calls := map[string]func() error{
		"GetAuthStatus":    func() error { _, err := client.GetAuthStatus(ctx); return err },
		"GetOverview":      func() error { _, err := client.GetOverview(ctx, OverviewOptions{}); return err },
		"ListNodes":        func() error { _, err := client.ListNodes(ctx, ListNodesOptions{}); return err },
		"DescribeNode":     func() error { _, err := client.DescribeNode(ctx, "node-1"); return err },
		"ListNodeGroups":   func() error { _, err := client.ListNodeGroups(ctx, ListNodeGroupsOptions{}); return err },
		"ListComputeZones": func() error { _, err := client.ListComputeZones(ctx, ListComputeZonesOptions{}); return err },
		"ListAlerts":       func() error { _, err := client.ListAlerts(ctx, ListAlertsOptions{}); return err },
		"ListAlertTimelineNodes": func() error {
			_, err := client.ListAlertTimelineNodes(ctx, ListAlertTimelineNodesOptions{})
			return err
		},
		"ListNodeAlertTimeline": func() error {
			_, err := client.ListNodeAlertTimeline(ctx, ListNodeAlertTimelineOptions{NodeUUID: "node-1"})
			return err
		},
		"DescribeAlertTimeline": func() error {
			_, err := client.DescribeAlertTimeline(ctx, "node-1", "alert-1")
			return err
		},
		"ListEvents":      func() error { _, err := client.ListEvents(ctx, EventListOptions{Window: "24h"}); return err },
		"GetEventBuckets": func() error { _, err := client.GetEventBuckets(ctx, EventBucketsOptions{Window: "24h"}); return err },
		"ListTags":        func() error { _, err := client.ListTags(ctx, TagListOptions{}); return err },
		"NodeHealthHistory": func() error {
			_, err := client.NodeHealthHistory(ctx, "node-1", NodeHealthHistoryOptions{
				StartTime: "2026-04-07T00:00:00Z",
				EndTime:   "2026-04-14T00:00:00Z",
			})
			return err
		},
		"GetInventoryReport": func() error {
			_, err := client.GetInventoryReport(ctx, InventoryReportOptions{})
			return err
		},
		"GetErrorReport": func() error {
			_, err := client.GetErrorReport(ctx, ErrorReportOptions{
				View: ErrorReportViewOverview, TimeMode: ErrorReportTimeModeRelative, Window: "24h",
			})
			return err
		},
	}

	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			if err := call(); !errors.Is(err, ErrInvalidResponse) {
				t.Fatalf("tampered payload was not rejected, got %v", err)
			}
		})
	}
}
