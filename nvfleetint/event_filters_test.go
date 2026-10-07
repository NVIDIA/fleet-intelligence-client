// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestEventFiltersAcrossEndpoints(t *testing.T) {
	for _, endpoint := range []string{"list", "buckets"} {
		for _, sources := range [][]DataSource{nil, {DataSourceOOB}, {DataSourceInband, DataSourceOOB}} {
			for _, exclude := range []bool{false, true} {
				t.Run(endpoint+"/"+stringSources(sources)+"/exclude="+map[bool]string{true: "true", false: "false"}[exclude], func(t *testing.T) {
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						q := r.URL.Query()
						expected := map[string][]string{
							"nameSearch": {"bmc"}, "eventNames": {"RedfishEvent", "Others"}, "severities": {"Critical", "Info"},
							"components": {"gpu", "oob-redfish-events"}, "gpuTypes": {"H100", "B200"},
							"timeMode": {"relative"}, "window": {"24h"},
						}
						if len(sources) > 0 {
							for _, source := range sources {
								expected["dataSources"] = append(expected["dataSources"], string(source))
							}
						}
						if exclude {
							expected["excludeNodeGroupIds"] = []string{"ng1", "ng2"}
							expected["excludeComputeZoneIds"] = []string{"cz1", "cz2"}
						} else {
							expected["nodeGroupIds"] = []string{"ng1", "ng2"}
							expected["computeZoneIds"] = []string{"cz1", "cz2"}
						}
						path := "/v1/events"
						body := `{"events":[{"eventId":"e1","nodeUUID":"n1","hostname":"os-1","nodeName":"machine-1","bmcHostname":"bmc-1","dataSource":"oob","gpuType":"H100","nodeGroup":"group","nodeGroupId":"ng1","computeZone":"zone","computeZoneId":"cz1"}],"total":1}`
						if endpoint == "list" {
							expected["sortBy"] = []string{"bmcHostname"}
							expected["sortOrder"] = []string{"asc"}
						} else {
							path += "/buckets"
							body = `{"bucketInterval":"1h","buckets":[]}`
						}
						if r.URL.Path != path || !reflect.DeepEqual(map[string][]string(q), expected) {
							t.Errorf("request = %s?%s; want %s %v", r.URL.Path, r.URL.RawQuery, path, expected)
						}
						w.Header().Set("Content-Type", "application/json")
						_, _ = w.Write([]byte(body))
					}))
					defer server.Close()
					client, err := NewClient(server.URL, "test-key")
					if err != nil {
						t.Fatal(err)
					}
					opts := EventListOptions{Window: "24h", NameSearch: " bmc ", EventNames: []string{"RedfishEvent", "Others"}, Severities: []string{"Critical", "Info"}, Components: []string{"gpu", "oob-redfish-events"}, GPUTypes: []string{"H100", "B200"}, DataSources: sources, SortBy: EventSortByBMCHostname, SortOrder: EventOrderAsc}
					if exclude {
						opts.ExcludeNodeGroupIDs = []string{"ng1", "ng2"}
						opts.ExcludeComputeZoneIDs = []string{"cz1", "cz2"}
					} else {
						opts.NodeGroupIDs = []string{"ng1", "ng2"}
						opts.ComputeZoneIDs = []string{"cz1", "cz2"}
					}
					if endpoint == "list" {
						page, err := client.ListEvents(context.Background(), opts)
						if err != nil {
							t.Fatal(err)
						}
						want := Event{EventID: "e1", NodeUUID: "n1", Hostname: "os-1", NodeName: "machine-1", BMCHostname: "bmc-1", DataSource: "oob", GPUType: "H100", NodeGroup: "group", NodeGroupID: "ng1", ComputeZone: "zone", ComputeZoneID: "cz1"}
						if len(page.Events) != 1 || !reflect.DeepEqual(page.Events[0], want) {
							t.Fatalf("events = %#v", page.Events)
						}
					} else {
						_, err = client.GetEventBuckets(context.Background(), EventBucketsOptions{Window: opts.Window, NameSearch: opts.NameSearch, EventNames: opts.EventNames, Severities: opts.Severities, Components: opts.Components, GPUTypes: opts.GPUTypes, DataSources: opts.DataSources, NodeGroupIDs: opts.NodeGroupIDs, ComputeZoneIDs: opts.ComputeZoneIDs, ExcludeNodeGroupIDs: opts.ExcludeNodeGroupIDs, ExcludeComputeZoneIDs: opts.ExcludeComputeZoneIDs})
						if err != nil {
							t.Fatal(err)
						}
					}
				})
			}
		}
	}
}

func stringSources(sources []DataSource) string { b, _ := json.Marshal(sources); return string(b) }

func TestEventInvalidFiltersMakeNoRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("unexpected request")
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatal(err)
	}
	for _, opts := range []EventListOptions{
		{DataSources: []DataSource{"invalid"}}, {DataSources: []DataSource{""}},
		{Component: "gpu", Components: []string{"gpu"}},
		{NodeGroupIDs: []string{"a"}, ExcludeNodeGroupIDs: []string{"b"}},
		{ComputeZoneIDs: []string{"a"}, ExcludeComputeZoneIDs: []string{"b"}},
	} {
		opts.Window = "24h"
		if _, err := client.ListEvents(context.Background(), opts); err == nil {
			t.Errorf("accepted list filters %#v", opts)
		}
		if _, err := client.GetEventBuckets(context.Background(), EventBucketsOptions{Window: opts.Window, DataSources: opts.DataSources, Component: opts.Component, Components: opts.Components, NodeGroupIDs: opts.NodeGroupIDs, ExcludeNodeGroupIDs: opts.ExcludeNodeGroupIDs, ComputeZoneIDs: opts.ComputeZoneIDs, ExcludeComputeZoneIDs: opts.ExcludeComputeZoneIDs}); err == nil {
			t.Errorf("accepted bucket filters %#v", opts)
		}
	}
	for _, opts := range []EventListOptions{{Window: "24h", SortBy: "dataSource"}, {Window: "24h", SortOrder: "up"}} {
		if _, err := client.ListEvents(context.Background(), opts); err == nil {
			t.Errorf("accepted sort %#v", opts)
		}
	}
}

func TestEventFilterOptions(t *testing.T) {
	body := `{"components":[{"component":"gpu","componentDisplayName":"GPU"}],"dataSources":["inband","oob"],"eventNames":["XID","Others"],"gpuTypes":["H100"],"severities":["Critical"],"sortFields":["timestamp","bmcHostname"]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/events/options" || r.URL.RawQuery != "" {
			t.Errorf("unexpected URL %s", r.URL)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.GetEventFilterOptions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := EventFilterOptions{Components: []EventComponentOption{{Component: "gpu", ComponentDisplayName: "GPU"}}, DataSources: []string{"inband", "oob"}, EventNames: []string{"XID", "Others"}, GPUTypes: []string{"H100"}, Severities: []string{"Critical"}, SortFields: []string{"timestamp", "bmcHostname"}, RawJSON: []byte(body)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("options=%#v; want %#v", got, want)
	}
}
