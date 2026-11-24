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
