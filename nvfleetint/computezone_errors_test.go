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
	"strings"
	"sync/atomic"
	"testing"
)

func TestComputeZoneMalformedResponses(t *testing.T) {
	for _, view := range []ComputeZoneView{ComputeZoneViewBasic, ComputeZoneViewDetail} {
		t.Run(string(view), func(t *testing.T) {
			var requests atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.Method != http.MethodGet || r.URL.Path != "/v1/computezones" || r.URL.Query().Get("view") != string(view) {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
				}
				w.Header().Set("Content-Type", "text/plain")
				_, _ = w.Write([]byte(`{invalid`))
			}))
			defer server.Close()
			client, err := NewClient(server.URL, "test-key")
			if err != nil {
				t.Fatal(err)
			}
			got, err := client.ListComputeZones(context.Background(), ListComputeZonesOptions{View: view})
			var syntaxErr *json.SyntaxError
			if !errors.As(err, &syntaxErr) || !reflect.DeepEqual(got, ComputeZonesPage{}) {
				t.Fatalf("got page %#v and error %v; want zero page and JSON syntax error", got, err)
			}
			if requests.Load() != 1 {
				t.Errorf("got %d requests; want 1", requests.Load())
			}
		})
	}

	t.Run("update", func(t *testing.T) {
		methods := make(chan string, 10)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			methods <- r.Method
			if r.URL.Path != "/v1/computezones" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}
			if r.Method == http.MethodGet {
				if r.URL.Query().Get("view") != "basic" {
					t.Errorf("unexpected view: %s", r.URL.RawQuery)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"computezones":[` + storedComputeZone + `]}`))
				return
			}
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte(`{invalid`))
		}))
		defer server.Close()
		client, err := NewClient(server.URL, "test-key")
		if err != nil {
			t.Fatal(err)
		}
		email := "updated@example.com"
		got, err := client.UpdateComputeZone(context.Background(), "cz-1", UpdateComputeZoneOptions{ContactEmail: &email})
		var syntaxErr *json.SyntaxError
		if !errors.As(err, &syntaxErr) || !reflect.DeepEqual(got, ComputeZoneUpdate{}) {
			t.Fatalf("got update %#v and error %v; want zero update and JSON syntax error", got, err)
		}
		close(methods)
		var order []string
		for method := range methods {
			order = append(order, method)
		}
		if !reflect.DeepEqual(order, []string{http.MethodGet, http.MethodPut}) {
			t.Errorf("request order %v; want GET then PUT", order)
		}
	})
}

func TestComputeZoneInvalidUpdateDoesNotRequest(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		call func() (any, error)
		zero any
	}{
		{"update", func() (any, error) {
			zoneType := "bogus"
			return client.UpdateComputeZone(context.Background(), "cz-1", UpdateComputeZoneOptions{Type: &zoneType})
		}, ComputeZoneUpdate{}},
		{"preview", func() (any, error) {
			zoneType := "bogus"
			return client.PreviewUpdateComputeZone(context.Background(), "cz-1", UpdateComputeZoneOptions{Type: &zoneType})
		}, RequestPreview{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.call()
			if err == nil || !strings.Contains(err.Error(), `invalid compute zone type "bogus"`) || !reflect.DeepEqual(got, tc.zero) {
				t.Fatalf("got %#v and error %v; want zero result and invalid type error", got, err)
			}
			if requests.Load() != 0 {
				t.Errorf("got %d requests; want none", requests.Load())
			}
		})
	}
}

func TestComputeZoneTransportFailures(t *testing.T) {
	for _, tc := range []struct {
		name       string
		update     bool
		failMethod string
		wantOrder  []string
	}{
		{"list", false, http.MethodGet, []string{http.MethodGet}},
		{"update GET", true, http.MethodGet, []string{http.MethodGet}},
		{"update PUT", true, http.MethodPut, []string{http.MethodGet, http.MethodPut}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/computezones" || r.URL.Query().Get("view") != "basic" {
					t.Errorf("unexpected request reaching server: %s %s", r.Method, r.URL.String())
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"computezones":[` + storedComputeZone + `]}`))
			}))
			defer server.Close()
			sentinel := errors.New("transport failure")
			var methods []string
			transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				methods = append(methods, req.Method)
				if req.Method == tc.failMethod {
					return nil, sentinel
				}
				return server.Client().Transport.RoundTrip(req)
			})
			client, err := NewClient(server.URL, "test-key", WithHTTPClient(&http.Client{Transport: transport}))
			if err != nil {
				t.Fatal(err)
			}
			var got, zero any
			if tc.update {
				email := "updated@example.com"
				got, err = client.UpdateComputeZone(context.Background(), "cz-1", UpdateComputeZoneOptions{ContactEmail: &email})
				zero = ComputeZoneUpdate{}
			} else {
				got, err = client.ListComputeZones(context.Background(), ListComputeZonesOptions{View: ComputeZoneViewBasic})
				zero = ComputeZonesPage{}
			}
			if !errors.Is(err, sentinel) || !reflect.DeepEqual(got, zero) {
				t.Fatalf("got %#v and error %v; want zero result and sentinel error", got, err)
			}
			if !reflect.DeepEqual(methods, tc.wantOrder) {
				t.Errorf("request order %v; want %v", methods, tc.wantOrder)
			}
		})
	}
}
