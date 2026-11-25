// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
)

func TestInventoryReportOptionValidationBoundaries(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name      string
		opts      InventoryReportOptions
		option    string
		value     string
		wantError string
	}{
		{name: "invalid format", opts: InventoryReportOptions{Format: "xml"}, option: "format", value: "xml"},
		{name: "invalid end time", opts: InventoryReportOptions{StartTime: "2026-05-01T00:00:00Z", EndTime: "bad"}, wantError: "inventory report end time must be RFC3339"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			valuesErr := tc.opts.ValidateValues()
			if tc.option == "" {
				if valuesErr != nil {
					t.Fatalf("ValidateValues: %v", valuesErr)
				}
			} else {
				assertReportOptionError(t, valuesErr, tc.option, tc.value)
			}
			validateErr := tc.opts.Validate()
			got, getErr := client.GetInventoryReport(context.Background(), tc.opts)
			if !reflect.DeepEqual(got, InventoryReport{}) {
				t.Errorf("got report %#v; want zero report", got)
			}
			if tc.option != "" {
				assertReportOptionError(t, validateErr, tc.option, tc.value)
				assertReportOptionError(t, getErr, tc.option, tc.value)
			} else {
				for _, err := range []error{validateErr, getErr} {
					if err == nil || err.Error() != tc.wantError {
						t.Errorf("got error %v; want %q", err, tc.wantError)
					}
				}
			}
		})
	}
	if got := requests.Load(); got != 0 {
		t.Errorf("server received %d requests; want none", got)
	}
}

func assertReportOptionError(t *testing.T, err error, option, value string) {
	t.Helper()
	var optionErr *InvalidOptionError
	if !errors.As(err, &optionErr) || optionErr.Option != option || optionErr.Value != value {
		t.Errorf("got error %v; want InvalidOptionError with option %q and value %q", err, option, value)
	}
}

func TestErrorReportOptionValidationBoundaries(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name      string
		opts      ErrorReportOptions
		option    string
		value     string
		wantError string
	}{
		{name: "invalid format", opts: ErrorReportOptions{View: ErrorReportViewList, GroupBy: ErrorReportGroupByError, Format: "xml"}, option: "format", value: "xml"},
		{name: "invalid time mode", opts: ErrorReportOptions{View: ErrorReportViewList, GroupBy: ErrorReportGroupByError, TimeMode: "weekly"}, option: "timeMode", value: "weekly"},
		{name: "overview grouping", opts: ErrorReportOptions{View: ErrorReportViewOverview, GroupBy: ErrorReportGroupByNode}, wantError: "error report overview view does not support group-by"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			valuesErr := tc.opts.ValidateValues()
			if tc.option == "" {
				if valuesErr != nil {
					t.Fatalf("ValidateValues: %v", valuesErr)
				}
			} else {
				assertReportOptionError(t, valuesErr, tc.option, tc.value)
			}
			validateErr := tc.opts.Validate()
			got, getErr := client.GetErrorReport(context.Background(), tc.opts)
			if !reflect.DeepEqual(got, ErrorReport{}) {
				t.Errorf("got report %#v; want zero report", got)
			}
			if tc.option != "" {
				assertReportOptionError(t, validateErr, tc.option, tc.value)
				assertReportOptionError(t, getErr, tc.option, tc.value)
			} else {
				for _, err := range []error{validateErr, getErr} {
					if err == nil || err.Error() != tc.wantError {
						t.Errorf("got error %v; want %q", err, tc.wantError)
					}
				}
			}
		})
	}
	if got := requests.Load(); got != 0 {
		t.Errorf("server received %d requests; want none", got)
	}
}

