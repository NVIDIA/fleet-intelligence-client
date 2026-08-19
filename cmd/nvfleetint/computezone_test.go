// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
)

// Verifies table output and request filters
func TestComputeZoneListTableAndFilters(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

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
		// --page 2 (1-based) maps to the SDK's 0-based page 1.
		if got := query.Get("page"); got != "1" {
			t.Fatalf("unexpected page: %q", got)
		}
		if got := query.Get("pageSize"); got != "50" {
			t.Fatalf("unexpected pageSize: %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"computezones":[{"id":"cz-1","name":"East","type":"datacenter","geoLocation":{"city":"Santa Clara","country":"US"},"nodesCount":7}],"hasMore":false,"page":1,"pageSize":50,"total":1}`))
	}))
	defer server.Close()

	saveTestConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"computezone", "list", "--include-metrics=false", "--zone-ids", "cz-1,cz-2", "--page", "2", "--page-size", "50"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{"ID", "NAME", "TYPE", "LOCATION", "NODE COUNT", "cz-1", "East", "datacenter", "Santa Clara, US", "Page: 1  Total Pages: 1"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q: %q", want, got)
		}
	}
}

// Verifies all-page JSON output
func TestComputeZoneListAllJSONMergesRawItems(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/computezones" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("view"); got != "basic" {
			t.Fatalf("unexpected view: %q", got)
		}
		if got := r.URL.Query().Get("pageSize"); got != "1" {
			t.Fatalf("unexpected pageSize: %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("page") {
		case "0":
			requests++
			_, _ = w.Write([]byte(`{"computezones":[{"id":"cz-1","name":"East","extra":"kept"}],"hasMore":true,"page":0,"pageSize":1,"total":2}`))
		case "1":
			requests++
			_, _ = w.Write([]byte(`{"computezones":[{"id":"cz-2","name":"West"}],"hasMore":false,"page":1,"pageSize":1,"total":2}`))
		default:
			t.Fatalf("unexpected page: %q", r.URL.Query().Get("page"))
		}
	}))
	defer server.Close()

	saveTestConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"computezone", "list", "--view", "basic", "--all", "--output", "json", "--page-size", "1"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	var got struct {
		Items      []map[string]any `json:"items"`
		Pagination struct {
			Page         int  `json:"page"`
			PageSize     int  `json:"pageSize"`
			Total        int  `json:"total"`
			HasMore      bool `json:"hasMore"`
			PagesFetched int  `json:"pagesFetched"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, out.String())
	}
	if requests != 2 {
		t.Fatalf("unexpected request count: %d", requests)
	}
	if len(got.Items) != 2 || got.Items[0]["id"] != "cz-1" || got.Items[0]["extra"] != "kept" {
		t.Fatalf("unexpected merged items: %#v", got.Items)
	}
	if got.Pagination.Page != 1 || got.Pagination.PageSize != 1 || got.Pagination.Total != 2 || got.Pagination.HasMore || got.Pagination.PagesFetched != 2 {
		t.Fatalf("unexpected pagination: %#v", got.Pagination)
	}
}

// Verifies basic compute zone rows
func TestWriteComputeZoneBasicTable(t *testing.T) {
	var out bytes.Buffer
	err := writeComputeZoneTable(&out, string(nvfleetint.ComputeZoneViewBasic), []nvfleetint.ComputeZone{
		{ID: "cz-1", Name: "East"},
	})
	if err != nil {
		t.Fatalf("write table failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{"ID", "NAME", "cz-1", "East"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q: %q", want, got)
		}
	}
}

// Verifies update reads current state before writing a merged body
func TestComputeZoneUpdatePreservesUnchangedBackendFields(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var sawPut bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if r.URL.Path != "/v1/computezones" {
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
			if got := r.URL.Query()["computeZoneIds"]; !slices.Equal(got, []string{"cz-1"}) {
				t.Fatalf("unexpected computeZoneIds: %#v", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"computezones":[{"id":"cz-1","type":"datacenter","contact":{"email":"old@example.com","pic":"Grace"},"geoLocation":{"city":"Santa Clara","country":"US","region":"us-west"}}],"hasMore":false,"page":0,"pageSize":20,"total":1}`))
		case http.MethodPut:
			sawPut = true
			data, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read body failed: %v", err)
			}
			body := string(data)
			for _, want := range []string{`"id":"cz-1"`, `"type":"datacenter"`, `"email":"new@example.com"`, `"pic":"Grace"`, `"city":"Santa Clara"`, `"country":"CA"`, `"region":"us-west"`} {
				if !strings.Contains(body, want) {
					t.Fatalf("body missing %q: %s", want, body)
				}
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"cz-1"}`))
		default:
			t.Fatalf("unexpected method: %s", r.Method)
		}
	}))
	defer server.Close()

	saveTestConfig(t, server.URL, "test-key")

	stdout, stderr := runCLI(t, "computezone", "update", "cz-1", "--contact-email", "new@example.com", "--geo-country", "CA", "--yes")
	if !sawPut {
		t.Fatal("expected PUT request")
	}
	if !strings.Contains(stdout, `Compute zone "cz-1" updated.`) {
		t.Fatalf("unexpected stdout: %q", stdout)
	}
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
}

// Verifies dry-run reads current state and prints the merged request without writing
func TestComputeZoneUpdateDryRunJSON(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("dry-run should not write, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"computezones":[{"id":"cz-1","type":"datacenter","contact":{"email":"old@example.com","pic":"Grace"}}],"hasMore":false,"page":0,"pageSize":20,"total":1}`))
	}))
	defer server.Close()

	saveTestConfig(t, server.URL, "test-key")

	stdout, stderr := runCLI(t, "computezone", "update", "cz-1", "--contact-email", "new@example.com", "--dry-run", "-o", "json")
	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}

	var got nvfleetint.RequestPreview
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode preview failed: %v\n%s", err, stdout)
	}
	if got.Method != http.MethodPut || got.URL != server.URL+"/v1/computezones" {
		t.Fatalf("unexpected preview: %#v", got)
	}
	if !strings.Contains(string(got.Body), `"email":"new@example.com"`) || !strings.Contains(string(got.Body), `"pic":"Grace"`) {
		t.Fatalf("preview body did not merge fields: %s", string(got.Body))
	}
}

