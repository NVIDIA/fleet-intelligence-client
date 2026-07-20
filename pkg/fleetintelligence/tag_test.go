// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package fleetintelligence

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

// Verifies tag list request construction and decoding
func TestListTagsSendsParamsAndDecodes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/tags" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		query := r.URL.Query()
		if got := query.Get("prefix"); got != "gpu" {
			t.Fatalf("unexpected prefix: %q", got)
		}
		if got := query.Get("nodeGroupId"); got != "ng-1" {
			t.Fatalf("unexpected nodeGroupId: %q", got)
		}
		if query.Has("nodeUUID") || query.Has("computeZoneId") {
			t.Fatalf("did not expect other resource filters: %q", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tags":["gpu-health","gpu-burn"]}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	got, err := client.ListTags(context.Background(), TagListOptions{Prefix: "gpu", NodeGroupID: "ng-1"})
	if err != nil {
		t.Fatalf("list tags failed: %v", err)
	}
	if !slices.Equal(got.Tags, []string{"gpu-health", "gpu-burn"}) {
		t.Fatalf("unexpected tags: %#v", got.Tags)
	}
	if !strings.Contains(string(got.RawJSON), `"tags"`) {
		t.Fatalf("raw JSON not preserved: %q", string(got.RawJSON))
	}
}

// Verifies more than one resource filter is rejected before any request
func TestListTagsRejectsMultipleResourceFilters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("did not expect a request for invalid input")
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	_, err = client.ListTags(context.Background(), TagListOptions{NodeUUID: "node-1", ComputeZoneID: "cz-1"})
	if err == nil {
		t.Fatal("expected error for multiple resource filters")
	}
	if !strings.Contains(err.Error(), "at most one") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Verifies tag list API errors are structured
func TestListTagsReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad","details":"invalid prefix"}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	_, err = client.ListTags(context.Background(), TagListOptions{Prefix: "!"})
	if err == nil {
		t.Fatal("expected API error")
	}
	if !strings.Contains(err.Error(), "400") || !strings.Contains(err.Error(), "invalid prefix") {
		t.Fatalf("unexpected error: %v", err)
	}
}