func TestErrorReportGraphFlexibleValues(t *testing.T) {
	const payload = `{"result":[{"label":{"error":"absent"}},{"label":{"error":"null"},"values":null},{"label":{"error":"string"},"values":"line\nnext"},{"label":{"error":"array"},"values": [ [ 1, 2 ], { "count": 3 } ]}]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/reports/error" || r.URL.Query().Get("view") != "graph" || r.URL.Query().Get("groupBy") != "error" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(payload))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.GetErrorReport(context.Background(), ErrorReportOptions{View: ErrorReportViewGraph})
	if err != nil {
		t.Fatal(err)
	}
	want := []ErrorTimeSeries{{Error: "absent", Values: ""}, {Error: "null", Values: ""}, {Error: "string", Values: "line\nnext"}, {Error: "array", Values: `[[1,2],{"count":3}]`}}
	if got.View != ErrorReportViewGraph || got.GroupBy != ErrorReportGroupByError || got.Graph == nil || string(got.RawJSON) != payload {
		t.Fatalf("unexpected graph report: %#v", got)
	}
	if len(got.Graph.Result) != len(want) {
		t.Fatalf("got %d series; want %d", len(got.Graph.Result), len(want))
	}
	for i, expected := range want {
		t.Run(expected.Error, func(t *testing.T) {
			if got.Graph.Result[i] != expected {
				t.Errorf("got series %#v; want %#v", got.Graph.Result[i], expected)
			}
		})
	}
}

func TestReportForbiddenResponses(t *testing.T) {
	for _, tc := range []struct {
		name string
		path string
	}{
		{"inventory", "/v1/reports/inventory"},
		{"errors", "/v1/reports/error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := make(chan struct{}, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				select {
				case requests <- struct{}{}:
				default:
					t.Error("unexpected extra API request while recorder is full")
				}
				if r.Method != http.MethodGet || r.URL.Path != tc.path || r.URL.Query().Get("dataSource") != string(DataSourceInband) || (tc.name == "errors" && (r.URL.Query().Get("view") != string(ErrorReportViewList) || r.URL.Query().Get("groupBy") != string(ErrorReportGroupByError))) {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"permission denied"}`))
			}))
			defer server.Close()
			client, err := NewClient(server.URL, "test-key")
			if err != nil {
				t.Fatal(err)
			}
			if tc.name == "inventory" {
				got, callErr := client.GetInventoryReport(context.Background(), InventoryReportOptions{DataSource: DataSourceInband})
				err = callErr
				if !reflect.DeepEqual(got, InventoryReport{}) {
					t.Fatalf("unexpected report: %#v", got)
				}
			} else {
				got, callErr := client.GetErrorReport(context.Background(), ErrorReportOptions{DataSource: DataSourceInband, View: ErrorReportViewList, GroupBy: ErrorReportGroupByError})
				err = callErr
				if !reflect.DeepEqual(got, ErrorReport{}) {
					t.Fatalf("unexpected report: %#v", got)
				}
			}
			var apiErr *APIError
			if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusForbidden || apiErr.Message != "permission denied" {
				t.Fatalf("got error %v; want 403 APIError with permission denied message", err)
			}
			select {
			case <-requests:
			default:
				t.Fatal("request did not reach server")
			}
		})
	}
}

func TestGetAuthStatusResponseFailures(t *testing.T) {
	for _, tc := range []struct {
		name        string
		contentType string
		transport   bool
	}{
		{name: "transport error", transport: true},
		{name: "malformed JSON response", contentType: "application/json"},
		{name: "malformed text response", contentType: "text/plain"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int64
			sentinel := errors.New("auth status transport failure")
			var client *Client
			var err error
			if tc.transport {
				transport := roundTripperFunc(func(*http.Request) (*http.Response, error) {
					calls.Add(1)
					return nil, sentinel
				})
				client, err = NewClient("https://example.com", "test-key", WithHTTPClient(&http.Client{Transport: transport}))
			} else {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					if r.Method != http.MethodGet || r.URL.Path != "/v1/auth/status" || r.Header.Get("Authorization") != "Bearer test-key" {
						t.Errorf("unexpected request: %s %s, authorization %q", r.Method, r.URL.Path, r.Header.Get("Authorization"))
					}
					w.Header().Set("Content-Type", tc.contentType)
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte("{!"))
				}))
				defer server.Close()
				client, err = NewClient(server.URL, "test-key")
			}
			if err != nil {
				t.Fatal(err)
			}
			got, err := client.GetAuthStatus(context.Background())
			if !reflect.DeepEqual(got, AuthStatus{}) || calls.Load() != 1 {
				t.Errorf("got status %#v and %d requests; want zero status and one request", got, calls.Load())
			}
			if tc.transport {
				if !errors.Is(err, sentinel) {
					t.Fatalf("got error %v; want transport sentinel", err)
				}
			} else {
				var syntaxErr *json.SyntaxError
				if !errors.As(err, &syntaxErr) {
					t.Fatalf("got error %v; want JSON syntax error", err)
				}
			}
		})
	}
}