// Verifies coordinate flags are validated before any request is made
func TestComputeZoneUpdateRejectsInvalidCoordinates(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		t.Fatalf("invalid coordinates should not reach the backend: %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()

	saveTestConfig(t, server.URL, "test-key")

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "latitude range", args: []string{"--geo-latitude", "1000"}, want: `--geo-latitude: invalid latitude "1000": must be between -90 and 90`},
		{name: "longitude range", args: []string{"--geo-longitude", "-400"}, want: `--geo-longitude: invalid longitude "-400": must be between -180 and 180`},
		{name: "latitude text", args: []string{"--geo-latitude", "north"}, want: "expected a decimal number"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newRootCmd()
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetArgs(append([]string{"computezone", "update", "cz-1", "--yes"}, tt.args...))

			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("unexpected error: got %v want %q", err, tt.want)
			}
		})
	}
}

// Verifies an empty coordinate clears the stored value
func TestComputeZoneUpdateClearsCoordinates(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"computezones":[{"id":"cz-1","geoLocation":{"city":"Santa Clara","latitude":37.4,"longitude":-121.9}}],"hasMore":false,"page":0,"pageSize":20,"total":1}`))
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

	saveTestConfig(t, server.URL, "test-key")

	runCLI(t, "computezone", "update", "cz-1", "--geo-latitude", "", "--geo-longitude", "", "--yes")
	if !strings.Contains(body, `"geoLocation":{"city":"Santa Clara"}`) {
		t.Fatalf("coordinates were not cleared: %s", body)
	}
}

// Verifies local flag validation
func TestComputeZoneListRejectsInvalidFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "view", args: []string{"computezone", "list", "--view", "wide"}, want: "invalid view"},
		{name: "basic include metrics", args: []string{"computezone", "list", "--view", "basic", "--include-metrics=false"}, want: "basic compute zone view is incompatible with --include-metrics"},
		{name: "zone ids", args: []string{"computezone", "list", "--zone-ids", "cz-1,,cz-2"}, want: "empty values are not allowed"},
		{name: "page size", args: []string{"computezone", "list", "--page-size", "0"}, want: "--page-size must be between"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newRootCmd()
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetArgs(tt.args)

			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("unexpected error: got %v want %q", err, tt.want)
			}
		})
	}
}
