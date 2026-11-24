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

func TestXIDBurstTransportFailures(t *testing.T) {
	for _, tc := range xidFailureEndpoints() {
		t.Run(tc.name, func(t *testing.T) {
			sentinel := errors.New("transport failure")
			calls := 0
			transport := roundTripperFunc(func(*http.Request) (*http.Response, error) {
				calls++
				return nil, sentinel
			})
			client, err := NewClient("https://example.com", "test-key", WithHTTPClient(&http.Client{Transport: transport}))
			if err != nil {
				t.Fatal(err)
			}
			got, err := tc.call(client)
			if !errors.Is(err, sentinel) || !reflect.DeepEqual(got, tc.zero) || calls != 1 {
				t.Fatalf("got result %#v, error %v, and %d calls; want zero result and transport error", got, err, calls)
			}
		})
	}
}

func TestXIDBurstMalformedSuccessfulResponses(t *testing.T) {
	for _, tc := range xidFailureEndpoints() {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != http.MethodGet || r.URL.Path != tc.path {
					t.Errorf("got request %s %s; want GET %s", r.Method, r.URL.Path, tc.path)
				}
				w.Header().Set("Content-Type", "text/plain")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("{!"))
			}))
			defer server.Close()
			client, err := NewClient(server.URL, "test-key")
			if err != nil {
				t.Fatal(err)
			}
			got, err := tc.call(client)
			var syntaxErr *json.SyntaxError
			if !errors.As(err, &syntaxErr) || !reflect.DeepEqual(got, tc.zero) || calls.Load() != 1 {
				t.Fatalf("got result %#v, error %v, and %d calls; want zero result and JSON syntax error", got, err, calls.Load())
			}
			if tc.name == "options" && !strings.Contains(err.Error(), "decode XID burst filter options") {
				t.Fatalf("options error lacks decode context: %v", err)
			}
		})
	}
}

func TestXIDBurstFilterOptionsReadFailure(t *testing.T) {
	sentinel := errors.New("body read failure")
	body := &xidOptionsFailingBody{err: sentinel}
	transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: body, Request: req}, nil
	})
	client, err := NewClient("https://example.com", "test-key", WithHTTPClient(&http.Client{Transport: transport}))
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.GetXIDBurstFilterOptions(context.Background(), XIDBurstFilterOptionsScope{})
	if !errors.Is(err, sentinel) || !strings.Contains(err.Error(), "read XID burst filter options") || !reflect.DeepEqual(got, XIDBurstFilterOptions{}) || !body.closed {
		t.Fatalf("got options %#v, error %v, body closed %t; want zero options, wrapped read error, and closed body", got, err, body.closed)
	}
}
