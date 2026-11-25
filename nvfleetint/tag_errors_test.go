// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
)

func TestListTagsResourceScopes(t *testing.T) {
	for _, tc := range []struct {
		name  string
		opts  TagListOptions
		field string
	}{
		{name: "node", opts: TagListOptions{Prefix: "  rack  ", NodeUUID: "  node-1  "}, field: "nodeUUID"},
		{name: "compute zone", opts: TagListOptions{Prefix: "  rack  ", ComputeZoneID: "  zone-1  "}, field: "computeZoneId"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const payload = `{"tags":["rack-blue","rack-red"]}`
			var requests atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				q := r.URL.Query()
				wantID := "node-1"
				if tc.field == "computeZoneId" {
					wantID = "zone-1"
				}
				if r.Method != http.MethodGet || r.URL.Path != "/v1/tags" || q.Get("prefix") != "rack" || q.Get(tc.field) != wantID {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
				}
				for _, field := range []string{"nodeUUID", "nodeGroupId", "computeZoneId"} {
					if field != tc.field && q.Has(field) {
						t.Errorf("unexpected resource filter %q in %s", field, r.URL.RawQuery)
					}
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(payload))
			}))
			defer server.Close()
			client, err := NewClient(server.URL, "test-key")
			if err != nil {
				t.Fatal(err)
			}
			got, err := client.ListTags(context.Background(), tc.opts)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got.Tags, []string{"rack-blue", "rack-red"}) || string(got.RawJSON) != payload || requests.Load() != 1 {
				t.Fatalf("got %#v after %d requests", got, requests.Load())
			}
		})
	}
}
