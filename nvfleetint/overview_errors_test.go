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

func TestOverviewInvalidSuccessfulResponses(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "malformed JSON", body: `{`},
		{name: "incompatible field type", body: `{"nodesCount":"bad"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.Method != http.MethodGet || r.URL.Path != "/v1/overview" {
					t.Errorf("got request %s %s; want GET /v1/overview", r.Method, r.URL.Path)
				}
				w.Header().Set("Content-Type", "text/plain")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()

			client, err := NewClient(server.URL, "test-key")
			if err != nil {
				t.Fatal(err)
			}
			got, err := client.GetOverview(context.Background(), OverviewOptions{})
			if !reflect.DeepEqual(got, Overview{}) || requests.Load() != 1 {
				t.Errorf("got overview %#v and %d requests; want zero overview and one request", got, requests.Load())
			}
			if tc.name == "malformed JSON" {
				var syntaxErr *json.SyntaxError
				if !errors.As(err, &syntaxErr) {
					t.Fatalf("got error %v; want JSON syntax error", err)
				}
			} else {
				var typeErr *json.UnmarshalTypeError
				if !errors.As(err, &typeErr) {
					t.Fatalf("got error %v; want JSON unmarshal type error", err)
				}
			}
		})
	}
}
