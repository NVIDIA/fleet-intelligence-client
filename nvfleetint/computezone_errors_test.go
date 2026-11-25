// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

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
