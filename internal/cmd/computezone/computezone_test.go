// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package computezone

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdutil"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
)

// newRootCmd builds a root command carrying only this package's commands, so
// the tests drive them through the same argument path a user types.
func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "nvfleetint",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(NewCmd())
	return root
}

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
		_, _ = w.Write([]byte(`{"computezones":[{"id":"cz-1","name":"East","type":"datacenter","location":{"city":"Santa Clara","country":"US"},"nodesCount":7}],"hasMore":false,"page":1,"pageSize":50,"total":1}`))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

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

	cmdtest.SaveConfig(t, server.URL, "test-key")

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

// updateServer serves the read-modify-write pair, recording every request the
// command issued.
func updateServer(t *testing.T, requests *[]string, body *[]byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*requests = append(*requests, r.Method+" "+r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"computezones":[{"id":"cz-1","name":"East","type":"datacenter",` +
				`"contact":{"email":"ops@example.com","pic":"Jane Doe"},` +
				`"location":{"city":"Baltimore","region":"us-east-1","latitude":39.04581234}}],"page":0,"pageSize":20,"total":1}`))
			return
		}

		read, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body failed: %v", err)
		}
		*body = read
		_, _ = w.Write([]byte(`{"id":"cz-1"}`))
	}))
}

// Verifies that per-field flags merge over the stored zone and that the result
// is rendered as a FIELD/VALUE table
func TestComputeZoneUpdateTable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var requests []string
	var body []byte
	server := updateServer(t, &requests, &body)
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"computezone", "update", "cz-1",
		"--type", "cloud provider",
		"--contact-email", "new@example.com",
		"--location-country", "United States",
		"--yes",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	if !slices.Equal(requests, []string{"GET /v1/computezones", "PUT /v1/computezones"}) {
		t.Fatalf("unexpected requests: %#v", requests)
	}

	want := `{"id":"cz-1","type":"cloud provider","contact":{"email":"new@example.com","pic":"Jane Doe"},` +
		`"location":{"city":"Baltimore","country":"United States","region":"us-east-1","latitude":39.04581234}}`
	if string(body) != want {
		t.Fatalf("unexpected body:\n got %s\nwant %s", body, want)
	}

	got := out.String()
	for _, expected := range []string{
		"FIELD", "VALUE", "cz-1", "cloud provider", "new@example.com", "Jane Doe",
		"Baltimore", "United States", "us-east-1", "39.04581234",
	} {
		if !strings.Contains(got, expected) {
			t.Fatalf("output missing %q:\n%s", expected, got)
		}
	}
}

// Verifies that an empty flag value clears one field and that JSON output is
// the backend's own payload
func TestComputeZoneUpdateClearsFieldJSON(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var requests []string
	var body []byte
	server := updateServer(t, &requests, &body)
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"computezone", "update", "cz-1", "--contact-pic", "", "--yes", "-o", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	want := `{"id":"cz-1","type":"datacenter","contact":{"email":"ops@example.com"},` +
		`"location":{"city":"Baltimore","region":"us-east-1","latitude":39.04581234}}`
	if string(body) != want {
		t.Fatalf("unexpected body:\n got %s\nwant %s", body, want)
	}

	var decoded map[string]any
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("json output not decodable: %v (%s)", err, out.String())
	}
	if decoded["id"] != "cz-1" {
		t.Fatalf("unexpected json output: %s", out.String())
	}
}

// Verifies that a declined confirmation writes nothing
func TestComputeZoneUpdateDeclinedWritesNothing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var requests []string
	var body []byte
	server := updateServer(t, &requests, &body)
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	// A strings.Reader is not an *os.File, so the prompt treats it as
	// answerable and reads the refusal from it.
	cmd.SetIn(strings.NewReader("n\n"))
	cmd.SetArgs([]string{"computezone", "update", "cz-1", "--type", "datacenter"})

	err := cmd.Execute()
	if !errors.Is(err, cmdutil.ErrAborted) {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(requests) != 0 {
		t.Fatalf("declined update still issued requests: %#v", requests)
	}
	if !strings.Contains(out.String(), "This updates compute zone cz-1") {
		t.Fatalf("confirmation summary missing:\n%s", out.String())
	}
}

// Verifies the flag validation that runs before any request
func TestComputeZoneUpdateRejectsBadFlags(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var requests []string
	var body []byte
	server := updateServer(t, &requests, &body)
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	cases := []struct {
		name string
		args []string
		want string
	}{
		{"no fields", []string{"computezone", "update", "cz-1", "--yes"}, "no changes requested"},
		{"bad type", []string{"computezone", "update", "cz-1", "--type", "bogus", "--yes"}, `invalid type "bogus"`},
		{"bad latitude", []string{"computezone", "update", "cz-1", "--location-latitude", "91", "--yes"}, `invalid location-latitude "91"`},
		{"nan latitude", []string{"computezone", "update", "cz-1", "--location-latitude", "NaN", "--yes"}, `invalid location-latitude "NaN"`},
		{"missing id", []string{"computezone", "update", "--yes"}, "compute zone ID is required"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs(testCase.args)

			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}

	if len(requests) != 0 {
		t.Fatalf("rejected updates still issued requests: %#v", requests)
	}
}
