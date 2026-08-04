// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
)

const eventsBody = `{"events":[{"eventId":"e1","nodeUUID":"node-1","component":"GPU","type":"error","name":"xid","message":"boom","timestamp":"2026-05-01T00:00:00Z","createdAt":"2026-05-01T00:00:05Z","suggestedActions":[{"action":"reboot node","code":"R1"}]}],"hasMore":false,"page":0,"pageSize":50,"total":1}`

// Verifies event list table output and time-range translation
func TestEventListTableAbsolute(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/events" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		query := r.URL.Query()
		if got := query.Get("timeMode"); got != "absolute" {
			t.Fatalf("unexpected timeMode: %q", got)
		}
		if got := query.Get("component"); got != "GPU" {
			t.Fatalf("unexpected component: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(eventsBody))
	}))
	defer server.Close()

	saveTestConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"event", "list", "--start", "2026-05-01T00:00:00Z", "--end", "2026-05-08T00:00:00Z", "--component", "GPU"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	got := out.String()
	// The narrow table exposes these columns/values.
	for _, want := range []string{
		"EVENT ID", "NODE UUID", "COMPONENT", "NAME", "TYPE", "MESSAGE", "TIMESTAMP",
		"e1", "node-1", "GPU", "xid", "boom", "2026-05-01T00:00:00Z",
		"Total Entries: 1",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
	// The verbose createdAt / suggestedActions fields are omitted from the table
	// (still available via -o json).
	for _, absent := range []string{
		"CREATED AT", "SUGGESTED ACTIONS", "2026-05-01T00:00:05Z", "reboot node (R1)",
	} {
		if strings.Contains(got, absent) {
			t.Fatalf("output should not contain %q:\n%s", absent, got)
		}
	}
}

// Verifies event list JSON output presents the page 1-based
func TestEventListJSON(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	want := `{"events":[{"eventId":"e1","nodeUUID":"node-1","component":"GPU","type":"error","name":"xid","message":"boom","timestamp":"2026-05-01T00:00:00Z","createdAt":"2026-05-01T00:00:05Z","suggestedActions":[{"action":"reboot node","code":"R1"}]}],"hasMore":false,"page":1,"pageSize":50,"total":1}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("window"); got != "24h" {
			t.Fatalf("unexpected window: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(eventsBody))
	}))
	defer server.Close()

	saveTestConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"event", "list", "--window", "24h", "--output", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}
	if strings.TrimSpace(out.String()) != want {
		t.Fatalf("unexpected JSON:\ngot  %s\nwant %s", strings.TrimSpace(out.String()), want)
	}
}

// Verifies the free-text MESSAGE column is truncated to keep the table readable
// while the full text remains available via -o json, and that the verbose
// createdAt / suggestedActions fields are omitted from the table.
func TestEventRowsTruncatesFreeText(t *testing.T) {
	longMessage := strings.Repeat("x", eventMessageColumnWidth+50)

	rows := eventRows([]nvfleetint.Event{{
		EventID:          "e1",
		Message:          longMessage,
		CreatedAt:        "2024-01-15T14:30:00Z",
		SuggestedActions: []nvfleetint.SuggestedAction{{Action: "reboot the node"}},
	}})
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if got := len(rows[0]); got != 7 {
		t.Fatalf("expected 7 columns, got %d: %q", got, rows[0])
	}

	message := rows[0][5]
	if runes := []rune(message); len(runes) != eventMessageColumnWidth {
		t.Fatalf("message column width = %d, want %d", len(runes), eventMessageColumnWidth)
	}
	if !strings.HasSuffix(message, "…") {
		t.Fatalf("message column not ellipsized: %q", message)
	}

	// The omitted verbose fields must not leak into any table cell.
	for _, cell := range rows[0] {
		if strings.Contains(cell, "reboot the node") || strings.Contains(cell, "2024-01-15T14:30:00Z") {
			t.Fatalf("unexpected omitted field in table cell: %q", cell)
		}
	}
}

// Verifies event list rejects a missing time range before any request
func TestEventListRequiresTimeRange(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("did not expect a request for invalid flags")
	}))
	defer server.Close()

	saveTestConfig(t, server.URL, "test-key")

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"event", "list"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for missing time range")
	}
}

// Verifies event buckets table output, footer, and max-buckets translation
func TestEventBucketsTable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/events/buckets" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("maxBuckets"); got != "50" {
			t.Fatalf("unexpected maxBuckets: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"bucketInterval":"1h","buckets":[{"startTime":"2026-05-01T00:00:00Z","endTime":"2026-05-01T01:00:00Z","firstEventTime":"2026-05-01T00:12:00Z","count":3}]}`))
	}))
	defer server.Close()

	saveTestConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"event", "buckets", "--window", "24h", "--max-buckets", "50"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{"START TIME", "END TIME", "COUNT", "FIRST EVENT TIME", "3", "Bucket Interval: 1h", "Total Buckets: 1"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

// Verifies event buckets rejects an out-of-range max-buckets value
func TestEventBucketsMaxBucketsValidation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("did not expect a request for invalid flags")
	}))
	defer server.Close()

	saveTestConfig(t, server.URL, "test-key")

	for _, value := range []string{"0", "2000"} {
		cmd := newRootCmd()
		cmd.SetOut(new(bytes.Buffer))
		cmd.SetArgs([]string{"event", "buckets", "--window", "24h", "--max-buckets", value})
		if err := cmd.Execute(); err == nil {
			t.Fatalf("expected error for --max-buckets %s", value)
		}
	}
}
