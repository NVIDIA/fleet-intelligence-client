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

func TestSignedInventoryResponseMetadataBoundaries(t *testing.T) {
	// An empty ZIP archive, including its end-of-central-directory record.
	bundle := []byte("PK\x05\x06\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00")
	for _, tc := range []struct {
		name, contentType, disposition, filename, wantError string
	}{
		{name: "parameterized type and path", contentType: "application/zip; charset=binary", disposition: `attachment; filename="reports/bundle.zip"`, filename: "bundle.zip"},
		{name: "malformed disposition", contentType: "application/zip", disposition: `attachment; filename="unterminated`, filename: "inventory-report.zip"},
		{name: "dot filename", contentType: "application/zip", disposition: `attachment; filename="."`, filename: "inventory-report.zip"},
		{name: "missing content type", wantError: "signed inventory report response missing content type"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query()
				if r.Method != http.MethodGet || r.URL.Path != "/v1/reports/inventory" || q.Get("dataSource") != "oob" || q.Get("signed") != "true" || q.Get("format") != "csv" || r.Header.Get("Accept") != "application/zip" {
					t.Errorf("unexpected request: %s %s, Accept %q", r.Method, r.URL.String(), r.Header.Get("Accept"))
				}
				if tc.contentType == "" {
					w.Header()["Content-Type"] = nil
				} else {
					w.Header().Set("Content-Type", tc.contentType)
				}
				if tc.disposition != "" {
					w.Header().Set("Content-Disposition", tc.disposition)
				}
				_, _ = w.Write(bundle)
			}))
			defer server.Close()
			client, err := NewClient(server.URL, "test-key")
			if err != nil {
				t.Fatal(err)
			}
			got, err := client.GetInventoryReport(context.Background(), InventoryReportOptions{DataSource: DataSourceOOB, Format: ReportFormatCSV, Signed: true})
			if tc.wantError != "" {
				if err == nil || err.Error() != tc.wantError || !reflect.DeepEqual(got, InventoryReport{}) {
					t.Fatalf("got report %#v and error %v; want zero report and %q", got, err, tc.wantError)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Filename != tc.filename || !reflect.DeepEqual(got.RawSigned, bundle) {
				t.Fatalf("got filename %q and bundle %q; want %q and %q", got.Filename, got.RawSigned, tc.filename, bundle)
			}
		})
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

func TestReportOptionValuesValid(t *testing.T) {
	for _, tc := range []struct {
		value ReportFormat
		want  bool
	}{{ReportFormatJSON, true}, {ReportFormatCSV, true}, {"xml", false}} {
		t.Run("format/"+string(tc.value), func(t *testing.T) {
			if got := tc.value.Valid(); got != tc.want {
				t.Errorf("Valid() = %v; want %v", got, tc.want)
			}
		})
	}
	for _, tc := range []struct {
		value ErrorReportTimeMode
		want  bool
	}{{ErrorReportTimeModeAbsolute, true}, {ErrorReportTimeModeRelative, true}, {"weekly", false}} {
		t.Run("time mode/"+string(tc.value), func(t *testing.T) {
			if got := tc.value.Valid(); got != tc.want {
				t.Errorf("Valid() = %v; want %v", got, tc.want)
			}
		})
	}
}

func TestReportResponseDecodingErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		path   string
		source DataSource
		view   ErrorReportView
		group  ErrorReportGroupBy
		body   string
	}{
		{"inventory inband", "/v1/reports/inventory", DataSourceInband, "", "", `{"page":"not-an-integer"}`},
		{"inventory oob", "/v1/reports/inventory", DataSourceOOB, "", "", `{"page":"not-an-integer"}`},
		{"errors list by error", "/v1/reports/error", DataSourceInband, ErrorReportViewList, ErrorReportGroupByError, `{"page":"not-an-integer"}`},
		{"errors list by node", "/v1/reports/error", DataSourceOOB, ErrorReportViewList, ErrorReportGroupByNode, `{"page":"not-an-integer"}`},
		{"errors graph", "/v1/reports/error", DataSourceInband, ErrorReportViewGraph, ErrorReportGroupByError, `{"result":"not-an-array"}`},
		{"errors overview", "/v1/reports/error", DataSourceOOB, ErrorReportViewOverview, "", `{"totalErrors":"not-an-integer"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := make(chan struct{}, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				select {
				case requests <- struct{}{}:
				default:
					t.Error("unexpected extra API request while recorder is full")
				}
				if r.Method != http.MethodGet || r.URL.Path != tc.path || r.URL.Query().Get("dataSource") != string(tc.source) || r.URL.Query().Get("view") != string(tc.view) || r.URL.Query().Get("groupBy") != string(tc.group) {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
				}
				w.Header().Set("Content-Type", "text/plain")
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			client, err := NewClient(server.URL, "test-key")
			if err != nil {
				t.Fatal(err)
			}
			var decodeErr *json.UnmarshalTypeError
			if tc.path == "/v1/reports/inventory" {
				got, err := client.GetInventoryReport(context.Background(), InventoryReportOptions{DataSource: tc.source})
				if !errors.As(err, &decodeErr) || !reflect.DeepEqual(got, InventoryReport{DataSource: tc.source}) {
					t.Fatalf("got result %#v and error %v; want empty payload and decoding error", got, err)
				}
			} else {
				got, err := client.GetErrorReport(context.Background(), ErrorReportOptions{DataSource: tc.source, View: tc.view, GroupBy: tc.group})
				if !errors.As(err, &decodeErr) || !reflect.DeepEqual(got, ErrorReport{DataSource: tc.source}) {
					t.Fatalf("got result %#v and error %v; want empty payload and decoding error", got, err)
				}
			}
			select {
			case <-requests:
			default:
				t.Fatal("request did not reach server")
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

func TestReportCanceledContexts(t *testing.T) {
	for _, endpoint := range []string{"inventory", "errors"} {
		t.Run(endpoint, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
				t.Error("canceled request reached server")
			}))
			defer server.Close()
			client, err := NewClient(server.URL, "test-key")
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if endpoint == "inventory" {
				got, err := client.GetInventoryReport(ctx, InventoryReportOptions{DataSource: DataSourceInband})
				if !errors.Is(err, context.Canceled) || !reflect.DeepEqual(got, InventoryReport{}) {
					t.Fatalf("got result %#v and error %v; want zero result and context cancellation", got, err)
				}
			} else {
				got, err := client.GetErrorReport(ctx, ErrorReportOptions{DataSource: DataSourceInband, View: ErrorReportViewList, GroupBy: ErrorReportGroupByError})
				if !errors.Is(err, context.Canceled) || !reflect.DeepEqual(got, ErrorReport{}) {
					t.Fatalf("got result %#v and error %v; want zero result and context cancellation", got, err)
				}
			}
		})
	}
}

func TestSignedReportNilResponse(t *testing.T) {
	err := validateSignedReportContentType(nil)
	if err == nil || err.Error() != "signed inventory report response missing content type" {
		t.Fatalf("validateSignedReportContentType(nil) = %v, want missing content type error", err)
	}
	if got := signedReportFilename(nil); got != "inventory-report.zip" {
		t.Fatalf("signedReportFilename(nil) = %q, want inventory-report.zip", got)
	}
}

func TestDecodeErrorReportUnsupportedView(t *testing.T) {
	got, err := decodeErrorReport([]byte(`{}`), "unknown", "")
	if err == nil || err.Error() != `invalid error report view "unknown"` {
		t.Fatalf("decodeErrorReport error = %v, want invalid view error", err)
	}
	if !reflect.DeepEqual(got, ErrorReport{}) {
		t.Fatalf("decodeErrorReport report = %#v, want zero ErrorReport", got)
	}
}

func TestReportRawValueString(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
		want string
	}{
		{name: "malformed", raw: "  [1,  ", want: "[1,"},
		{name: "compact JSON", raw: `  [1, 2]  `, want: `[1,2]`},
		{name: "JSON string", raw: `  "hello\nworld"  `, want: "hello\nworld"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := reportRawValueString([]byte(tc.raw)); got != tc.want {
				t.Fatalf("reportRawValueString(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}
