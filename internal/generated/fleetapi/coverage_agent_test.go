// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package fleetapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

func TestComputeZonesResponseContract(t *testing.T) {
	detailView := GetV1ComputezonesParamsViewDetail
	basicView := GetV1ComputezonesParamsViewBasic
	includeMetrics := false
	ids := []string{"zone-1", "zone-2"}
	sortBy := GetV1ComputezonesParamsSortByName
	order := GetV1ComputezonesParamsOrderDesc
	page, pageSize := 2, 50

	tests := []struct {
		name        string
		params      *GetV1ComputezonesParams
		query       url.Values
		status      int
		contentType string
		body        string
		check       func(*testing.T, *GetV1ComputezonesResponse, error)
	}{
		{
			name: "detail",
			params: &GetV1ComputezonesParams{View: &detailView, IncludeMetrics: &includeMetrics,
				ComputeZoneIds: &ids, SortBy: &sortBy, Order: &order, Page: &page, PageSize: &pageSize},
			query: url.Values{"view": {"detail"}, "includeMetrics": {"false"}, "computeZoneIds": {"zone-1", "zone-2"},
				"sortBy": {"name"}, "order": {"desc"}, "page": {"2"}, "pageSize": {"50"}},
			status: http.StatusOK, contentType: "application/json",
			body: `{"computezones":[{"id":"zone-1","name":"Alpha","nodesCount":7}],"page":2,"pageSize":50,"total":101,"hasMore":true}`,
			check: func(t *testing.T, response *GetV1ComputezonesResponse, err error) {
				if err != nil {
					t.Fatalf("request: %v", err)
				}
				if response.JSON200 == nil {
					t.Fatal("missing JSON200")
				}
				zoneList, err := response.JSON200.AsModelsComputeZonesResponse()
				if err != nil {
					t.Fatalf("decode detail: %v", err)
				}
				if zoneList.Computezones == nil || len(*zoneList.Computezones) != 1 {
					t.Fatalf("detail zones: %+v", zoneList.Computezones)
				}
				zone := (*zoneList.Computezones)[0]
				if zone.Id != "zone-1" || zone.Name == nil || *zone.Name != "Alpha" || zone.NodesCount == nil || *zone.NodesCount != 7 {
					t.Errorf("detail zone: %+v", zone)
				}
				checkComputeZonePage(t, zoneList.Page, zoneList.PageSize, zoneList.Total, zoneList.HasMore, 2, 50, 101, true)
			},
		},
		{
			name: "basic", params: &GetV1ComputezonesParams{View: &basicView},
			query: url.Values{"view": {"basic"}}, status: http.StatusOK, contentType: "application/json",
			body: `{"computezones":[{"id":"zone-basic","name":"Beta"}],"page":1,"pageSize":10,"total":11,"hasMore":true}`,
			check: func(t *testing.T, response *GetV1ComputezonesResponse, err error) {
				if err != nil {
					t.Fatalf("request: %v", err)
				}
				if response.JSON200 == nil {
					t.Fatal("missing JSON200")
				}
				zoneList, err := response.JSON200.AsModelsListComputeZonesResponse()
				if err != nil {
					t.Fatalf("decode basic: %v", err)
				}
				if zoneList.Computezones == nil || len(*zoneList.Computezones) != 1 {
					t.Fatalf("basic zones: %+v", zoneList.Computezones)
				}
				zone := (*zoneList.Computezones)[0]
				if zone.Id != "zone-basic" || zone.Name == nil || *zone.Name != "Beta" {
					t.Errorf("basic zone: %+v", zone)
				}
				checkComputeZonePage(t, zoneList.Page, zoneList.PageSize, zoneList.Total, zoneList.HasMore, 1, 10, 11, true)
			},
		},
		{
			name: "json forbidden", status: http.StatusForbidden, contentType: "application/json",
			body: `{"error":"forbidden","details":"missing permission"}`,
			check: func(t *testing.T, response *GetV1ComputezonesResponse, err error) {
				if err != nil {
					t.Fatalf("request: %v", err)
				}
				if response.JSON200 != nil || response.JSON403 == nil {
					t.Fatalf("unexpected decoded response: %+v", response)
				}
				if response.JSON403.Error == nil || *response.JSON403.Error != "forbidden" || response.JSON403.Details == nil || *response.JSON403.Details != "missing permission" {
					t.Errorf("error response: %+v", response.JSON403)
				}
			},
		},
		{
			name: "malformed json", status: http.StatusOK, contentType: "application/json; charset=utf-8", body: `{"computezones":`,
			check: func(t *testing.T, response *GetV1ComputezonesResponse, err error) {
				if err == nil {
					t.Errorf("expected JSON decoding error, got response %+v", response)
				}
			},
		},
		{
			name: "non-json forbidden", status: http.StatusForbidden, contentType: "text/plain", body: "access denied",
			check: func(t *testing.T, response *GetV1ComputezonesResponse, err error) {
				if err != nil {
					t.Fatalf("request: %v", err)
				}
				if response.JSON403 != nil || response.JSON200 != nil {
					t.Errorf("unexpected decoded response: %+v", response)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/computezones" {
					t.Errorf("request = %s %s", r.Method, r.URL.Path)
				}
				if !reflect.DeepEqual(r.URL.Query(), tt.query) && !(len(r.URL.Query()) == 0 && len(tt.query) == 0) {
					t.Errorf("query = %v, want %v", r.URL.Query(), tt.query)
				}
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(tt.status)
				if _, err := w.Write([]byte(tt.body)); err != nil {
					t.Errorf("write response: %v", err)
				}
			}))
			defer server.Close()
			client, err := NewClientWithResponses(server.URL)
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			response, err := client.GetV1ComputezonesWithResponse(context.Background(), tt.params)
			if err == nil {
				if response == nil {
					t.Fatal("nil response without error")
				}
				if response.StatusCode() != tt.status || string(response.Body) != tt.body {
					t.Errorf("status/body = %d %q, want %d %q", response.StatusCode(), response.Body, tt.status, tt.body)
				}
			}
			tt.check(t, response, err)
		})
	}
}

func TestEventOptionsResponseContract(t *testing.T) {
	tests := []struct {
		name, contentType, body string
		status                  int
		check                   func(*testing.T, *GetV1EventsOptionsResponse, error)
	}{
		{
			name: "success", status: http.StatusOK, contentType: "application/json",
			body: `{"components":[{"component":"gpu","componentDisplayName":"GPU"}],"dataSources":["inband","oob"],"eventNames":["XID","Others"],"gpuTypes":["H100"],"severities":["Critical"],"sortFields":["timestamp","bmcMac"]}`,
			check: func(t *testing.T, response *GetV1EventsOptionsResponse, err error) {
				if err != nil {
					t.Fatalf("request: %v", err)
				}
				options := response.JSON200
				if options == nil || options.Components == nil || len(*options.Components) != 1 {
					t.Fatalf("missing options or component: %+v", options)
				}
				component := (*options.Components)[0]
				if component.Component == nil || *component.Component != "gpu" || component.ComponentDisplayName == nil || *component.ComponentDisplayName != "GPU" {
					t.Errorf("component = %+v", component)
				}
				for _, field := range []struct {
					name string
					got  *[]string
					want []string
				}{
					{"dataSources", options.DataSources, []string{"inband", "oob"}},
					{"eventNames", options.EventNames, []string{"XID", "Others"}},
					{"gpuTypes", options.GpuTypes, []string{"H100"}},
					{"severities", options.Severities, []string{"Critical"}},
					{"sortFields", options.SortFields, []string{"timestamp", "bmcMac"}},
				} {
					if field.got == nil || !reflect.DeepEqual(*field.got, field.want) {
						t.Errorf("%s = %v, want %v", field.name, field.got, field.want)
					}
				}
			},
		},
		{
			name: "unauthorized", status: http.StatusUnauthorized, contentType: "application/json",
			body: `{"error":"unauthorized","details":"missing credentials"}`,
			check: func(t *testing.T, response *GetV1EventsOptionsResponse, err error) {
				if err != nil {
					t.Fatalf("request: %v", err)
				}
				if response.JSON200 != nil || response.JSON401 == nil || response.JSON403 != nil {
					t.Fatalf("unexpected decoded response: %+v", response)
				}
				if response.JSON401.Error == nil || *response.JSON401.Error != "unauthorized" || response.JSON401.Details == nil || *response.JSON401.Details != "missing credentials" {
					t.Errorf("error response: %+v", response.JSON401)
				}
			},
		},
		{
			name: "forbidden", status: http.StatusForbidden, contentType: "application/json",
			body: `{"error":"forbidden","details":"missing permission"}`,
			check: func(t *testing.T, response *GetV1EventsOptionsResponse, err error) {
				if err != nil {
					t.Fatalf("request: %v", err)
				}
				if response.JSON200 != nil || response.JSON403 == nil || response.JSON401 != nil {
					t.Fatalf("unexpected decoded response: %+v", response)
				}
				if response.JSON403.Error == nil || *response.JSON403.Error != "forbidden" || response.JSON403.Details == nil || *response.JSON403.Details != "missing permission" {
					t.Errorf("error response: %+v", response.JSON403)
				}
			},
		},
		{
			name: "malformed json", status: http.StatusOK, contentType: "application/json", body: `{"components":`,
			check: func(t *testing.T, response *GetV1EventsOptionsResponse, err error) {
				if err == nil {
					t.Errorf("expected JSON decoding error, got response %+v", response)
				}
			},
		},
		{
			name: "plain text", status: http.StatusOK, contentType: "text/plain", body: "options unavailable",
			check: func(t *testing.T, response *GetV1EventsOptionsResponse, err error) {
				if err != nil {
					t.Fatalf("request: %v", err)
				}
				if response.JSON200 != nil {
					t.Errorf("unexpected decoded response: %+v", response.JSON200)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/events/options" || r.URL.RawQuery != "" || r.URL.ForceQuery {
					t.Errorf("request = %s %s", r.Method, r.URL.String())
				}
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(tt.status)
				if _, err := w.Write([]byte(tt.body)); err != nil {
					t.Errorf("write response: %v", err)
				}
			}))
			defer server.Close()
			client, err := NewClientWithResponses(server.URL)
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			response, err := client.GetV1EventsOptionsWithResponse(context.Background())
			if err == nil {
				if response == nil {
					t.Fatal("nil response without error")
				}
				if response.StatusCode() != tt.status || string(response.Body) != tt.body {
					t.Errorf("status/body = %d %q, want %d %q", response.StatusCode(), response.Body, tt.status, tt.body)
				}
			}
			tt.check(t, response, err)
		})
	}
}

func checkComputeZonePage(t *testing.T, page, pageSize, total *int, hasMore *bool, wantPage, wantPageSize, wantTotal int, wantHasMore bool) {
	t.Helper()
	if page == nil || *page != wantPage || pageSize == nil || *pageSize != wantPageSize || total == nil || *total != wantTotal || hasMore == nil || *hasMore != wantHasMore {
		t.Errorf("pagination = page %v, pageSize %v, total %v, hasMore %v", page, pageSize, total, hasMore)
	}
}

func TestNodeGroupsResponseContract(t *testing.T) {
	detailView, basicView := GetV1NodegroupsParamsViewDetail, GetV1NodegroupsParamsViewBasic
	includeMetrics := false
	zoneIDs := []string{"zone-1", "zone-2"}
	zoneNames := []string{"east", "west"}
	groupIDs := []string{"group-1", "group-2"}
	healthStatuses := []GetV1NodegroupsParamsHealthStatuses{GetV1NodegroupsParamsHealthStatusesHealthy, GetV1NodegroupsParamsHealthStatusesDegraded}
	gpuTypes := []string{"H100", "A100"}
	healthSort, nameSort := GetV1NodegroupsParamsSortByHealth, GetV1NodegroupsParamsSortByName
	order := GetV1NodegroupsParamsOrderAsc
	page, pageSize := 2, 50

	tests := []struct {
		name        string
		params      *GetV1NodegroupsParams
		query       url.Values
		status      int
		contentType string
		body        string
		check       func(*testing.T, *GetV1NodegroupsResponse, error)
	}{
		{
			name: "detail",
			params: &GetV1NodegroupsParams{View: &detailView, IncludeMetrics: &includeMetrics,
				ComputeZoneIds: &zoneIDs, ComputeZoneNames: &zoneNames, NodeGroupIds: &groupIDs,
				HealthStatuses: &healthStatuses, GpuTypes: &gpuTypes, SortBy: &healthSort,
				Order: &order, Page: &page, PageSize: &pageSize},
			query: url.Values{"view": {"detail"}, "includeMetrics": {"false"},
				"computeZoneIds": {"zone-1", "zone-2"}, "computeZoneNames": {"east", "west"},
				"nodeGroupIds": {"group-1", "group-2"}, "healthStatuses": {"Healthy", "Degraded"},
				"gpuTypes": {"H100", "A100"}, "sortBy": {"health"}, "order": {"asc"}, "page": {"2"}, "pageSize": {"50"}},
			status: http.StatusOK, contentType: "application/json",
			body: `{"nodeGroups":[{"id":"group-1","name":"Alpha","computeZoneId":"zone-1","nodesCount":7,"hasInbandAgent":true,"hasOobAgent":false}],"page":2,"pageSize":50,"total":101,"hasMore":true}`,
			check: func(t *testing.T, response *GetV1NodegroupsResponse, err error) {
				if err != nil {
					t.Fatalf("request: %v", err)
				}
				if response.JSON200 == nil {
					t.Fatal("missing JSON200")
				}
				groups, err := response.JSON200.AsModelsNodeGroupsResponse()
				if err != nil {
					t.Fatalf("decode detail: %v", err)
				}
				if groups.NodeGroups == nil || len(*groups.NodeGroups) != 1 {
					t.Fatalf("detail groups: %+v", groups.NodeGroups)
				}
				group := (*groups.NodeGroups)[0]
				if group.Id != "group-1" || group.Name == nil || *group.Name != "Alpha" || group.ComputeZoneId == nil || *group.ComputeZoneId != "zone-1" || group.NodesCount == nil || *group.NodesCount != 7 || group.HasInbandAgent == nil || !*group.HasInbandAgent || group.HasOobAgent == nil || *group.HasOobAgent {
					t.Errorf("detail group: %+v", group)
				}
				checkComputeZonePage(t, groups.Page, groups.PageSize, groups.Total, groups.HasMore, 2, 50, 101, true)
			},
		},
		{
			name: "basic", params: &GetV1NodegroupsParams{View: &basicView, ComputeZoneIds: &zoneIDs, NodeGroupIds: &groupIDs, SortBy: &nameSort},
			query:  url.Values{"view": {"basic"}, "computeZoneIds": {"zone-1", "zone-2"}, "nodeGroupIds": {"group-1", "group-2"}, "sortBy": {"name"}},
			status: http.StatusOK, contentType: "application/json",
			body: `{"nodeGroups":[{"id":"group-basic","name":"Beta","computeZoneId":"zone-basic"}],"page":1,"pageSize":10,"total":11,"hasMore":true}`,
			check: func(t *testing.T, response *GetV1NodegroupsResponse, err error) {
				if err != nil {
					t.Fatalf("request: %v", err)
				}
				if response.JSON200 == nil {
					t.Fatal("missing JSON200")
				}
				groups, err := response.JSON200.AsModelsListNodeGroupsResponse()
				if err != nil {
					t.Fatalf("decode basic: %v", err)
				}
				if groups.NodeGroups == nil || len(*groups.NodeGroups) != 1 {
					t.Fatalf("basic groups: %+v", groups.NodeGroups)
				}
				group := (*groups.NodeGroups)[0]
				if group.Id != "group-basic" || group.Name == nil || *group.Name != "Beta" || group.ComputeZoneId == nil || *group.ComputeZoneId != "zone-basic" {
					t.Errorf("basic group: %+v", group)
				}
				checkComputeZonePage(t, groups.Page, groups.PageSize, groups.Total, groups.HasMore, 1, 10, 11, true)
			},
		},
		{
			name: "json forbidden", status: http.StatusForbidden, contentType: "application/json",
			body: `{"error":"forbidden","details":"missing permission"}`,
			check: func(t *testing.T, response *GetV1NodegroupsResponse, err error) {
				if err != nil {
					t.Fatalf("request: %v", err)
				}
				if response.JSON200 != nil || response.JSON403 == nil {
					t.Fatalf("unexpected decoded response: %+v", response)
				}
				if response.JSON403.Error == nil || *response.JSON403.Error != "forbidden" || response.JSON403.Details == nil || *response.JSON403.Details != "missing permission" {
					t.Errorf("error response: %+v", response.JSON403)
				}
			},
		},
		{
			name: "malformed json", status: http.StatusOK, contentType: "application/json", body: `{"nodeGroups":`,
			check: func(t *testing.T, response *GetV1NodegroupsResponse, err error) {
				if err == nil {
					t.Errorf("expected JSON decoding error, got response %+v", response)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/nodegroups" {
					t.Errorf("request = %s %s", r.Method, r.URL.Path)
				}
				if !reflect.DeepEqual(r.URL.Query(), tt.query) && !(len(r.URL.Query()) == 0 && len(tt.query) == 0) {
					t.Errorf("query = %v, want %v", r.URL.Query(), tt.query)
				}
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(tt.status)
				if _, err := w.Write([]byte(tt.body)); err != nil {
					t.Errorf("write response: %v", err)
				}
			}))
			defer server.Close()
			client, err := NewClientWithResponses(server.URL)
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			response, err := client.GetV1NodegroupsWithResponse(context.Background(), tt.params)
			if err == nil {
				if response == nil {
					t.Fatal("nil response without error")
				}
				if response.StatusCode() != tt.status || string(response.Body) != tt.body {
					t.Errorf("status/body = %d %q, want %d %q", response.StatusCode(), response.Body, tt.status, tt.body)
				}
			}
			tt.check(t, response, err)
		})
	}
}

func TestAlertsResponseContract(t *testing.T) {
	node, component, state, severity := "node-1", "GPU & fan", "Triggered", "Critical"
	page, pageSize := 0, 25
	params := &GetV1AlertsParams{NodeUUID: &node, Component: &component, State: &state, Severity: &severity, Page: &page, PageSize: &pageSize}
	query := url.Values{"nodeUUID": {node}, "component": {component}, "state": {state}, "severity": {severity}, "page": {"0"}, "pageSize": {"25"}}

	tests := []struct {
		name, contentType, body string
		status                  int
		params                  *GetV1AlertsParams
		query                   url.Values
		check                   func(*testing.T, *GetV1AlertsResponse, error)
	}{
		{
			name: "success", params: params, query: query, status: http.StatusOK, contentType: "application/json",
			body: `{"alerts":[{"alertUUID":"alert-1","nodeUUID":"node-1","component":"GPU & fan","dataSource":"inband","severity":"Critical","state":"Triggered","isMuted":true}],"page":0,"pageSize":25,"total":42,"pageCursorNext":"next-cursor"}`,
			check: func(t *testing.T, response *GetV1AlertsResponse, err error) {
				if err != nil {
					t.Fatalf("request: %v", err)
				}
				if response.JSON200 == nil || response.JSON200.Alerts == nil || len(*response.JSON200.Alerts) != 1 {
					t.Fatalf("missing alert: %+v", response.JSON200)
				}
				alert := (*response.JSON200.Alerts)[0]
				if alert.AlertUUID != "alert-1" || alert.NodeUUID != node || alert.Component == nil || *alert.Component != component || alert.DataSource == nil || *alert.DataSource != "inband" || alert.Severity == nil || string(*alert.Severity) != severity || alert.State == nil || string(*alert.State) != state || alert.IsMuted == nil || !*alert.IsMuted {
					t.Errorf("alert = %+v", alert)
				}
				result := response.JSON200
				if result.Page == nil || *result.Page != 0 || result.PageSize == nil || *result.PageSize != 25 || result.Total == nil || *result.Total != 42 || result.PageCursorNext == nil || *result.PageCursorNext != "next-cursor" {
					t.Errorf("pagination = %+v", result)
				}
			},
		},
		{
			name: "json bad request", status: http.StatusBadRequest, contentType: "application/json", body: `{"error":"bad request","details":"invalid filter"}`,
			check: func(t *testing.T, response *GetV1AlertsResponse, err error) {
				if err != nil {
					t.Fatalf("request: %v", err)
				}
				if response.JSON200 != nil || response.JSON400 == nil || response.JSON499 != nil {
					t.Fatalf("unexpected decoded response: %+v", response)
				}
				if response.JSON400.Error == nil || *response.JSON400.Error != "bad request" || response.JSON400.Details == nil || *response.JSON400.Details != "invalid filter" {
					t.Errorf("error response: %+v", response.JSON400)
				}
			},
		},
		{
			name: "json 499", status: 499, contentType: "application/json", body: `{"error":"cancelled","details":"client closed request"}`,
			check: func(t *testing.T, response *GetV1AlertsResponse, err error) {
				if err != nil {
					t.Fatalf("request: %v", err)
				}
				if response.JSON200 != nil || response.JSON499 == nil || response.JSON400 != nil {
					t.Fatalf("unexpected decoded response: %+v", response)
				}
				if response.JSON499.Error == nil || *response.JSON499.Error != "cancelled" || response.JSON499.Details == nil || *response.JSON499.Details != "client closed request" {
					t.Errorf("error response: %+v", response.JSON499)
				}
			},
		},
		{
			name: "malformed json", status: http.StatusOK, contentType: "application/json", body: `{"alerts":`,
			check: func(t *testing.T, response *GetV1AlertsResponse, err error) {
				if err == nil {
					t.Errorf("expected JSON decoding error, got response %+v", response)
				}
			},
		},
		{
			name: "non-json error", status: http.StatusBadRequest, contentType: "text/plain", body: "invalid request",
			check: func(t *testing.T, response *GetV1AlertsResponse, err error) {
				if err != nil {
					t.Fatalf("request: %v", err)
				}
				if response.JSON200 != nil || response.JSON400 != nil || response.JSON499 != nil || response.JSON500 != nil {
					t.Errorf("unexpected decoded response: %+v", response)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/alerts" {
					t.Errorf("request = %s %s", r.Method, r.URL.Path)
				}
				if !reflect.DeepEqual(r.URL.Query(), tt.query) && !(len(r.URL.Query()) == 0 && len(tt.query) == 0) {
					t.Errorf("query = %v, want %v", r.URL.Query(), tt.query)
				}
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(tt.status)
				if _, err := w.Write([]byte(tt.body)); err != nil {
					t.Errorf("write response: %v", err)
				}
			}))
			defer server.Close()
			client, err := NewClientWithResponses(server.URL)
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			response, err := client.GetV1AlertsWithResponse(context.Background(), tt.params)
			if err == nil {
				if response == nil {
					t.Fatal("nil response without error")
				}
				if response.StatusCode() != tt.status || string(response.Body) != tt.body {
					t.Errorf("status/body = %d %q, want %d %q", response.StatusCode(), response.Body, tt.status, tt.body)
				}
			}
			tt.check(t, response, err)
		})
	}
}

func TestEventsResponseContract(t *testing.T) {
	mode := GetV1EventsParamsTimeModeRelative
	window, search := "24h", "rack & fan"
	eventNames := []string{"overheat", "recovered"}
	components := []string{"gpu", "fan"}
	sources := []GetV1EventsParamsDataSources{GetV1EventsParamsDataSourcesInband, GetV1EventsParamsDataSourcesOob}
	excluded := []string{"group-1", "group-2"}
	sortBy, sortOrder := GetV1EventsParamsSortByTimestamp, GetV1EventsParamsSortOrderDesc
	page, pageSize := 0, 25
	params := &GetV1EventsParams{TimeMode: &mode, Window: &window, EventNames: &eventNames, Components: &components, DataSources: &sources, ExcludeNodeGroupIds: &excluded, NameSearch: &search, SortBy: &sortBy, SortOrder: &sortOrder, Page: &page, PageSize: &pageSize}
	query := url.Values{"timeMode": {"relative"}, "window": {"24h"}, "eventNames": {"overheat", "recovered"}, "components": {"gpu", "fan"}, "dataSources": {"inband", "oob"}, "excludeNodeGroupIds": {"group-1", "group-2"}, "nameSearch": {"rack & fan"}, "sortBy": {"timestamp"}, "sortOrder": {"desc"}, "page": {"0"}, "pageSize": {"25"}}

	tests := []struct {
		name, contentType, body string
		status                  int
		params                  *GetV1EventsParams
		query                   url.Values
		check                   func(*testing.T, *GetV1EventsResponse, error)
	}{
		{
			name: "success", contentType: "application/json", status: http.StatusOK, params: params, query: query,
			body: `{"events":[{"eventId":"event-1","nodeUUID":"node-1","dataSource":"inband","name":"overheat","timestamp":"2024-01-01T00:00:00Z"}],"page":0,"pageSize":25,"total":42,"hasMore":true}`,
			check: func(t *testing.T, response *GetV1EventsResponse, err error) {
				if err != nil {
					t.Fatalf("request: %v", err)
				}
				if response.JSON200 == nil || response.JSON200.Events == nil || len(*response.JSON200.Events) != 1 {
					t.Fatalf("missing event: %+v", response.JSON200)
				}
				event := (*response.JSON200.Events)[0]
				if event.EventId != "event-1" || event.NodeUUID != "node-1" || event.DataSource == nil || *event.DataSource != "inband" || event.Name == nil || *event.Name != "overheat" || event.Timestamp == nil || *event.Timestamp != "2024-01-01T00:00:00Z" {
					t.Errorf("event = %+v", event)
				}
				result := response.JSON200
				if result.Page == nil || *result.Page != 0 || result.PageSize == nil || *result.PageSize != 25 || result.Total == nil || *result.Total != 42 || result.HasMore == nil || !*result.HasMore {
					t.Errorf("pagination = %+v", result)
				}
			},
		},
		{
			name: "json forbidden", contentType: "application/json", status: http.StatusForbidden, body: `{"error":"forbidden","details":"access denied"}`,
			check: func(t *testing.T, response *GetV1EventsResponse, err error) {
				if err != nil {
					t.Fatalf("request: %v", err)
				}
				if response.JSON200 != nil || response.JSON403 == nil {
					t.Fatalf("unexpected decoded response: %+v", response)
				}
				if response.JSON403.Error == nil || *response.JSON403.Error != "forbidden" || response.JSON403.Details == nil || *response.JSON403.Details != "access denied" {
					t.Errorf("error response: %+v", response.JSON403)
				}
			},
		},
		{
			name: "malformed json", contentType: "application/json", status: http.StatusOK, body: `{"events":`,
			check: func(t *testing.T, response *GetV1EventsResponse, err error) {
				if err == nil || response != nil {
					t.Errorf("expected decoding error and nil response, got response %+v, error %v", response, err)
				}
			},
		},
		{
			name: "non-json forbidden", contentType: "text/plain", status: http.StatusForbidden, body: "access denied",
			check: func(t *testing.T, response *GetV1EventsResponse, err error) {
				if err != nil {
					t.Fatalf("request: %v", err)
				}
				if response.JSON200 != nil || response.JSON400 != nil || response.JSON401 != nil || response.JSON403 != nil || response.JSON499 != nil || response.JSON500 != nil || response.JSON503 != nil {
					t.Errorf("unexpected decoded response: %+v", response)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/events" {
					t.Errorf("request = %s %s", r.Method, r.URL.Path)
				}
				if !reflect.DeepEqual(r.URL.Query(), tt.query) && !(len(r.URL.Query()) == 0 && len(tt.query) == 0) {
					t.Errorf("query = %v, want %v", r.URL.Query(), tt.query)
				}
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(tt.status)
				if _, err := w.Write([]byte(tt.body)); err != nil {
					t.Errorf("write response: %v", err)
				}
			}))
			defer server.Close()
			client, err := NewClientWithResponses(server.URL)
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			response, err := client.GetV1EventsWithResponse(context.Background(), tt.params)
			if err == nil {
				if response == nil {
					t.Fatal("nil response without error")
				}
				if response.StatusCode() != tt.status || string(response.Body) != tt.body {
					t.Errorf("status/body = %d %q, want %d %q", response.StatusCode(), response.Body, tt.status, tt.body)
				}
			}
			tt.check(t, response, err)
		})
	}
}

func TestNodesResponseContract(t *testing.T) {
	detail, basic := GetV1NodesParamsViewDetail, GetV1NodesParamsViewBasic
	inband, oob := GetV1NodesParamsDataSourceInband, GetV1NodesParamsDataSourceOob
	hostSort, nameSort, gpuSort := GetV1NodesParamsSortByHostname, GetV1NodesParamsSortByNodeName, GetV1NodesParamsSortByGpuCount
	desc := GetV1NodesParamsOrderDesc
	agents := []string{"agent-1", "agent-2"}
	systems := []string{"system-1", "system-2"}
	nodes := []string{"node-1", "node-2"}
	gpuCounts := []int{0, 8}
	page, pageSize := 2, 50
	hostname, search, nodeName, bmcMac := "host", "rack & host", "rack", "aa:bb"

	tests := []struct {
		name, contentType, body string
		status                  int
		params                  *GetV1NodesParams
		query                   url.Values
		check                   func(*testing.T, *GetV1NodesResponse, error)
	}{
		{
			name: "inband detail", status: http.StatusOK, contentType: "application/json",
			params: &GetV1NodesParams{View: &detail, DataSource: &inband, AgentUUIDs: &agents, SystemUUIDs: &systems, Page: &page, PageSize: &pageSize, Hostname: &hostname, Search: &search, SortBy: &hostSort},
			query:  url.Values{"view": {"detail"}, "dataSource": {"inband"}, "agentUUIDs": {"agent-1", "agent-2"}, "systemUUIDs": {"system-1", "system-2"}, "page": {"2"}, "pageSize": {"50"}, "hostname": {"host"}, "search": {"rack & host"}, "sortBy": {"hostname"}},
			body:   `{"nodes":[{"nodeUUID":"node-inband","dataSource":"inband","agentUUID":"agent-1","hostname":"host-1","gpuCount":0}],"page":2,"pageSize":50,"total":101,"hasMore":true}`,
			check: func(t *testing.T, response *GetV1NodesResponse, err error) {
				if err != nil {
					t.Fatalf("request: %v", err)
				}
				if response.JSON200 == nil {
					t.Fatal("missing JSON200")
				}
				result, err := response.JSON200.AsModelsInbandNodesResponse()
				if err != nil {
					t.Fatalf("decode inband: %v", err)
				}
				if result.Nodes == nil || len(*result.Nodes) != 1 {
					t.Fatalf("inband nodes: %+v", result.Nodes)
				}
				node := (*result.Nodes)[0]
				if node.NodeUUID != "node-inband" || node.DataSource != "inband" || node.AgentUUID == nil || *node.AgentUUID != "agent-1" || node.Hostname == nil || *node.Hostname != "host-1" || node.GpuCount == nil || *node.GpuCount != 0 {
					t.Errorf("inband node: %+v", node)
				}
				checkComputeZonePage(t, result.Page, result.PageSize, result.Total, result.HasMore, 2, 50, 101, true)
			},
		},
		{
			name: "oob detail", status: http.StatusOK, contentType: "application/json",
			params: &GetV1NodesParams{View: &detail, DataSource: &oob, SystemUUIDs: &systems, NodeName: &nodeName, BmcMac: &bmcMac, GpuCounts: &gpuCounts, SortBy: &nameSort, Order: &desc},
			query:  url.Values{"view": {"detail"}, "dataSource": {"oob"}, "systemUUIDs": {"system-1", "system-2"}, "nodeName": {"rack"}, "bmcMac": {"aa:bb"}, "gpuCounts": {"0", "8"}, "sortBy": {"nodeName"}, "order": {"desc"}},
			body:   `{"nodes":[{"nodeUUID":"node-oob","dataSource":"oob","nodeName":"rack-1","bmcMac":"aa:bb","bmcIP":"192.0.2.1","gpuCount":8}],"page":0,"pageSize":10,"total":1,"hasMore":false}`,
			check: func(t *testing.T, response *GetV1NodesResponse, err error) {
				if err != nil {
					t.Fatalf("request: %v", err)
				}
				if response.JSON200 == nil {
					t.Fatal("missing JSON200")
				}
				result, err := response.JSON200.AsModelsOobNodesResponse()
				if err != nil {
					t.Fatalf("decode oob: %v", err)
				}
				if result.Nodes == nil || len(*result.Nodes) != 1 {
					t.Fatalf("oob nodes: %+v", result.Nodes)
				}
				node := (*result.Nodes)[0]
				if node.NodeUUID != "node-oob" || node.DataSource != "oob" || node.NodeName == nil || *node.NodeName != "rack-1" || node.BmcMac == nil || *node.BmcMac != "aa:bb" || node.BmcIP == nil || *node.BmcIP != "192.0.2.1" || node.GpuCount == nil || *node.GpuCount != 8 {
					t.Errorf("oob node: %+v", node)
				}
				checkComputeZonePage(t, result.Page, result.PageSize, result.Total, result.HasMore, 0, 10, 1, false)
			},
		},
		{
			name: "combined basic", status: http.StatusOK, contentType: "application/json",
			params: &GetV1NodesParams{View: &basic, NodeUUIDs: &nodes, Search: &search, SortBy: &gpuSort},
			query:  url.Values{"view": {"basic"}, "nodeUUIDs": {"node-1", "node-2"}, "search": {"rack & host"}, "sortBy": {"gpuCount"}},
			body:   `{"nodes":[{"nodeUUID":"node-1","agentUUID":"agent-1","hostname":"host-1","nodeName":"rack-1","bmcMac":"aa:bb","bmcIP":"192.0.2.1"}],"page":0,"pageSize":10,"total":1,"hasMore":false}`,
			check: func(t *testing.T, response *GetV1NodesResponse, err error) {
				if err != nil {
					t.Fatalf("request: %v", err)
				}
				if response.JSON200 == nil {
					t.Fatal("missing JSON200")
				}
				result, err := response.JSON200.AsModelsBasicNodesResponse()
				if err != nil {
					t.Fatalf("decode basic: %v", err)
				}
				if result.Nodes == nil || len(*result.Nodes) != 1 {
					t.Fatalf("basic nodes: %+v", result.Nodes)
				}
				node := (*result.Nodes)[0]
				if node.NodeUUID != "node-1" || node.AgentUUID == nil || *node.AgentUUID != "agent-1" || node.Hostname == nil || *node.Hostname != "host-1" || node.NodeName == nil || *node.NodeName != "rack-1" || node.BmcMac == nil || *node.BmcMac != "aa:bb" || node.BmcIP == nil || *node.BmcIP != "192.0.2.1" {
					t.Errorf("basic node: %+v", node)
				}
				checkComputeZonePage(t, result.Page, result.PageSize, result.Total, result.HasMore, 0, 10, 1, false)
			},
		},
		{
			name: "json forbidden", status: http.StatusForbidden, contentType: "application/json", body: `{"error":"forbidden","details":"missing permission"}`,
			check: func(t *testing.T, response *GetV1NodesResponse, err error) {
				if err != nil {
					t.Fatalf("request: %v", err)
				}
				if response.JSON200 != nil || response.JSON403 == nil {
					t.Fatalf("unexpected decoded response: %+v", response)
				}
				if response.JSON403.Error == nil || *response.JSON403.Error != "forbidden" || response.JSON403.Details == nil || *response.JSON403.Details != "missing permission" {
					t.Errorf("error response: %+v", response.JSON403)
				}
			},
		},
		{
			name: "malformed json", status: http.StatusOK, contentType: "application/json", body: `{"nodes":`,
			check: func(t *testing.T, response *GetV1NodesResponse, err error) {
				if err == nil {
					t.Errorf("expected JSON decoding error, got response %+v", response)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/nodes" {
					t.Errorf("request = %s %s", r.Method, r.URL.Path)
				}
				if !reflect.DeepEqual(r.URL.Query(), tt.query) && !(len(r.URL.Query()) == 0 && len(tt.query) == 0) {
					t.Errorf("query = %v, want %v", r.URL.Query(), tt.query)
				}
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(tt.status)
				if _, err := w.Write([]byte(tt.body)); err != nil {
					t.Errorf("write response: %v", err)
				}
			}))
			defer server.Close()
			client, err := NewClientWithResponses(server.URL)
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			response, err := client.GetV1NodesWithResponse(context.Background(), tt.params)
			if err == nil {
				if response == nil {
					t.Fatal("nil response without error")
				}
				if response.StatusCode() != tt.status || string(response.Body) != tt.body {
					t.Errorf("status/body = %d %q, want %d %q", response.StatusCode(), response.Body, tt.status, tt.body)
				}
			}
			tt.check(t, response, err)
		})
	}
}

func TestXIDBurstsResponseContract(t *testing.T) {
	listSource, listMode := GetV1XIDBurstsParamsDataSourceAll, GetV1XIDBurstsParamsTimeModeRelative
	optionsSource, optionsMode := GetV1XIDBurstOptionsParamsDataSourceAll, GetV1XIDBurstOptionsParamsTimeModeRelative
	window, search := "24h", "rack & gpu/?=+"
	xids, gpuTypes := []int{31, 79}, []string{"H100", "A100"}
	excluded := []string{"group-1", "group-2"}
	disruption := false
	sortBy, sortOrder := GetV1XIDBurstsParamsSortByStartTime, GetV1XIDBurstsParamsSortOrderDesc
	page, pageSize := 2, 25
	listParams := &GetV1XIDBurstsParams{DataSource: &listSource, TimeMode: &listMode, Window: &window, XidNumbers: &xids, GpuTypes: &gpuTypes, JobDisruption: &disruption, NameSearch: &search, SortBy: &sortBy, SortOrder: &sortOrder, Page: &page, PageSize: &pageSize}
	optionsParams := &GetV1XIDBurstOptionsParams{DataSource: &optionsSource, TimeMode: &optionsMode, Window: &window, ExcludeNodeGroupIds: &excluded}

	tests := []struct {
		name, path, contentType, body string
		query                         url.Values
		status                        int
	}{
		{name: "list", path: "/v1/xid/bursts", query: url.Values{"dataSource": {"all"}, "timeMode": {"relative"}, "window": {"24h"}, "xidNumbers": {"31", "79"}, "gpuTypes": {"H100", "A100"}, "jobDisruption": {"false"}, "nameSearch": {"rack & gpu/?=+"}, "sortBy": {"startTime"}, "sortOrder": {"desc"}, "page": {"2"}, "pageSize": {"25"}}, status: http.StatusOK, contentType: "application/json", body: `{"items":[{"burstId":"burst-1","nodeUuid":"node-1","dataSource":"oob","nodeName":"rack-1","bmcMac":"aa:bb","deviceIds":{"0000:01:00.0":[31,79]},"jobDisruption":false}],"page":2,"pageSize":25,"total":42}`},
		{name: "options", path: "/v1/xid/bursts/options", query: url.Values{"dataSource": {"all"}, "timeMode": {"relative"}, "window": {"24h"}, "excludeNodeGroupIds": {"group-1", "group-2"}}, status: http.StatusOK, contentType: "application/json", body: `{"dataSource":"all","gpuTypes":["H100","A100"],"xidNumbers":[31,79],"jobDisruption":[true,false]}`},
		{name: "detail", path: "/v1/xid/bursts/burst-1", status: http.StatusOK, contentType: "application/json", body: `{"burstId":"burst-1","nodeUuid":"node-1","dataSource":"oob","nodeName":"rack-1","bmcMac":"aa:bb","deviceIds":{"0000:01:00.0":[31,79]},"jobDisruption":true}`},
		{name: "list forbidden", path: "/v1/xid/bursts", status: http.StatusForbidden, contentType: "application/json", body: `{"error":"forbidden","details":"access denied"}`},
		{name: "detail not found", path: "/v1/xid/bursts/burst-1", status: http.StatusNotFound, contentType: "application/json", body: `{"error":"not found","details":"unknown burst"}`},
		{name: "malformed list", path: "/v1/xid/bursts", status: http.StatusOK, contentType: "application/json", body: `{"items":`},
		{name: "plain forbidden", path: "/v1/xid/bursts", status: http.StatusForbidden, contentType: "text/plain", body: "access denied"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != tt.path {
					t.Errorf("request = %s %s, want GET %s", r.Method, r.URL.Path, tt.path)
				}
				if !reflect.DeepEqual(r.URL.Query(), tt.query) && !(len(r.URL.Query()) == 0 && len(tt.query) == 0) {
					t.Errorf("query = %v, want %v", r.URL.Query(), tt.query)
				}
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(tt.status)
				if _, err := w.Write([]byte(tt.body)); err != nil {
					t.Errorf("write response: %v", err)
				}
			}))
			defer server.Close()
			client, err := NewClientWithResponses(server.URL)
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			switch tt.name {
			case "options":
				response, err := client.GetV1XIDBurstOptionsWithResponse(context.Background(), optionsParams)
				if err != nil || response == nil {
					t.Fatalf("options response = %+v, error = %v", response, err)
				}
				if response.StatusCode() != tt.status || string(response.Body) != tt.body {
					t.Errorf("status/body = %d %q", response.StatusCode(), response.Body)
				}
				result := response.JSON200
				if result == nil || result.DataSource == nil || *result.DataSource != "all" || result.GpuTypes == nil || !reflect.DeepEqual(*result.GpuTypes, []string{"H100", "A100"}) || result.XidNumbers == nil || !reflect.DeepEqual(*result.XidNumbers, []int{31, 79}) || result.JobDisruption == nil || !reflect.DeepEqual(*result.JobDisruption, []bool{true, false}) {
					t.Errorf("options = %+v", result)
				}
			case "detail", "detail not found":
				response, err := client.GetV1XIDBurstDetailWithResponse(context.Background(), "burst-1")
				if err != nil || response == nil {
					t.Fatalf("detail response = %+v, error = %v", response, err)
				}
				if response.StatusCode() != tt.status || string(response.Body) != tt.body {
					t.Errorf("status/body = %d %q", response.StatusCode(), response.Body)
				}
				if tt.name == "detail not found" {
					if response.JSON200 != nil || response.JSON404 == nil || response.JSON404.Error == nil || *response.JSON404.Error != "not found" || response.JSON404.Details == nil || *response.JSON404.Details != "unknown burst" {
						t.Errorf("detail error = %+v", response)
					}
				} else {
					result := response.JSON200
					if result == nil || result.BurstId != "burst-1" || result.NodeUuid != "node-1" || result.DataSource == nil || *result.DataSource != "oob" || result.NodeName == nil || *result.NodeName != "rack-1" || result.BmcMac == nil || *result.BmcMac != "aa:bb" || result.DeviceIds == nil || !reflect.DeepEqual(*result.DeviceIds, map[string][]int{"0000:01:00.0": {31, 79}}) || result.JobDisruption == nil || !*result.JobDisruption {
						t.Errorf("detail = %+v", result)
					}
				}
			default:
				var params *GetV1XIDBurstsParams
				if tt.name == "list" {
					params = listParams
				}
				response, err := client.GetV1XIDBurstsWithResponse(context.Background(), params)
				if tt.name == "malformed list" {
					if err == nil || response != nil {
						t.Errorf("expected decoding error and nil response, got %+v, %v", response, err)
					}
					return
				}
				if err != nil || response == nil {
					t.Fatalf("list response = %+v, error = %v", response, err)
				}
				if response.StatusCode() != tt.status || string(response.Body) != tt.body {
					t.Errorf("status/body = %d %q", response.StatusCode(), response.Body)
				}
				switch tt.name {
				case "list":
					result := response.JSON200
					if result == nil || result.Items == nil || len(*result.Items) != 1 {
						t.Fatalf("list = %+v", result)
					}
					item := (*result.Items)[0]
					if item.BurstId != "burst-1" || item.NodeUuid != "node-1" || item.DataSource == nil || *item.DataSource != "oob" || item.NodeName == nil || *item.NodeName != "rack-1" || item.BmcMac == nil || *item.BmcMac != "aa:bb" || item.DeviceIds == nil || !reflect.DeepEqual(*item.DeviceIds, map[string][]int{"0000:01:00.0": {31, 79}}) || item.JobDisruption == nil || *item.JobDisruption {
						t.Errorf("item = %+v", item)
					}
					if result.Page == nil || *result.Page != 2 || result.PageSize == nil || *result.PageSize != 25 || result.Total == nil || *result.Total != 42 {
						t.Errorf("pagination = %+v", result)
					}
				case "list forbidden":
					if response.JSON200 != nil || response.JSON403 == nil || response.JSON403.Error == nil || *response.JSON403.Error != "forbidden" || response.JSON403.Details == nil || *response.JSON403.Details != "access denied" {
						t.Errorf("list error = %+v", response)
					}
				case "plain forbidden":
					if response.JSON200 != nil || response.JSON403 != nil {
						t.Errorf("unexpected decoded response: %+v", response)
					}
				}
			}
		})
	}
}

func TestInventoryReportResponseContract(t *testing.T) {
	inband, oob := GetV1ReportsInventoryParamsDataSourceInband, GetV1ReportsInventoryParamsDataSourceOob
	jsonFormat, csvFormat := GetV1ReportsInventoryParamsFormatJson, GetV1ReportsInventoryParamsFormatCsv
	hostnameSort, macSort := GetV1ReportsInventoryParamsSortByHostname, GetV1ReportsInventoryParamsSortByBmcMac
	asc, desc := GetV1ReportsInventoryParamsOrderAsc, GetV1ReportsInventoryParamsOrderDesc
	page, pageSize := 2, 25
	zones, tags := []string{"zone-1", "zone-2"}, []string{"rack=one", "gpu & fan"}

	tests := []struct {
		name, contentType, body string
		params                  *GetV1ReportsInventoryParams
		query                   url.Values
		status                  int
		check                   func(*testing.T, *GetV1ReportsInventoryResponse)
	}{
		{
			name: "inband json", contentType: "application/json; charset=utf-8", status: http.StatusOK,
			params: &GetV1ReportsInventoryParams{DataSource: &inband, Format: &jsonFormat, Page: &page, PageSize: &pageSize, ComputeZoneIds: &zones, Tags: &tags, SortBy: &hostnameSort, Order: &asc},
			query:  url.Values{"dataSource": {"inband"}, "format": {"json"}, "page": {"2"}, "pageSize": {"25"}, "computeZoneIds": {"zone-1", "zone-2"}, "tags": {"rack=one", "gpu & fan"}, "sortBy": {"hostname"}, "order": {"asc"}},
			body:   `{"nodes":[{"nodeUUID":"inband-1","hostname":"host-1","gpuCount":8}],"page":2,"pageSize":25,"total":51,"hasMore":true}`,
			check: func(t *testing.T, response *GetV1ReportsInventoryResponse) {
				if response.JSON200 == nil {
					t.Fatal("missing JSON200")
				}
				result, err := response.JSON200.AsModelsInbandInventoryReportResponse()
				if err != nil {
					t.Fatalf("decode inband: %v", err)
				}
				if result.Nodes == nil || len(*result.Nodes) != 1 {
					t.Fatalf("inband nodes: %+v", result.Nodes)
				}
				node := (*result.Nodes)[0]
				if node.NodeUUID == nil || *node.NodeUUID != "inband-1" || node.Hostname == nil || *node.Hostname != "host-1" || node.GpuCount == nil || *node.GpuCount != 8 {
					t.Errorf("inband node: %+v", node)
				}
				checkComputeZonePage(t, result.Page, result.PageSize, result.Total, result.HasMore, 2, 25, 51, true)
			},
		},
		{
			name: "oob json", contentType: "application/json", status: http.StatusOK,
			params: &GetV1ReportsInventoryParams{DataSource: &oob, Format: &jsonFormat, SortBy: &macSort, Order: &desc},
			query:  url.Values{"dataSource": {"oob"}, "format": {"json"}, "sortBy": {"bmcMac"}, "order": {"desc"}},
			body:   `{"nodes":[{"nodeUUID":"oob-1","nodeName":"server-1","bmcMac":"00:11:22:33:44:55","gpuCount":0}]}`,
			check: func(t *testing.T, response *GetV1ReportsInventoryResponse) {
				if response.JSON200 == nil {
					t.Fatal("missing JSON200")
				}
				result, err := response.JSON200.AsModelsOobInventoryReportResponse()
				if err != nil {
					t.Fatalf("decode oob: %v", err)
				}
				if result.Nodes == nil || len(*result.Nodes) != 1 {
					t.Fatalf("oob nodes: %+v", result.Nodes)
				}
				node := (*result.Nodes)[0]
				if node.NodeUUID == nil || *node.NodeUUID != "oob-1" || node.NodeName == nil || *node.NodeName != "server-1" || node.BmcMac == nil || *node.BmcMac != "00:11:22:33:44:55" || node.GpuCount == nil || *node.GpuCount != 0 {
					t.Errorf("oob node: %+v", node)
				}
			},
		},
		{
			name: "csv", contentType: "text/csv", status: http.StatusOK,
			params: &GetV1ReportsInventoryParams{Format: &csvFormat}, query: url.Values{"format": {"csv"}},
			body: "nodeUUID,hostname,gpuCount\ninband-1,host-1,8\n",
			check: func(t *testing.T, response *GetV1ReportsInventoryResponse) {
				if response.JSON200 != nil {
					t.Errorf("unexpected JSON200: %+v", response.JSON200)
				}
			},
		},
		{
			name: "forbidden", contentType: "application/json", status: http.StatusForbidden,
			params: &GetV1ReportsInventoryParams{Format: &jsonFormat}, query: url.Values{"format": {"json"}},
			body: `{"error":"forbidden","details":"missing permission"}`,
			check: func(t *testing.T, response *GetV1ReportsInventoryResponse) {
				if response.JSON200 != nil || response.JSON403 == nil {
					t.Fatalf("unexpected decoded response: %+v", response)
				}
				if response.JSON403.Error == nil || *response.JSON403.Error != "forbidden" || response.JSON403.Details == nil || *response.JSON403.Details != "missing permission" {
					t.Errorf("error response: %+v", response.JSON403)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/reports/inventory" {
					t.Errorf("request = %s %s", r.Method, r.URL.Path)
				}
				if !reflect.DeepEqual(r.URL.Query(), tt.query) {
					t.Errorf("query = %v, want %v", r.URL.Query(), tt.query)
				}
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(tt.status)
				if _, err := w.Write([]byte(tt.body)); err != nil {
					t.Errorf("write response: %v", err)
				}
			}))
			defer server.Close()
			client, err := NewClientWithResponses(server.URL)
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			response, err := client.GetV1ReportsInventoryWithResponse(context.Background(), tt.params)
			if err != nil {
				t.Fatalf("request: %v", err)
			}
			if response == nil {
				t.Fatal("nil response without error")
			}
			if response.StatusCode() != tt.status || string(response.Body) != tt.body {
				t.Errorf("status/body = %d %q, want %d %q", response.StatusCode(), response.Body, tt.status, tt.body)
			}
			tt.check(t, response)
		})
	}
}

func TestSecurityVulnerabilitiesReportResponseContract(t *testing.T) {
	zones := []string{"zone-1", "zone-2"}
	severities := []ModelsSecurityVulnerabilitySeverity{SecurityVulnerabilitySeverityCritical, SecurityVulnerabilitySeverityHigh}
	components := []ModelsVulnerabilityComponent{VulnerabilityComponentGPUDriver, VulnerabilityComponentVBIOS}
	search := "CVE-2026"
	page, pageSize := 0, 25
	sortBy := GetV1ReportsSecurityVulnerabilitiesParamsSortByAffectedNodes
	order := GetV1ReportsSecurityVulnerabilitiesParamsOrderDesc
	jsonFormat, csvFormat := GetV1ReportsSecurityVulnerabilitiesParamsFormatJson, GetV1ReportsSecurityVulnerabilitiesParamsFormatCsv
	countOnly := true
	const disposition = `attachment; filename="vulnerabilities.csv"`

	tests := []struct {
		name, contentType, body string
		params                  *GetV1ReportsSecurityVulnerabilitiesParams
		query                   url.Values
		status                  int
		headers                 http.Header
		wantError               bool
		check                   func(*testing.T, *GetV1ReportsSecurityVulnerabilitiesResponse)
	}{
		{
			name: "filtered json page", contentType: "application/json; charset=utf-8", status: http.StatusOK,
			params: &GetV1ReportsSecurityVulnerabilitiesParams{ComputeZoneIds: &zones, Severities: &severities, ComponentTypes: &components, VulnerabilitySearch: &search, Page: &page, PageSize: &pageSize, SortBy: &sortBy, Order: &order, Format: &jsonFormat},
			query:  url.Values{"computeZoneIds": {"zone-1", "zone-2"}, "severities": {"CRITICAL", "HIGH"}, "componentTypes": {"gpuDriver", "vbios"}, "vulnerabilitySearch": {"CVE-2026"}, "page": {"0"}, "pageSize": {"25"}, "sortBy": {"affectedNodes"}, "order": {"desc"}, "format": {"json"}},
			body:   `{"vulnerabilities":[{"vulnerability":"CVE-2026-1234","vulnerabilityUrl":"https://example.com/CVE-2026-1234","component":"gpuDriver","severity":"CRITICAL","affectedNodes":7,"fixVersions":["550.1","560.2"],"description":"Driver issue"}],"summary":{"affectedNodes":9,"distinctComponentCount":2,"totalNodes":20,"severityCounts":{"critical":3,"high":4,"medium":5,"low":6}},"page":0,"pageSize":25,"total":30,"hasMore":true}`,
			check: func(t *testing.T, response *GetV1ReportsSecurityVulnerabilitiesResponse) {
				if response.JSON200 == nil {
					t.Fatal("missing JSON200")
				}
				result, err := response.JSON200.AsModelsSecurityVulnerabilitiesResponse()
				if err != nil {
					t.Fatalf("decode page: %v", err)
				}
				if len(result.Vulnerabilities) != 1 {
					t.Fatalf("vulnerabilities: %+v", result.Vulnerabilities)
				}
				v := result.Vulnerabilities[0]
				if v.Vulnerability != "CVE-2026-1234" || v.VulnerabilityUrl != "https://example.com/CVE-2026-1234" || v.Component != VulnerabilityComponentGPUDriver || v.Severity != SecurityVulnerabilitySeverityCritical || v.AffectedNodes != 7 || !reflect.DeepEqual(v.FixVersions, []string{"550.1", "560.2"}) || v.Description == nil || *v.Description != "Driver issue" {
					t.Errorf("vulnerability: %+v", v)
				}
				if result.Page != 0 || result.PageSize != 25 || result.Total != 30 || !result.HasMore {
					t.Errorf("pagination: %+v", result)
				}
				wantSummary := ModelsSecurityVulnerabilitySummary{AffectedNodes: 9, DistinctComponentCount: 2, TotalNodes: 20, SeverityCounts: ModelsVulnerabilitySeverityCounts{Critical: 3, High: 4, Medium: 5, Low: 6}}
				if result.Summary != wantSummary {
					t.Errorf("summary = %+v, want %+v", result.Summary, wantSummary)
				}
			},
		},
		{
			name: "count only", contentType: "application/json", status: http.StatusOK,
			params: &GetV1ReportsSecurityVulnerabilitiesParams{CountOnly: &countOnly}, query: url.Values{"countOnly": {"true"}}, body: `{"total":3}`,
			check: func(t *testing.T, response *GetV1ReportsSecurityVulnerabilitiesResponse) {
				if response.JSON200 == nil {
					t.Fatal("missing JSON200")
				}
				result, err := response.JSON200.AsGetV1ReportsSecurityVulnerabilities200JSONResponseBody1()
				if err != nil || result.Total != 3 {
					t.Errorf("count = %+v, error = %v", result, err)
				}
			},
		},
		{
			name: "csv", contentType: "text/csv", status: http.StatusOK,
			params: &GetV1ReportsSecurityVulnerabilitiesParams{Format: &csvFormat}, query: url.Values{"format": {"csv"}},
			body: "vulnerability,component\nCVE-2026-1234,gpuDriver\n", headers: http.Header{"Content-Disposition": {disposition}, "X-Total-Count": {"3"}},
			check: func(t *testing.T, response *GetV1ReportsSecurityVulnerabilitiesResponse) {
				if response.JSON200 != nil || response.Headers200 == nil || response.Headers200.ContentDisposition == nil || *response.Headers200.ContentDisposition != disposition || response.Headers200.XTotalCount == nil || *response.Headers200.XTotalCount != 3 {
					t.Errorf("csv response: %+v, headers: %+v", response.JSON200, response.Headers200)
				}
			},
		},
		{
			name: "forbidden", contentType: "application/json", status: http.StatusForbidden,
			params: &GetV1ReportsSecurityVulnerabilitiesParams{Format: &jsonFormat}, query: url.Values{"format": {"json"}}, body: `{"error":"forbidden","details":"missing permission"}`,
			check: func(t *testing.T, response *GetV1ReportsSecurityVulnerabilitiesResponse) {
				if response.JSON200 != nil || response.JSON403 == nil {
					t.Fatalf("unexpected decoded response: %+v", response)
				}
				if response.JSON403.Error == nil || *response.JSON403.Error != "forbidden" || response.JSON403.Details == nil || *response.JSON403.Details != "missing permission" {
					t.Errorf("error response: %+v", response.JSON403)
				}
			},
		},
		{
			name: "malformed json", contentType: "application/json", status: http.StatusOK,
			params: &GetV1ReportsSecurityVulnerabilitiesParams{Format: &jsonFormat}, query: url.Values{"format": {"json"}}, body: `{"total":`, wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/reports/security/vulnerabilities" {
					t.Errorf("request = %s %s", r.Method, r.URL.Path)
				}
				if !reflect.DeepEqual(r.URL.Query(), tt.query) {
					t.Errorf("query = %v, want %v", r.URL.Query(), tt.query)
				}
				w.Header().Set("Content-Type", tt.contentType)
				for key, values := range tt.headers {
					for _, value := range values {
						w.Header().Add(key, value)
					}
				}
				w.WriteHeader(tt.status)
				if _, err := w.Write([]byte(tt.body)); err != nil {
					t.Errorf("write response: %v", err)
				}
			}))
			defer server.Close()
			client, err := NewClientWithResponses(server.URL)
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			response, err := client.GetV1ReportsSecurityVulnerabilitiesWithResponse(context.Background(), tt.params)
			if tt.wantError {
				if err == nil || response != nil {
					t.Fatalf("expected decoding error and nil response, got response=%+v err=%v", response, err)
				}
				return
			}
			if err != nil || response == nil {
				t.Fatalf("request: response=%+v err=%v", response, err)
			}
			if response.StatusCode() != tt.status || string(response.Body) != tt.body {
				t.Errorf("status/body = %d %q, want %d %q", response.StatusCode(), response.Body, tt.status, tt.body)
			}
			tt.check(t, response)
		})
	}
}

func TestErrorReportResponseContract(t *testing.T) {
	groupError, groupNode := GetV1ReportsErrorParamsGroupByError, GetV1ReportsErrorParamsGroupByNode
	jsonFormat, csvFormat := GetV1ReportsErrorParamsFormatJson, GetV1ReportsErrorParamsFormatCsv
	oob := GetV1ReportsErrorParamsDataSourceOob
	page, pageSize := 2, 25
	zones, groups := []string{"zone-1", "zone-2"}, []string{"group-1", "group-2"}
	tags := []string{"rack-a", "rack-b"}
	severities := []ModelsErrorSeverity{ErrorSeverityFatal, ErrorSeverityWarning}
	errors := []string{"xid_154", "NVSwitch Fatal Error"}
	timeMode, window, step := "relative", "24h", "5m"

	tests := []struct {
		name              string
		params            *GetV1ReportsErrorParams
		query             url.Values
		status            int
		contentType, body string
		check             func(*testing.T, *GetV1ReportsErrorResponse, error)
	}{
		{
			name: "list by error", params: &GetV1ReportsErrorParams{View: List, GroupBy: &groupError, Format: &jsonFormat, Page: &page, PageSize: &pageSize, ComputeZoneIds: &zones, NodeGroupIds: &groups, Tags: &tags, Severities: &severities, TimeMode: &timeMode, Window: &window},
			query:  url.Values{"view": {"list"}, "groupBy": {"error"}, "format": {"json"}, "page": {"2"}, "pageSize": {"25"}, "computeZoneIds": {"zone-1", "zone-2"}, "nodeGroupIds": {"group-1", "group-2"}, "tags": {"rack-a", "rack-b"}, "severities": {"Fatal", "Warning"}, "timeMode": {"relative"}, "window": {"24h"}},
			status: http.StatusOK, contentType: "application/json", body: `{"errors":[{"name":"xid_154","count":7,"nodeCount":3,"severities":["Fatal","Warning"],"suggestedAction":{"action":"Inspect GPU","code":"A1"}}],"page":2,"pageSize":25,"total":51,"hasMore":true}`,
			check: func(t *testing.T, response *GetV1ReportsErrorResponse, err error) {
				if err != nil || response == nil || response.JSON200 == nil {
					t.Fatalf("request: response=%+v err=%v", response, err)
				}
				result, err := response.JSON200.AsModelsByErrorResponse()
				if err != nil {
					t.Fatalf("decode by error: %v", err)
				}
				if result.Errors == nil || len(*result.Errors) != 1 {
					t.Fatalf("errors: %+v", result.Errors)
				}
				row := (*result.Errors)[0]
				if row.Name == nil || *row.Name != "xid_154" || row.Count == nil || *row.Count != 7 || row.NodeCount == nil || *row.NodeCount != 3 || row.Severities == nil || !reflect.DeepEqual(*row.Severities, severities) || row.SuggestedAction == nil || row.SuggestedAction.Action == nil || *row.SuggestedAction.Action != "Inspect GPU" || row.SuggestedAction.Code == nil || *row.SuggestedAction.Code != "A1" {
					t.Errorf("error row: %+v", row)
				}
				checkComputeZonePage(t, result.Page, result.PageSize, result.Total, result.HasMore, 2, 25, 51, true)
			},
		},
		{
			name: "list by node", params: &GetV1ReportsErrorParams{View: List, GroupBy: &groupNode, Format: &jsonFormat, DataSource: &oob, Errors: &errors},
			query: url.Values{"view": {"list"}, "groupBy": {"node"}, "format": {"json"}, "dataSource": {"oob"}, "errors": {"xid_154", "NVSwitch Fatal Error"}}, status: http.StatusOK, contentType: "application/json", body: `{"nodes":[{"nodeUUID":"node-1","nodeName":"alpha","bmcMac":"00:11:22:33:44:55","errors":["xid_154","NVSwitch Fatal Error"]}]}`,
			check: func(t *testing.T, response *GetV1ReportsErrorResponse, err error) {
				if err != nil || response == nil || response.JSON200 == nil {
					t.Fatalf("request: response=%+v err=%v", response, err)
				}
				result, err := response.JSON200.AsModelsByNodeResponse()
				if err != nil {
					t.Fatalf("decode by node: %v", err)
				}
				if result.Nodes == nil || len(*result.Nodes) != 1 {
					t.Fatalf("nodes: %+v", result.Nodes)
				}
				row := (*result.Nodes)[0]
				if row.NodeUUID == nil || *row.NodeUUID != "node-1" || row.NodeName == nil || *row.NodeName != "alpha" || row.BmcMac == nil || *row.BmcMac != "00:11:22:33:44:55" || row.Errors == nil || !reflect.DeepEqual(*row.Errors, errors) {
					t.Errorf("node row: %+v", row)
				}
			},
		},
		{
			name: "graph by error", params: &GetV1ReportsErrorParams{View: Graph, GroupBy: &groupError, Step: &step}, query: url.Values{"view": {"graph"}, "groupBy": {"error"}, "step": {"5m"}}, status: http.StatusOK, contentType: "application/json", body: `{"result":[{"label":{"error":"xid_154"},"values":"[[1716153600,5]]"}],"timeRange":{"start":"2026-05-01T00:00:00Z","end":"2026-05-02T00:00:00Z"}}`,
			check: func(t *testing.T, response *GetV1ReportsErrorResponse, err error) {
				if err != nil || response == nil || response.JSON200 == nil {
					t.Fatalf("request: response=%+v err=%v", response, err)
				}
				result, err := response.JSON200.AsModelsErrorReportGraphByErrorResponse()
				if err != nil {
					t.Fatalf("decode graph: %v", err)
				}
				if result.Result == nil || len(*result.Result) != 1 || (*result.Result)[0].Label == nil || (*result.Result)[0].Label.Error == nil || *(*result.Result)[0].Label.Error != "xid_154" {
					t.Fatalf("graph result: %+v", result.Result)
				}
				if (*result.Result)[0].Values == nil || *(*result.Result)[0].Values != "[[1716153600,5]]" {
					t.Errorf("graph values: %+v", (*result.Result)[0].Values)
				}
				if result.TimeRange == nil || result.TimeRange.Start == nil || result.TimeRange.End == nil || result.TimeRange.Start.Format("2006-01-02T15:04:05Z07:00") != "2026-05-01T00:00:00Z" || result.TimeRange.End.Format("2006-01-02T15:04:05Z07:00") != "2026-05-02T00:00:00Z" {
					t.Errorf("time range: %+v", result.TimeRange)
				}
			},
		},
		{
			name: "graph array values", params: &GetV1ReportsErrorParams{View: Graph, GroupBy: &groupError, Step: &step}, query: url.Values{"view": {"graph"}, "groupBy": {"error"}, "step": {"5m"}}, status: http.StatusOK, contentType: "application/json", body: `{"result":[{"label":{"error":"xid_154"},"values":[[1716153600,5]]}],"timeRange":{"start":"2026-05-01T00:00:00Z","end":"2026-05-02T00:00:00Z"}}`,
			check: func(t *testing.T, response *GetV1ReportsErrorResponse, err error) {
				if err != nil || response == nil || response.JSON200 == nil {
					t.Fatalf("request: response=%+v err=%v", response, err)
				}
				if _, err := response.JSON200.AsModelsErrorReportGraphByErrorResponse(); err == nil {
					t.Error("expected array values to fail decoding into the generated string field")
				}
			},
		},
		{
			name: "overview", params: &GetV1ReportsErrorParams{View: Overview}, query: url.Values{"view": {"overview"}}, status: http.StatusOK, contentType: "application/json", body: `{"totalErrors":12,"totalErrorNodes":4,"totalErrorTypes":3}`,
			check: func(t *testing.T, response *GetV1ReportsErrorResponse, err error) {
				if err != nil || response == nil || response.JSON200 == nil {
					t.Fatalf("request: response=%+v err=%v", response, err)
				}
				result, err := response.JSON200.AsModelsErrorOverviewResponse()
				if err != nil {
					t.Fatalf("decode overview: %v", err)
				}
				if result.TotalErrors == nil || *result.TotalErrors != 12 || result.TotalErrorNodes == nil || *result.TotalErrorNodes != 4 || result.TotalErrorTypes == nil || *result.TotalErrorTypes != 3 {
					t.Errorf("overview: %+v", result)
				}
			},
		},
		{
			name: "csv", params: &GetV1ReportsErrorParams{View: List, GroupBy: &groupError, Format: &csvFormat}, query: url.Values{"view": {"list"}, "groupBy": {"error"}, "format": {"csv"}}, status: http.StatusOK, contentType: "text/csv", body: "name,count\nxid_154,7\n",
			check: func(t *testing.T, response *GetV1ReportsErrorResponse, err error) {
				if err != nil || response == nil || response.JSON200 != nil {
					t.Errorf("csv response=%+v err=%v", response, err)
				}
			},
		},
		{
			name: "forbidden", params: &GetV1ReportsErrorParams{View: Overview}, query: url.Values{"view": {"overview"}}, status: http.StatusForbidden, contentType: "application/json", body: `{"error":"forbidden","details":"missing permission"}`,
			check: func(t *testing.T, response *GetV1ReportsErrorResponse, err error) {
				if err != nil || response == nil || response.JSON200 != nil || response.JSON403 == nil {
					t.Fatalf("forbidden response=%+v err=%v", response, err)
				}
				if response.JSON403.Error == nil || *response.JSON403.Error != "forbidden" || response.JSON403.Details == nil || *response.JSON403.Details != "missing permission" {
					t.Errorf("error response: %+v", response.JSON403)
				}
			},
		},
		{
			name: "malformed json", params: &GetV1ReportsErrorParams{View: Overview}, query: url.Values{"view": {"overview"}}, status: http.StatusOK, contentType: "application/json", body: `{"totalErrors":`,
			check: func(t *testing.T, response *GetV1ReportsErrorResponse, err error) {
				if err == nil {
					t.Errorf("expected decoding error, got %+v", response)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/reports/error" {
					t.Errorf("request = %s %s", r.Method, r.URL.Path)
				}
				if !reflect.DeepEqual(r.URL.Query(), tt.query) {
					t.Errorf("query = %v, want %v", r.URL.Query(), tt.query)
				}
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(tt.status)
				if _, err := w.Write([]byte(tt.body)); err != nil {
					t.Errorf("write response: %v", err)
				}
			}))
			defer server.Close()
			client, err := NewClientWithResponses(server.URL)
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			response, err := client.GetV1ReportsErrorWithResponse(context.Background(), tt.params)
			if err == nil {
				if response == nil {
					t.Fatal("nil response without error")
				}
				if response.StatusCode() != tt.status || string(response.Body) != tt.body {
					t.Errorf("status/body = %d %q, want %d %q", response.StatusCode(), response.Body, tt.status, tt.body)
				}
			}
			tt.check(t, response, err)
		})
	}
}

func TestAlertTimelineNodesResponseContract(t *testing.T) {
	active := true
	page, pageSize := 0, 25
	search := "rack & fan"
	inband := []ModelsAlertAgentStatus{"offline", "not_enrolled"}
	oob := []ModelsAlertAgentStatus{"online"}
	sources := []GetV1AlertTimelineNodesParamsDataSources{GetV1AlertTimelineNodesParamsDataSourcesInband, GetV1AlertTimelineNodesParamsDataSourcesOob}
	states := []GetV1AlertTimelineNodesParamsAlertStates{GetV1AlertTimelineNodesParamsAlertStatesCritical, GetV1AlertTimelineNodesParamsAlertStatesWarning}
	sortBy, order := GetV1AlertTimelineNodesParamsSortByAlert, GetV1AlertTimelineNodesParamsOrderDesc
	gpuTypes := []string{"H100", "A100"}
	groups := []string{"group-1", "group-2"}
	zones := []string{"zone-1", "zone-2"}
	components := []string{"gpu", "fan"}
	params := &GetV1AlertTimelineNodesParams{Active: &active, Page: &page, PageSize: &pageSize, NameSearch: &search, InbandAgentStatuses: &inband, OobAgentStatuses: &oob, DataSources: &sources, AlertStates: &states, SortBy: &sortBy, Order: &order, GpuTypes: &gpuTypes, NodeGroupIds: &groups, ComputeZoneIds: &zones, ComponentTypes: &components}
	query := url.Values{"active": {"true"}, "page": {"0"}, "pageSize": {"25"}, "nameSearch": {"rack & fan"}, "inbandAgentStatuses": {"offline", "not_enrolled"}, "oobAgentStatuses": {"online"}, "dataSources": {"inband", "oob"}, "alertStates": {"Critical", "Warning"}, "sortBy": {"alert"}, "order": {"desc"}, "gpuTypes": {"H100", "A100"}, "nodeGroupIds": {"group-1", "group-2"}, "computeZoneIds": {"zone-1", "zone-2"}, "componentTypes": {"gpu", "fan"}}

	tests := []struct {
		name, contentType, body string
		status                  int
		params                  *GetV1AlertTimelineNodesParams
		query                   url.Values
		check                   func(*testing.T, *GetV1AlertTimelineNodesResponse, error)
	}{
		{
			name: "success", contentType: "application/json", status: http.StatusOK, params: params, query: query,
			body: `{"nodes":[{"nodeUuid":"node-1","hostname":"host-1","criticalCount":3,"warningCount":2}],"page":0,"pageSize":25,"total":42,"hasMore":true,"totalCritical":7,"totalWarning":5}`,
			check: func(t *testing.T, response *GetV1AlertTimelineNodesResponse, err error) {
				if err != nil {
					t.Fatalf("request: %v", err)
				}
				if response.JSON200 == nil || response.JSON200.Nodes == nil || len(*response.JSON200.Nodes) != 1 {
					t.Fatalf("missing node: %+v", response.JSON200)
				}
				node := (*response.JSON200.Nodes)[0]
				if node.NodeUuid != "node-1" || node.Hostname == nil || *node.Hostname != "host-1" || node.CriticalCount == nil || *node.CriticalCount != 3 || node.WarningCount == nil || *node.WarningCount != 2 {
					t.Errorf("node = %+v", node)
				}
				result := response.JSON200
				if result.Page == nil || *result.Page != 0 || result.PageSize == nil || *result.PageSize != 25 || result.Total == nil || *result.Total != 42 || result.HasMore == nil || !*result.HasMore || result.TotalCritical == nil || *result.TotalCritical != 7 || result.TotalWarning == nil || *result.TotalWarning != 5 {
					t.Errorf("summary = %+v", result)
				}
			},
		},
		{
			name: "json unauthorized", contentType: "application/json", status: http.StatusUnauthorized, body: `{"error":"unauthorized","details":"access denied"}`,
			check: func(t *testing.T, response *GetV1AlertTimelineNodesResponse, err error) {
				if err != nil {
					t.Fatalf("request: %v", err)
				}
				if response.JSON200 != nil || response.JSON401 == nil {
					t.Fatalf("unexpected decoded response: %+v", response)
				}
				if response.JSON401.Error == nil || *response.JSON401.Error != "unauthorized" || response.JSON401.Details == nil || *response.JSON401.Details != "access denied" {
					t.Errorf("error response: %+v", response.JSON401)
				}
			},
		},
		{
			name: "malformed json", contentType: "application/json", status: http.StatusOK, body: `{"nodes":`,
			check: func(t *testing.T, response *GetV1AlertTimelineNodesResponse, err error) {
				if err == nil || response != nil {
					t.Errorf("expected decoding error and nil response, got response %+v, error %v", response, err)
				}
			},
		},
		{
			name: "non-json unauthorized", contentType: "text/plain", status: http.StatusUnauthorized, body: "access denied",
			check: func(t *testing.T, response *GetV1AlertTimelineNodesResponse, err error) {
				if err != nil {
					t.Fatalf("request: %v", err)
				}
				if response.JSON200 != nil || response.JSON400 != nil || response.JSON401 != nil || response.JSON499 != nil || response.JSON500 != nil {
					t.Errorf("unexpected decoded response: %+v", response)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/alert_timeline/nodes" {
					t.Errorf("request = %s %s", r.Method, r.URL.Path)
				}
				if !reflect.DeepEqual(r.URL.Query(), tt.query) && !(len(r.URL.Query()) == 0 && len(tt.query) == 0) {
					t.Errorf("query = %v, want %v", r.URL.Query(), tt.query)
				}
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(tt.status)
				if _, err := w.Write([]byte(tt.body)); err != nil {
					t.Errorf("write response: %v", err)
				}
			}))
			defer server.Close()
			client, err := NewClientWithResponses(server.URL)
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			response, err := client.GetV1AlertTimelineNodesWithResponse(context.Background(), tt.params)
			if err == nil {
				if response == nil {
					t.Fatal("nil response without error")
				}
				if response.StatusCode() != tt.status || string(response.Body) != tt.body {
					t.Errorf("status/body = %d %q, want %d %q", response.StatusCode(), response.Body, tt.status, tt.body)
				}
			}
			tt.check(t, response, err)
		})
	}
}

func TestAlertTimelineNodeAlertsResponseContract(t *testing.T) {
	active := true
	sortBy := GetV1AlertTimelineNodesNodeUuidAlertsParamsSortByDataSource
	order := GetV1AlertTimelineNodesNodeUuidAlertsParamsOrderAsc
	sources := []GetV1AlertTimelineNodesNodeUuidAlertsParamsDataSources{GetV1AlertTimelineNodesNodeUuidAlertsParamsDataSourcesInband, GetV1AlertTimelineNodesNodeUuidAlertsParamsDataSourcesOob}
	states := []GetV1AlertTimelineNodesNodeUuidAlertsParamsAlertStates{GetV1AlertTimelineNodesNodeUuidAlertsParamsAlertStatesCritical, GetV1AlertTimelineNodesNodeUuidAlertsParamsAlertStatesWarning}
	components := []string{"gpu", "fan"}
	gpuTypes := []string{"H100", "A100"}
	groups := []string{"group-1", "group-2"}
	zones := []string{"zone-1", "zone-2"}
	page, pageSize := 1, 10
	params := &GetV1AlertTimelineNodesNodeUuidAlertsParams{Active: &active, SortBy: &sortBy, Order: &order, DataSources: &sources, AlertStates: &states, ComponentTypes: &components, GpuTypes: &gpuTypes, NodeGroupIds: &groups, ComputeZoneIds: &zones, Page: &page, PageSize: &pageSize}
	query := url.Values{"active": {"true"}, "sortBy": {"dataSource"}, "order": {"asc"}, "dataSources": {"inband", "oob"}, "alertStates": {"Critical", "Warning"}, "componentTypes": {"gpu", "fan"}, "gpuTypes": {"H100", "A100"}, "nodeGroupIds": {"group-1", "group-2"}, "computeZoneIds": {"zone-1", "zone-2"}, "page": {"1"}, "pageSize": {"10"}}

	tests := []struct {
		name, contentType, body string
		status                  int
		params                  *GetV1AlertTimelineNodesNodeUuidAlertsParams
		query                   url.Values
		check                   func(*testing.T, *GetV1AlertTimelineNodesNodeUuidAlertsResponse, error)
	}{
		{
			name: "success", contentType: "application/json", status: http.StatusOK, params: params, query: query,
			body: `{"nodeUuid":"node-1","hostname":"host-1","alerts":[{"alertUuid":"alert-1","component":"gpu","dataSource":"inband","alertStatus":"Critical"},{"alertUuid":"alert-2","component":"fan","dataSource":"oob","alertStatus":"Warning"}],"page":1,"pageSize":10,"total":12,"hasMore":true}`,
			check: func(t *testing.T, response *GetV1AlertTimelineNodesNodeUuidAlertsResponse, err error) {
				if err != nil {
					t.Fatalf("request: %v", err)
				}
				result := response.JSON200
				if result == nil || result.Alerts == nil || len(*result.Alerts) != 2 {
					t.Fatalf("missing alerts: %+v", result)
				}
				if result.NodeUuid != "node-1" || result.Hostname == nil || *result.Hostname != "host-1" {
					t.Errorf("node = %+v", result)
				}
				for i, want := range []struct{ uuid, component, source, status string }{
					{"alert-1", "gpu", "inband", "Critical"},
					{"alert-2", "fan", "oob", "Warning"},
				} {
					alert := (*result.Alerts)[i]
					if alert.AlertUuid != want.uuid || alert.Component == nil || *alert.Component != want.component || alert.DataSource == nil || *alert.DataSource != want.source || alert.AlertStatus == nil || *alert.AlertStatus != want.status {
						t.Errorf("alert %d = %+v, want %+v", i, alert, want)
					}
				}
				checkComputeZonePage(t, result.Page, result.PageSize, result.Total, result.HasMore, 1, 10, 12, true)
			},
		},
		{
			name: "json not found", contentType: "application/json", status: http.StatusNotFound, body: `{"error":"not found","details":"node missing"}`,
			check: func(t *testing.T, response *GetV1AlertTimelineNodesNodeUuidAlertsResponse, err error) {
				if err != nil {
					t.Fatalf("request: %v", err)
				}
				if response.JSON200 != nil || response.JSON404 == nil {
					t.Fatalf("unexpected decoded response: %+v", response)
				}
				if response.JSON404.Error == nil || *response.JSON404.Error != "not found" || response.JSON404.Details == nil || *response.JSON404.Details != "node missing" {
					t.Errorf("error response: %+v", response.JSON404)
				}
			},
		},
		{
			name: "malformed json", contentType: "application/json", status: http.StatusOK, body: `{"alerts":`,
			check: func(t *testing.T, response *GetV1AlertTimelineNodesNodeUuidAlertsResponse, err error) {
				if err == nil || response != nil {
					t.Errorf("expected decoding error and nil response, got response %+v, error %v", response, err)
				}
			},
		},
		{
			name: "non-json unauthorized", contentType: "text/plain", status: http.StatusUnauthorized, body: "access denied",
			check: func(t *testing.T, response *GetV1AlertTimelineNodesNodeUuidAlertsResponse, err error) {
				if err != nil {
					t.Fatalf("request: %v", err)
				}
				if response.JSON200 != nil || response.JSON400 != nil || response.JSON401 != nil || response.JSON404 != nil || response.JSON499 != nil || response.JSON500 != nil {
					t.Errorf("unexpected decoded response: %+v", response)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/alert_timeline/nodes/node-1/alerts" {
					t.Errorf("request = %s %s", r.Method, r.URL.Path)
				}
				if !reflect.DeepEqual(r.URL.Query(), tt.query) && !(len(r.URL.Query()) == 0 && len(tt.query) == 0) {
					t.Errorf("query = %v, want %v", r.URL.Query(), tt.query)
				}
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(tt.status)
				if _, err := w.Write([]byte(tt.body)); err != nil {
					t.Errorf("write response: %v", err)
				}
			}))
			defer server.Close()
			client, err := NewClientWithResponses(server.URL)
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			response, err := client.GetV1AlertTimelineNodesNodeUuidAlertsWithResponse(context.Background(), "node-1", tt.params)
			if err == nil {
				if response == nil {
					t.Fatal("nil response without error")
				}
				if response.StatusCode() != tt.status || string(response.Body) != tt.body {
					t.Errorf("status/body = %d %q, want %d %q", response.StatusCode(), response.Body, tt.status, tt.body)
				}
			}
			tt.check(t, response, err)
		})
	}
}

func TestAlertTimelineDetailResponseContract(t *testing.T) {
	order := GetV1AlertTimelineNodesNodeUuidAlertsAlertUuidParamsOrderAsc
	page, pageSize := 1, 10
	checkTimeline := func(t *testing.T, result *ModelsAlertTimelineDetailResponse) {
		t.Helper()
		if result == nil || result.Timeline == nil || len(*result.Timeline) != 1 {
			t.Fatalf("missing timeline: %+v", result)
		}
		if result.AlertUuid != "alert-1" || result.NodeUuid != "node-1" || result.Component == nil || *result.Component != "gpu" {
			t.Errorf("detail identifiers: %+v", result)
		}
		event := (*result.Timeline)[0]
		if event.EventType == nil || string(*event.EventType) != "triggered" || event.Message == nil || *event.Message != "GPU critical" || event.ExtraInfo == nil || !reflect.DeepEqual(*event.ExtraInfo, map[string]interface{}{"gpu": "0"}) {
			t.Errorf("timeline event: %+v", event)
		}
	}

	tests := []struct {
		name, body string
		status     int
		params     *GetV1AlertTimelineNodesNodeUuidAlertsAlertUuidParams
		query      url.Values
		check      func(*testing.T, *GetV1AlertTimelineNodesNodeUuidAlertsAlertUuidResponse, error)
	}{
		{
			name: "paginated success", status: http.StatusOK,
			params: &GetV1AlertTimelineNodesNodeUuidAlertsAlertUuidParams{Order: &order, Page: &page, PageSize: &pageSize},
			query:  url.Values{"order": {"asc"}, "page": {"1"}, "pageSize": {"10"}},
			body:   `{"alertUuid":"alert-1","nodeUuid":"node-1","component":"gpu","page":1,"pageSize":10,"total":12,"hasMore":true,"timeline":[{"eventType":"triggered","message":"GPU critical","extraInfo":{"gpu":"0"}}]}`,
			check: func(t *testing.T, response *GetV1AlertTimelineNodesNodeUuidAlertsAlertUuidResponse, err error) {
				if err != nil {
					t.Fatalf("request: %v", err)
				}
				checkTimeline(t, response.JSON200)
				result := response.JSON200
				checkComputeZonePage(t, result.Page, result.PageSize, result.Total, result.HasMore, 1, 10, 12, true)
			},
		},
		{
			name: "unpaginated success", status: http.StatusOK,
			body: `{"alertUuid":"alert-1","nodeUuid":"node-1","component":"gpu","timeline":[{"eventType":"triggered","message":"GPU critical","extraInfo":{"gpu":"0"}}]}`,
			check: func(t *testing.T, response *GetV1AlertTimelineNodesNodeUuidAlertsAlertUuidResponse, err error) {
				if err != nil {
					t.Fatalf("request: %v", err)
				}
				checkTimeline(t, response.JSON200)
				result := response.JSON200
				if result.Page != nil || result.PageSize != nil || result.Total != nil || result.HasMore != nil {
					t.Errorf("unexpected pagination: %+v", result)
				}
			},
		},
		{
			name: "json not found", status: http.StatusNotFound, body: `{"error":"not found","details":"alert missing"}`,
			check: func(t *testing.T, response *GetV1AlertTimelineNodesNodeUuidAlertsAlertUuidResponse, err error) {
				if err != nil {
					t.Fatalf("request: %v", err)
				}
				if response.JSON200 != nil || response.JSON404 == nil {
					t.Fatalf("unexpected decoded response: %+v", response)
				}
				if response.JSON404.Error == nil || *response.JSON404.Error != "not found" || response.JSON404.Details == nil || *response.JSON404.Details != "alert missing" {
					t.Errorf("error response: %+v", response.JSON404)
				}
			},
		},
		{
			name: "malformed json", status: http.StatusOK, body: `{"timeline":`,
			check: func(t *testing.T, response *GetV1AlertTimelineNodesNodeUuidAlertsAlertUuidResponse, err error) {
				if err == nil || response != nil {
					t.Errorf("expected decoding error and nil response, got response %+v, error %v", response, err)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/alert_timeline/nodes/node-1/alerts/alert-1" {
					t.Errorf("request = %s %s", r.Method, r.URL.Path)
				}
				if !reflect.DeepEqual(r.URL.Query(), tt.query) && !(len(r.URL.Query()) == 0 && len(tt.query) == 0) {
					t.Errorf("query = %v, want %v", r.URL.Query(), tt.query)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				if _, err := w.Write([]byte(tt.body)); err != nil {
					t.Errorf("write response: %v", err)
				}
			}))
			defer server.Close()
			client, err := NewClientWithResponses(server.URL)
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			response, err := client.GetV1AlertTimelineNodesNodeUuidAlertsAlertUuidWithResponse(context.Background(), "node-1", "alert-1", tt.params)
			if err == nil {
				if response == nil {
					t.Fatal("nil response without error")
				}
				if response.StatusCode() != tt.status || string(response.Body) != tt.body {
					t.Errorf("status/body = %d %q, want %d %q", response.StatusCode(), response.Body, tt.status, tt.body)
				}
			}
			tt.check(t, response, err)
		})
	}
}

func TestTagsResponseContract(t *testing.T) {
	prefix, node, group, zone := "rack &", "node-1", "group-1", "zone-1"
	tests := []struct {
		name   string
		params *GetV1TagsParams
		query  url.Values
		status int
		body   string
	}{
		{"node filter", &GetV1TagsParams{Prefix: &prefix, NodeUUID: &node}, url.Values{"prefix": {prefix}, "nodeUUID": {node}}, http.StatusOK, `{"tags":["rack & blue","rack & red"]}`},
		{"group filter", &GetV1TagsParams{Prefix: &prefix, NodeGroupId: &group}, url.Values{"prefix": {prefix}, "nodeGroupId": {group}}, http.StatusOK, `{"tags":["rack & blue","rack & red"]}`},
		{"zone filter", &GetV1TagsParams{Prefix: &prefix, ComputeZoneId: &zone}, url.Values{"prefix": {prefix}, "computeZoneId": {zone}}, http.StatusOK, `{"tags":["rack & blue","rack & red"]}`},
		{"forbidden", nil, url.Values{}, http.StatusForbidden, `{"error":"forbidden","details":"missing permission"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/tags" {
					t.Errorf("request = %s %s", r.Method, r.URL.Path)
				}
				if !reflect.DeepEqual(r.URL.Query(), tt.query) {
					t.Errorf("query = %v, want %v", r.URL.Query(), tt.query)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				if _, err := w.Write([]byte(tt.body)); err != nil {
					t.Errorf("write response: %v", err)
				}
			}))
			defer server.Close()
			client, err := NewClientWithResponses(server.URL)
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			response, err := client.GetV1TagsWithResponse(context.Background(), tt.params)
			if err != nil {
				t.Fatalf("request: %v", err)
			}
			if response == nil || response.StatusCode() != tt.status || string(response.Body) != tt.body {
				t.Fatalf("response = %+v", response)
			}
			if tt.status == http.StatusForbidden {
				if response.JSON200 != nil || response.JSON403 == nil || response.JSON403.Error == nil || *response.JSON403.Error != "forbidden" || response.JSON403.Details == nil || *response.JSON403.Details != "missing permission" {
					t.Errorf("error response = %+v", response)
				}
			} else if response.JSON200 == nil || response.JSON200.Tags == nil || !reflect.DeepEqual(*response.JSON200.Tags, []string{"rack & blue", "rack & red"}) {
				t.Errorf("tags response = %+v", response.JSON200)
			}
		})
	}
}

func TestSetNodeTagsResponseContract(t *testing.T) {
	tests := []struct {
		name              string
		tags              []string
		status            int
		contentType, body string
	}{
		{"populated", []string{"rack & blue", "production"}, http.StatusOK, "application/json", `{"nodeUUID":"node-1","tags":["rack & blue","production"]}`},
		{"empty", []string{}, http.StatusOK, "application/json", `{"nodeUUID":"node-1","tags":[]}`},
		{"not found", []string{"production"}, http.StatusNotFound, "application/json", `{"error":"not found","details":"node missing"}`},
		{"malformed json", []string{}, http.StatusOK, "application/json; charset=utf-8", `{"tags":`},
		{"non-json success", []string{}, http.StatusOK, "text/plain", "accepted as text"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPut || r.URL.Path != "/v1/nodes/node-1/tags" || len(r.URL.Query()) != 0 {
					t.Errorf("request = %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
				}
				if r.Header.Get("Content-Type") != "application/json" {
					t.Errorf("content type = %q", r.Header.Get("Content-Type"))
				}
				var request map[string]interface{}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Errorf("decode request: %v", err)
				} else {
					wantTags := make([]interface{}, len(tt.tags))
					for i, tag := range tt.tags {
						wantTags[i] = tag
					}
					want := map[string]interface{}{"tags": wantTags}
					if !reflect.DeepEqual(request, want) {
						t.Errorf("request body = %#v, want %#v", request, want)
					}
				}
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(tt.status)
				if _, err := w.Write([]byte(tt.body)); err != nil {
					t.Errorf("write response: %v", err)
				}
			}))
			defer server.Close()
			client, err := NewClientWithResponses(server.URL)
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			response, err := client.PutV1NodesNodeUuidTagsWithResponse(context.Background(), "node-1", PutV1NodesNodeUuidTagsJSONRequestBody{Tags: tt.tags})
			if tt.name == "malformed json" {
				if err == nil {
					t.Errorf("expected JSON decoding error, got response %+v", response)
				}
				return
			}
			if err != nil {
				t.Fatalf("request: %v", err)
			}
			if response == nil || response.StatusCode() != tt.status || string(response.Body) != tt.body {
				t.Fatalf("response = %+v", response)
			}
			switch tt.name {
			case "not found":
				if response.JSON200 != nil || response.JSON404 == nil || response.JSON404.Error == nil || *response.JSON404.Error != "not found" || response.JSON404.Details == nil || *response.JSON404.Details != "node missing" {
					t.Errorf("error response = %+v", response)
				}
			case "non-json success":
				if response.JSON200 != nil {
					t.Errorf("unexpected decoded response = %+v", response.JSON200)
				}
			default:
				result := response.JSON200
				if result == nil || result.NodeUUID == nil || *result.NodeUUID != "node-1" || result.Tags == nil || *result.Tags == nil || !reflect.DeepEqual(*result.Tags, tt.tags) {
					t.Errorf("tags response = %+v", result)
				}
			}
		})
	}
}

func TestEventBucketsResponseContract(t *testing.T) {
	mode := GetV1EventsBucketsParamsTimeModeRelative
	window, search := "24h", "rack & fan"
	maxBuckets := 50
	eventNames := []string{"overheat", "recovered"}
	sources := []GetV1EventsBucketsParamsDataSources{GetV1EventsBucketsParamsDataSourcesInband, GetV1EventsBucketsParamsDataSourcesOob}
	excluded := []string{"group-1", "group-2"}
	params := &GetV1EventsBucketsParams{TimeMode: &mode, Window: &window, MaxBuckets: &maxBuckets, EventNames: &eventNames, DataSources: &sources, ExcludeNodeGroupIds: &excluded, NameSearch: &search}
	query := url.Values{"timeMode": {"relative"}, "window": {"24h"}, "maxBuckets": {"50"}, "eventNames": {"overheat", "recovered"}, "dataSources": {"inband", "oob"}, "excludeNodeGroupIds": {"group-1", "group-2"}, "nameSearch": {"rack & fan"}}

	tests := []struct {
		name, contentType, body string
		status                  int
		params                  *GetV1EventsBucketsParams
		query                   url.Values
		check                   func(*testing.T, *GetV1EventsBucketsResponse, error)
	}{
		{
			name: "success", contentType: "application/json", status: http.StatusOK, params: params, query: query,
			body: `{"bucketInterval":"1h","buckets":[{"startTime":"2024-01-01T00:00:00Z","endTime":"2024-01-01T01:00:00Z","firstEventTime":"2024-01-01T00:15:00Z","count":3}]}`,
			check: func(t *testing.T, response *GetV1EventsBucketsResponse, err error) {
				if err != nil {
					t.Fatalf("request: %v", err)
				}
				if response.JSON200 == nil || response.JSON200.BucketInterval == nil || *response.JSON200.BucketInterval != "1h" || response.JSON200.Buckets == nil || len(*response.JSON200.Buckets) != 1 {
					t.Fatalf("bucket response: %+v", response.JSON200)
				}
				bucket := (*response.JSON200.Buckets)[0]
				if bucket.StartTime == nil || *bucket.StartTime != "2024-01-01T00:00:00Z" || bucket.EndTime == nil || *bucket.EndTime != "2024-01-01T01:00:00Z" || bucket.FirstEventTime == nil || *bucket.FirstEventTime != "2024-01-01T00:15:00Z" || bucket.Count == nil || *bucket.Count != 3 {
					t.Errorf("bucket = %+v", bucket)
				}
			},
		},
		{
			name: "json forbidden", contentType: "application/json", status: http.StatusForbidden, body: `{"error":"forbidden","details":"access denied"}`,
			check: func(t *testing.T, response *GetV1EventsBucketsResponse, err error) {
				if err != nil {
					t.Fatalf("request: %v", err)
				}
				if response.JSON200 != nil || response.JSON403 == nil {
					t.Fatalf("unexpected decoded response: %+v", response)
				}
				if response.JSON403.Error == nil || *response.JSON403.Error != "forbidden" || response.JSON403.Details == nil || *response.JSON403.Details != "access denied" {
					t.Errorf("error response: %+v", response.JSON403)
				}
			},
		},
		{
			name: "malformed json", contentType: "application/json", status: http.StatusOK, body: `{"buckets":`,
			check: func(t *testing.T, response *GetV1EventsBucketsResponse, err error) {
				if err == nil || response != nil {
					t.Errorf("expected decoding error and nil response, got response %+v, error %v", response, err)
				}
			},
		},
		{
			name: "non-json forbidden", contentType: "text/plain", status: http.StatusForbidden, body: "access denied",
			check: func(t *testing.T, response *GetV1EventsBucketsResponse, err error) {
				if err != nil {
					t.Fatalf("request: %v", err)
				}
				if response.JSON200 != nil || response.JSON400 != nil || response.JSON401 != nil || response.JSON403 != nil || response.JSON499 != nil || response.JSON500 != nil || response.JSON503 != nil {
					t.Errorf("unexpected decoded response: %+v", response)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/events/buckets" {
					t.Errorf("request = %s %s", r.Method, r.URL.Path)
				}
				if !reflect.DeepEqual(r.URL.Query(), tt.query) && !(len(r.URL.Query()) == 0 && len(tt.query) == 0) {
					t.Errorf("query = %v, want %v", r.URL.Query(), tt.query)
				}
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(tt.status)
				if _, err := w.Write([]byte(tt.body)); err != nil {
					t.Errorf("write response: %v", err)
				}
			}))
			defer server.Close()
			client, err := NewClientWithResponses(server.URL)
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			response, err := client.GetV1EventsBucketsWithResponse(context.Background(), tt.params)
			if err == nil {
				if response == nil {
					t.Fatal("nil response without error")
				}
				if response.StatusCode() != tt.status || string(response.Body) != tt.body {
					t.Errorf("status/body = %d %q, want %d %q", response.StatusCode(), response.Body, tt.status, tt.body)
				}
			}
			tt.check(t, response, err)
		})
	}
}

func TestOverviewResponseContract(t *testing.T) {
	include, exclude := true, false
	tests := []struct {
		name, contentType, body string
		params                  *GetV1OverviewParams
		query                   url.Values
		status                  int
		check                   func(*testing.T, *GetV1OverviewResponse, error)
	}{
		{
			name: "metrics included", params: &GetV1OverviewParams{IncludeMetrics: &include},
			query: url.Values{"includeMetrics": {"true"}}, status: http.StatusOK, contentType: "application/json; charset=utf-8",
			body: `{"nodesCount":12,"healthPercentage":75,"metrics":[{"name":"gpu_utilization","unit":"percent","aggregation":"avg","value":42.5}]}`,
			check: func(t *testing.T, response *GetV1OverviewResponse, err error) {
				if err != nil {
					t.Fatalf("request: %v", err)
				}
				if response.JSON200 == nil || response.JSON403 != nil {
					t.Fatalf("unexpected decoded response: %+v", response)
				}
				result := response.JSON200
				if result.NodesCount == nil || *result.NodesCount != 12 || result.HealthPercentage == nil || *result.HealthPercentage != 75 || result.Metrics == nil || len(*result.Metrics) != 1 {
					t.Fatalf("overview: %+v", result)
				}
				metric := (*result.Metrics)[0]
				if metric.Name == nil || *metric.Name != "gpu_utilization" || metric.Unit == nil || string(*metric.Unit) != "percent" || metric.Aggregation == nil || string(*metric.Aggregation) != "avg" || metric.Value == nil || *metric.Value != 42.5 {
					t.Errorf("metric: %+v", metric)
				}
			},
		},
		{
			name: "metrics excluded", params: &GetV1OverviewParams{IncludeMetrics: &exclude},
			query: url.Values{"includeMetrics": {"false"}}, status: http.StatusOK, contentType: "application/json",
			body: `{"nodesCount":12,"computeZoneCount":2,"nodeGroupCount":3,"healthNodeCount":9,"degradedNodeCount":2,"unhealthyNodeCount":1}`,
			check: func(t *testing.T, response *GetV1OverviewResponse, err error) {
				if err != nil {
					t.Fatalf("request: %v", err)
				}
				if response.JSON200 == nil || response.JSON403 != nil {
					t.Fatalf("unexpected decoded response: %+v", response)
				}
				result := response.JSON200
				if result.NodesCount == nil || *result.NodesCount != 12 || result.ComputeZoneCount == nil || *result.ComputeZoneCount != 2 || result.NodeGroupCount == nil || *result.NodeGroupCount != 3 || result.HealthNodeCount == nil || *result.HealthNodeCount != 9 || result.DegradedNodeCount == nil || *result.DegradedNodeCount != 2 || result.UnhealthyNodeCount == nil || *result.UnhealthyNodeCount != 1 || result.Metrics != nil {
					t.Errorf("overview: %+v", result)
				}
			},
		},
		{
			name: "omitted metrics forbidden", params: &GetV1OverviewParams{}, query: url.Values{},
			status: http.StatusForbidden, contentType: "application/json", body: `{"error":"forbidden","details":"missing permission"}`,
			check: func(t *testing.T, response *GetV1OverviewResponse, err error) {
				if err != nil {
					t.Fatalf("request: %v", err)
				}
				if response.JSON200 != nil || response.JSON403 == nil {
					t.Fatalf("unexpected decoded response: %+v", response)
				}
				if response.JSON403.Error == nil || *response.JSON403.Error != "forbidden" || response.JSON403.Details == nil || *response.JSON403.Details != "missing permission" {
					t.Errorf("error response: %+v", response.JSON403)
				}
			},
		},
		{
			name: "malformed json", query: url.Values{}, status: http.StatusOK, contentType: "application/json", body: `{"nodesCount":`,
			check: func(t *testing.T, response *GetV1OverviewResponse, err error) {
				if err == nil || response != nil {
					t.Errorf("expected decoding error and nil response, got response %+v, error %v", response, err)
				}
			},
		},
		{
			name: "non-json forbidden", query: url.Values{}, status: http.StatusForbidden, contentType: "text/plain", body: "access denied",
			check: func(t *testing.T, response *GetV1OverviewResponse, err error) {
				if err != nil {
					t.Fatalf("request: %v", err)
				}
				if response.JSON200 != nil || response.JSON403 != nil {
					t.Errorf("unexpected decoded response: %+v", response)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/overview" {
					t.Errorf("request = %s %s", r.Method, r.URL.Path)
				}
				if !reflect.DeepEqual(r.URL.Query(), tt.query) {
					t.Errorf("query = %v, want %v", r.URL.Query(), tt.query)
				}
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(tt.status)
				if _, err := w.Write([]byte(tt.body)); err != nil {
					t.Errorf("write response: %v", err)
				}
			}))
			defer server.Close()
			client, err := NewClientWithResponses(server.URL)
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			response, err := client.GetV1OverviewWithResponse(context.Background(), tt.params)
			if err == nil {
				if response == nil {
					t.Fatal("nil response without error")
				}
				if response.StatusCode() != tt.status || string(response.Body) != tt.body {
					t.Errorf("status/body = %d %q, want %d %q", response.StatusCode(), response.Body, tt.status, tt.body)
				}
			}
			tt.check(t, response, err)
		})
	}
}

func TestNodeDetailResponseContract(t *testing.T) {
	inband := GetV1NodesNodeUuidParamsDataSourceInband
	oob := GetV1NodesNodeUuidParamsDataSourceOob
	agentType := GetV1NodesNodeUuidParamsAgentTypeOob

	tests := []struct {
		name   string
		params *GetV1NodesNodeUuidParams
		query  url.Values
		status int
		body   string
		check  func(*testing.T, *GetV1NodesNodeUuidResponse)
	}{
		{
			name: "inband", params: &GetV1NodesNodeUuidParams{DataSource: &inband},
			query: url.Values{"dataSource": {"inband"}}, status: http.StatusOK,
			body: `{"nodeUUID":"node-1","dataSource":"inband","hostname":"host-1","agentUUID":"agent-1","agentVersion":"1.2.3","gpuCount":0}`,
			check: func(t *testing.T, response *GetV1NodesNodeUuidResponse) {
				if response.JSON200 == nil {
					t.Fatal("missing JSON200")
				}
				detail, err := response.JSON200.AsModelsInbandNodeDetailsResponse()
				if err != nil {
					t.Fatalf("decode inband detail: %v", err)
				}
				if detail.NodeUUID != "node-1" || detail.DataSource != "inband" || detail.Hostname == nil || *detail.Hostname != "host-1" || detail.AgentUUID == nil || *detail.AgentUUID != "agent-1" || detail.AgentVersion == nil || *detail.AgentVersion != "1.2.3" || detail.GpuCount == nil || *detail.GpuCount != 0 {
					t.Errorf("inband detail: %+v", detail)
				}
			},
		},
		{
			name: "oob", params: &GetV1NodesNodeUuidParams{DataSource: &oob, AgentType: &agentType},
			query: url.Values{"dataSource": {"oob"}, "agentType": {"oob"}}, status: http.StatusOK,
			body: `{"nodeUUID":"node-1","dataSource":"oob","nodeName":"machine-1","bmcMac":"00:11:22:33:44:55","bmcIP":"192.0.2.1","collectorVersion":"2.3.4"}`,
			check: func(t *testing.T, response *GetV1NodesNodeUuidResponse) {
				if response.JSON200 == nil {
					t.Fatal("missing JSON200")
				}
				detail, err := response.JSON200.AsModelsOobNodeDetailsResponse()
				if err != nil {
					t.Fatalf("decode oob detail: %v", err)
				}
				if detail.NodeUUID != "node-1" || detail.DataSource != "oob" || detail.NodeName == nil || *detail.NodeName != "machine-1" || detail.BmcMac == nil || *detail.BmcMac != "00:11:22:33:44:55" || detail.BmcIP == nil || *detail.BmcIP != "192.0.2.1" || detail.CollectorVersion == nil || *detail.CollectorVersion != "2.3.4" {
					t.Errorf("oob detail: %+v", detail)
				}
			},
		},
		{
			name: "json not found", query: url.Values{}, status: http.StatusNotFound,
			body: `{"error":"not found","details":"node does not exist"}`,
			check: func(t *testing.T, response *GetV1NodesNodeUuidResponse) {
				if response.JSON200 != nil || response.JSON404 == nil {
					t.Fatalf("unexpected decoded response: %+v", response)
				}
				if response.JSON404.Error == nil || *response.JSON404.Error != "not found" || response.JSON404.Details == nil || *response.JSON404.Details != "node does not exist" {
					t.Errorf("error response: %+v", response.JSON404)
				}
			},
		},
		{
			name: "malformed json", query: url.Values{}, status: http.StatusOK,
			body: `{"nodeUUID":`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/nodes/node-1" {
					t.Errorf("request = %s %s", r.Method, r.URL.Path)
				}
				if !reflect.DeepEqual(r.URL.Query(), tt.query) {
					t.Errorf("query = %v, want %v", r.URL.Query(), tt.query)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				if _, err := w.Write([]byte(tt.body)); err != nil {
					t.Errorf("write response: %v", err)
				}
			}))
			defer server.Close()
			client, err := NewClientWithResponses(server.URL)
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			response, err := client.GetV1NodesNodeUuidWithResponse(context.Background(), "node-1", tt.params)
			if tt.check == nil {
				if err == nil || response != nil {
					t.Errorf("expected decoding error and nil response, got response %+v, error %v", response, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("request: %v", err)
			}
			if response == nil {
				t.Fatal("nil response without error")
			}
			if response.StatusCode() != tt.status || string(response.Body) != tt.body {
				t.Errorf("status/body = %d %q, want %d %q", response.StatusCode(), response.Body, tt.status, tt.body)
			}
			tt.check(t, response)
		})
	}
}

func TestNodeOptionsResponseContract(t *testing.T) {
	basic, detail := GetV1NodesOptionsParamsViewBasic, GetV1NodesOptionsParamsViewDetail
	source, agent := GetV1NodesOptionsParamsDataSourceOob, GetV1NodesOptionsParamsAgentTypeOob
	const successBody = `{"filters":{"fields":[{"name":"computeZone","options":[{"id":"cz-1","value":"East"}]}]},"sorting":{"fields":["name","health"],"orders":["asc","desc"],"defaults":{"field":"name","order":"asc"}}}`

	tests := []struct {
		name, contentType, body string
		params                  *GetV1NodesOptionsParams
		query                   url.Values
		status                  int
		wantDecodeError         bool
	}{
		{name: "basic without source", params: &GetV1NodesOptionsParams{View: &basic}, query: url.Values{"view": {"basic"}}, status: http.StatusOK, contentType: "application/json", body: successBody},
		{name: "detail with source and legacy agent", params: &GetV1NodesOptionsParams{View: &detail, DataSource: &source, AgentType: &agent}, query: url.Values{"view": {"detail"}, "dataSource": {"oob"}, "agentType": {"oob"}}, status: http.StatusOK, contentType: "application/json", body: successBody},
		{name: "forbidden", status: http.StatusForbidden, contentType: "application/json", body: `{"error":"forbidden","details":"missing permission"}`},
		{name: "malformed JSON", status: http.StatusOK, contentType: "application/json", body: `{"filters":`, wantDecodeError: true},
		{name: "non-JSON success", status: http.StatusOK, contentType: "text/plain", body: "plain response"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/nodes/options" {
					t.Errorf("request = %s %s", r.Method, r.URL.Path)
				}
				if !reflect.DeepEqual(r.URL.Query(), tt.query) && !(len(r.URL.Query()) == 0 && len(tt.query) == 0) {
					t.Errorf("query = %v, want %v", r.URL.Query(), tt.query)
				}
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(tt.status)
				if _, err := w.Write([]byte(tt.body)); err != nil {
					t.Errorf("write response: %v", err)
				}
			}))
			defer server.Close()
			client, err := NewClientWithResponses(server.URL)
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			response, err := client.GetV1NodesOptionsWithResponse(context.Background(), tt.params)
			if tt.wantDecodeError {
				if err == nil {
					t.Fatalf("expected decoding error, got response %+v", response)
				}
				return
			}
			if err != nil {
				t.Fatalf("request: %v", err)
			}
			if response == nil {
				t.Fatal("nil response without error")
			}
			if response.StatusCode() != tt.status || string(response.Body) != tt.body {
				t.Errorf("status/body = %d %q, want %d %q", response.StatusCode(), response.Body, tt.status, tt.body)
			}
			switch {
			case tt.status == http.StatusForbidden:
				if response.JSON200 != nil || response.JSON403 == nil {
					t.Fatalf("unexpected decoded response: %+v", response)
				}
				if response.JSON403.Error == nil || *response.JSON403.Error != "forbidden" || response.JSON403.Details == nil || *response.JSON403.Details != "missing permission" {
					t.Errorf("error response: %+v", response.JSON403)
				}
			case tt.contentType != "application/json":
				if response.JSON200 != nil {
					t.Errorf("unexpected JSON200: %+v", response.JSON200)
				}
			default:
				result := response.JSON200
				if result == nil || result.Filters == nil || result.Filters.Fields == nil || len(*result.Filters.Fields) != 1 {
					t.Fatalf("missing filter field: %+v", result)
				}
				field := (*result.Filters.Fields)[0]
				if field.Name == nil || *field.Name != "computeZone" || field.Options == nil || len(*field.Options) != 1 {
					t.Fatalf("filter field: %+v", field)
				}
				option := (*field.Options)[0]
				if option["id"] != "cz-1" || option["value"] != "East" {
					t.Errorf("filter option: %+v", option)
				}
				if result.Sorting == nil || result.Sorting.Fields == nil || !reflect.DeepEqual(*result.Sorting.Fields, []string{"name", "health"}) || result.Sorting.Orders == nil || !reflect.DeepEqual(*result.Sorting.Orders, []string{"asc", "desc"}) || result.Sorting.Defaults == nil || result.Sorting.Defaults.Field == nil || *result.Sorting.Defaults.Field != "name" || result.Sorting.Defaults.Order == nil || *result.Sorting.Defaults.Order != "asc" {
					t.Errorf("sorting: %+v", result.Sorting)
				}
			}
		})
	}
}

func TestAlertTimelineFilterOptionsResponseContract(t *testing.T) {
	active, inactive := true, false
	const successBody = `{"filters":{"fields":[{"name":"severity","options":[{"id":"critical","value":"Critical"}]}]},"sorting":{"fields":["name","health"],"orders":["asc","desc"],"defaults":{"field":"name","order":"asc"}}}`

	tests := []struct {
		name, contentType, body string
		params                  *GetV1AlertTimelineFilterOptionsParams
		query                   url.Values
		status                  int
		wantDecodeError         bool
	}{
		{name: "omitted params", status: http.StatusOK, contentType: "application/json", body: successBody},
		{name: "inactive", params: &GetV1AlertTimelineFilterOptionsParams{Active: &inactive}, query: url.Values{"active": {"false"}}, status: http.StatusOK, contentType: "application/json", body: successBody},
		{name: "active", params: &GetV1AlertTimelineFilterOptionsParams{Active: &active}, query: url.Values{"active": {"true"}}, status: http.StatusOK, contentType: "application/json", body: successBody},
		{name: "unauthorized", status: http.StatusUnauthorized, contentType: "application/json", body: `{"error":"unauthorized","details":"missing credentials"}`},
		{name: "malformed JSON", status: http.StatusOK, contentType: "application/json", body: `{"filters":`, wantDecodeError: true},
		{name: "non-JSON success", status: http.StatusOK, contentType: "text/plain", body: "plain response"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/alert_timeline/filter_options" {
					t.Errorf("request = %s %s", r.Method, r.URL.Path)
				}
				if !reflect.DeepEqual(r.URL.Query(), tt.query) && !(len(r.URL.Query()) == 0 && len(tt.query) == 0) {
					t.Errorf("query = %v, want %v", r.URL.Query(), tt.query)
				}
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(tt.status)
				if _, err := w.Write([]byte(tt.body)); err != nil {
					t.Errorf("write response: %v", err)
				}
			}))
			defer server.Close()
			client, err := NewClientWithResponses(server.URL)
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			response, err := client.GetV1AlertTimelineFilterOptionsWithResponse(context.Background(), tt.params)
			if tt.wantDecodeError {
				if err == nil {
					t.Fatalf("expected decoding error, got response %+v", response)
				}
				return
			}
			if err != nil {
				t.Fatalf("request: %v", err)
			}
			if response == nil {
				t.Fatal("nil response without error")
			}
			if response.StatusCode() != tt.status || string(response.Body) != tt.body {
				t.Errorf("status/body = %d %q, want %d %q", response.StatusCode(), response.Body, tt.status, tt.body)
			}
			switch {
			case tt.status == http.StatusUnauthorized:
				if response.JSON200 != nil || response.JSON401 == nil {
					t.Fatalf("unexpected decoded response: %+v", response)
				}
				if response.JSON401.Error == nil || *response.JSON401.Error != "unauthorized" || response.JSON401.Details == nil || *response.JSON401.Details != "missing credentials" {
					t.Errorf("error response: %+v", response.JSON401)
				}
			case tt.contentType != "application/json":
				if response.JSON200 != nil {
					t.Errorf("unexpected JSON200: %+v", response.JSON200)
				}
			default:
				result := response.JSON200
				if result == nil || result.Filters == nil || result.Filters.Fields == nil || len(*result.Filters.Fields) != 1 {
					t.Fatalf("missing filter field: %+v", result)
				}
				field := (*result.Filters.Fields)[0]
				if field.Name == nil || *field.Name != "severity" || field.Options == nil || len(*field.Options) != 1 {
					t.Fatalf("filter field: %+v", field)
				}
				option := (*field.Options)[0]
				if option["id"] != "critical" || option["value"] != "Critical" {
					t.Errorf("filter option: %+v", option)
				}
				if result.Sorting == nil || result.Sorting.Fields == nil || !reflect.DeepEqual(*result.Sorting.Fields, []string{"name", "health"}) || result.Sorting.Orders == nil || !reflect.DeepEqual(*result.Sorting.Orders, []string{"asc", "desc"}) || result.Sorting.Defaults == nil || result.Sorting.Defaults.Field == nil || *result.Sorting.Defaults.Field != "name" || result.Sorting.Defaults.Order == nil || *result.Sorting.Defaults.Order != "asc" {
					t.Errorf("sorting: %+v", result.Sorting)
				}
			}
		})
	}
}

func TestNodeHealthHistoryResponseContract(t *testing.T) {
	const nodeUUID = "1e9c0d2a-0000-4a1b-9c3d-000000000001"
	const startTime = "2026-04-07T00:00:00Z"
	const endTime = "2026-04-14T00:00:00Z"

	tests := []struct {
		name, contentType, body string
		status                  int
		wantError, wantDetails  string
		malformed               bool
	}{
		{name: "history", status: http.StatusOK, contentType: "application/json; charset=utf-8", body: `{"enrolledAt":"2026-04-01T00:00:00Z","healthSummary":{"healthyPercentage":99.5,"degradedDurationSeconds":3000},"machineStatus":[{"startTime":"2026-04-07T00:00:00Z","endTime":"2026-04-08T00:00:00Z","status":"healthy"},{"startTime":"2026-04-08T00:00:00Z","endTime":"2026-04-14T00:00:00Z","status":"degraded"}]}`},
		{name: "bad request", status: http.StatusBadRequest, contentType: "application/json", body: `{"error":"bad request","details":"invalid time range"}`, wantError: "bad request", wantDetails: "invalid time range"},
		{name: "unauthorized", status: http.StatusUnauthorized, contentType: "application/json", body: `{"error":"unauthorized","details":"authentication required"}`, wantError: "unauthorized", wantDetails: "authentication required"},
		{name: "not found", status: http.StatusNotFound, contentType: "application/json", body: `{"error":"not found","details":"node missing"}`, wantError: "not found", wantDetails: "node missing"},
		{name: "plain not found", status: http.StatusNotFound, contentType: "text/plain", body: "node missing"},
		{name: "malformed json", status: http.StatusOK, contentType: "application/json", body: `{"enrolledAt":`, malformed: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/nodes/"+nodeUUID+"/health_history" {
					t.Errorf("request = %s %s", r.Method, r.URL.Path)
				}
				wantQuery := url.Values{"startTime": {startTime}, "endTime": {endTime}}
				if !reflect.DeepEqual(r.URL.Query(), wantQuery) {
					t.Errorf("query = %v, want %v", r.URL.Query(), wantQuery)
				}
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(tt.status)
				if _, err := w.Write([]byte(tt.body)); err != nil {
					t.Errorf("write response: %v", err)
				}
			}))
			defer server.Close()
			client, err := NewClientWithResponses(server.URL)
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			response, err := client.GetV1NodesNodeUuidHealthHistoryWithResponse(context.Background(), nodeUUID, &GetV1NodesNodeUuidHealthHistoryParams{StartTime: startTime, EndTime: endTime})
			if tt.malformed {
				if err == nil || response != nil {
					t.Errorf("expected decoding error and nil response, got response %+v, error %v", response, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("request: %v", err)
			}
			if response == nil {
				t.Fatal("nil response without error")
			}
			if response.StatusCode() != tt.status || string(response.Body) != tt.body {
				t.Errorf("status/body = %d %q, want %d %q", response.StatusCode(), response.Body, tt.status, tt.body)
			}
			if tt.status == http.StatusOK {
				history := response.JSON200
				if history == nil {
					t.Fatal("missing JSON200")
				}
				if history.EnrolledAt == nil || *history.EnrolledAt != "2026-04-01T00:00:00Z" || history.HealthSummary == nil || history.HealthSummary.HealthyPercentage == nil || *history.HealthSummary.HealthyPercentage != 99.5 || history.HealthSummary.DegradedDurationSeconds == nil || *history.HealthSummary.DegradedDurationSeconds != 3000 {
					t.Errorf("history summary: %+v", history)
				}
				if history.MachineStatus == nil || len(*history.MachineStatus) != 2 {
					t.Fatalf("machine status: %+v", history.MachineStatus)
				}
				segments := *history.MachineStatus
				for i, want := range []struct{ start, end, status string }{{startTime, "2026-04-08T00:00:00Z", "healthy"}, {"2026-04-08T00:00:00Z", endTime, "degraded"}} {
					segment := segments[i]
					if segment.StartTime == nil || *segment.StartTime != want.start || segment.EndTime == nil || *segment.EndTime != want.end || segment.Status == nil || string(*segment.Status) != want.status {
						t.Errorf("segment %d = %+v, want %+v", i, segment, want)
					}
				}
			} else {
				if response.JSON200 != nil {
					t.Errorf("unexpected JSON200: %+v", response.JSON200)
				}
				var typedError *ModelsErrorResponse
				switch tt.status {
				case http.StatusBadRequest:
					typedError = response.JSON400
				case http.StatusUnauthorized:
					typedError = response.JSON401
				case http.StatusNotFound:
					typedError = response.JSON404
				}
				if tt.wantError == "" {
					if response.JSON400 != nil || response.JSON401 != nil || response.JSON404 != nil || response.JSON499 != nil || response.JSON500 != nil {
						t.Errorf("unexpected typed error: %+v", response)
					}
				} else if typedError == nil || typedError.Error == nil || *typedError.Error != tt.wantError || typedError.Details == nil || *typedError.Details != tt.wantDetails {
					t.Errorf("typed error = %+v, want error %q and details %q", typedError, tt.wantError, tt.wantDetails)
				}
			}
		})
	}
}

func TestNodeGroupOptionsResponseContract(t *testing.T) {
	tests := []struct {
		name, contentType, body string
		status                  int
		check                   func(*testing.T, *GetV1NodegroupsOptionsResponse, error)
	}{
		{
			name: "success", status: http.StatusOK, contentType: "application/json",
			body: `{"filters":{"fields":[{"name":"computeZones","options":[{"id":"cz-1","value":"East","options":[{"id":"ng-1","value":"Training"}]}]}]},"sorting":{"fields":["health","nodes"],"orders":["asc","desc"],"defaults":{"field":"health","order":"desc"}}}`,
			check: func(t *testing.T, response *GetV1NodegroupsOptionsResponse, err error) {
				if err != nil {
					t.Fatalf("request: %v", err)
				}
				options := response.JSON200
				if options == nil || options.Filters == nil || options.Filters.Fields == nil || len(*options.Filters.Fields) != 1 {
					t.Fatalf("missing filter fields: %+v", options)
				}
				field := (*options.Filters.Fields)[0]
				if field.Name == nil || *field.Name != "computeZones" || field.Options == nil || len(*field.Options) != 1 {
					t.Fatalf("filter field: %+v", field)
				}
				want := map[string]interface{}{"id": "cz-1", "value": "East", "options": []interface{}{map[string]interface{}{"id": "ng-1", "value": "Training"}}}
				if got := (*field.Options)[0]; !reflect.DeepEqual(got, want) {
					t.Errorf("hierarchical option = %#v, want %#v", got, want)
				}
				sorting := options.Sorting
				if sorting == nil || sorting.Fields == nil || !reflect.DeepEqual(*sorting.Fields, []string{"health", "nodes"}) || sorting.Orders == nil || !reflect.DeepEqual(*sorting.Orders, []string{"asc", "desc"}) || sorting.Defaults == nil || sorting.Defaults.Field == nil || *sorting.Defaults.Field != "health" || sorting.Defaults.Order == nil || *sorting.Defaults.Order != "desc" {
					t.Errorf("sorting = %+v", sorting)
				}
			},
		},
		{
			name: "unauthorized", status: http.StatusUnauthorized, contentType: "application/json",
			body: `{"error":"unauthorized","details":"missing credentials"}`,
			check: func(t *testing.T, response *GetV1NodegroupsOptionsResponse, err error) {
				if err != nil {
					t.Fatalf("request: %v", err)
				}
				if response.JSON200 != nil || response.JSON401 == nil || response.JSON403 != nil {
					t.Fatalf("unexpected decoded response: %+v", response)
				}
				if response.JSON401.Error == nil || *response.JSON401.Error != "unauthorized" || response.JSON401.Details == nil || *response.JSON401.Details != "missing credentials" {
					t.Errorf("error response: %+v", response.JSON401)
				}
			},
		},
		{
			name: "forbidden", status: http.StatusForbidden, contentType: "application/json",
			body: `{"error":"forbidden","details":"missing permission"}`,
			check: func(t *testing.T, response *GetV1NodegroupsOptionsResponse, err error) {
				if err != nil {
					t.Fatalf("request: %v", err)
				}
				if response.JSON200 != nil || response.JSON403 == nil || response.JSON401 != nil {
					t.Fatalf("unexpected decoded response: %+v", response)
				}
				if response.JSON403.Error == nil || *response.JSON403.Error != "forbidden" || response.JSON403.Details == nil || *response.JSON403.Details != "missing permission" {
					t.Errorf("error response: %+v", response.JSON403)
				}
			},
		},
		{
			name: "malformed json", status: http.StatusOK, contentType: "application/json", body: `{"filters":`,
			check: func(t *testing.T, response *GetV1NodegroupsOptionsResponse, err error) {
				if err == nil {
					t.Errorf("expected JSON decoding error, got response %+v", response)
				}
			},
		},
		{
			name: "plain text", status: http.StatusOK, contentType: "text/plain", body: "options unavailable",
			check: func(t *testing.T, response *GetV1NodegroupsOptionsResponse, err error) {
				if err != nil {
					t.Fatalf("request: %v", err)
				}
				if response.JSON200 != nil {
					t.Errorf("unexpected decoded response: %+v", response.JSON200)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/nodegroups/options" || r.URL.RawQuery != "" || r.URL.ForceQuery {
					t.Errorf("request = %s %s", r.Method, r.URL.String())
				}
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(tt.status)
				if _, err := w.Write([]byte(tt.body)); err != nil {
					t.Errorf("write response: %v", err)
				}
			}))
			defer server.Close()
			client, err := NewClientWithResponses(server.URL)
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			response, err := client.GetV1NodegroupsOptionsWithResponse(context.Background())
			if err == nil {
				if response == nil {
					t.Fatal("nil response without error")
				}
				if response.StatusCode() != tt.status || string(response.Body) != tt.body {
					t.Errorf("status/body = %d %q, want %d %q", response.StatusCode(), response.Body, tt.status, tt.body)
				}
			}
			tt.check(t, response, err)
		})
	}
}

func TestNodeGroupMembershipsResponseContract(t *testing.T) {
	groupID, nodeUUID := "group /&+?", "node /&+?"
	page, pageSize := 0, 20
	params := &GetV1NodeGroupMembershipsParams{NodeGroupId: &groupID, NodeUUID: &nodeUUID, Page: &page, PageSize: &pageSize}
	query := url.Values{"nodeGroupId": {groupID}, "nodeUUID": {nodeUUID}, "page": {"0"}, "pageSize": {"20"}}
	const successBody = `{"memberships":[{"nodeGroupId":"group /&+?","nodeUUID":"node /&+?","createdAt":"2026-01-02T03:04:05Z","updatedAt":"2026-02-03T04:05:06Z"}],"page":0,"pageSize":20,"total":21,"hasMore":true}`

	tests := []struct {
		name, contentType, body string
		params                  *GetV1NodeGroupMembershipsParams
		query                   url.Values
		status                  int
		wantError, wantDetails  string
		malformed               bool
	}{
		{name: "filtered page", params: params, query: query, status: http.StatusOK, contentType: "application/json", body: successBody},
		{name: "nil params", status: http.StatusOK, contentType: "application/json", body: successBody},
		{name: "bad request", status: http.StatusBadRequest, contentType: "application/json", body: `{"error":"bad request","details":"invalid filter"}`, wantError: "bad request", wantDetails: "invalid filter"},
		{name: "forbidden", status: http.StatusForbidden, contentType: "application/json", body: `{"error":"forbidden","details":"missing permission"}`, wantError: "forbidden", wantDetails: "missing permission"},
		{name: "malformed json", status: http.StatusOK, contentType: "application/json", body: `{"memberships":`, malformed: true},
		{name: "non-json", status: http.StatusForbidden, contentType: "text/plain", body: "access denied"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/node_group_memberships" {
					t.Errorf("request = %s %s", r.Method, r.URL.Path)
				}
				if !reflect.DeepEqual(r.URL.Query(), tt.query) && !(len(r.URL.Query()) == 0 && len(tt.query) == 0) {
					t.Errorf("query = %v, want %v", r.URL.Query(), tt.query)
				}
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(tt.status)
				if _, err := w.Write([]byte(tt.body)); err != nil {
					t.Errorf("write response: %v", err)
				}
			}))
			defer server.Close()
			client, err := NewClientWithResponses(server.URL)
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			response, err := client.GetV1NodeGroupMembershipsWithResponse(context.Background(), tt.params)
			if tt.malformed {
				if err == nil {
					t.Fatalf("expected decoding error, got response %+v", response)
				}
				return
			}
			if err != nil {
				t.Fatalf("request: %v", err)
			}
			if response == nil {
				t.Fatal("nil response without error")
			}
			if response.StatusCode() != tt.status || string(response.Body) != tt.body {
				t.Errorf("status/body = %d %q, want %d %q", response.StatusCode(), response.Body, tt.status, tt.body)
			}
			if tt.contentType != "application/json" {
				if response.JSON200 != nil || response.JSON400 != nil || response.JSON401 != nil || response.JSON403 != nil || response.JSON499 != nil || response.JSON500 != nil || response.JSON503 != nil {
					t.Errorf("unexpected typed response: %+v", response)
				}
				return
			}
			if tt.wantError != "" {
				var typedError *ModelsErrorResponse
				switch tt.status {
				case http.StatusBadRequest:
					typedError = response.JSON400
					if response.JSON403 != nil {
						t.Errorf("unexpected JSON403: %+v", response.JSON403)
					}
				case http.StatusForbidden:
					typedError = response.JSON403
					if response.JSON400 != nil {
						t.Errorf("unexpected JSON400: %+v", response.JSON400)
					}
				}
				if response.JSON200 != nil || typedError == nil || typedError.Error == nil || *typedError.Error != tt.wantError || typedError.Details == nil || *typedError.Details != tt.wantDetails {
					t.Errorf("typed response = %+v, want error %q and details %q", response, tt.wantError, tt.wantDetails)
				}
				return
			}
			result := response.JSON200
			if result == nil || result.Memberships == nil || len(*result.Memberships) != 1 {
				t.Fatalf("missing membership: %+v", result)
			}
			membership := (*result.Memberships)[0]
			if membership.NodeGroupId == nil || *membership.NodeGroupId != groupID || membership.NodeUUID == nil || *membership.NodeUUID != nodeUUID || membership.CreatedAt == nil || *membership.CreatedAt != "2026-01-02T03:04:05Z" || membership.UpdatedAt == nil || *membership.UpdatedAt != "2026-02-03T04:05:06Z" {
				t.Errorf("membership = %+v", membership)
			}
			checkComputeZonePage(t, result.Page, result.PageSize, result.Total, result.HasMore, 0, 20, 21, true)
		})
	}
}

func TestSigningKeyPubResponseContract(t *testing.T) {
	tests := []struct {
		name, contentType, body string
		status                  int
		wantError, wantDetails  string
		malformed               bool
	}{
		{name: "public key", status: http.StatusOK, contentType: "application/x-pem-file", body: "-----BEGIN PUBLIC KEY-----\nYWJjZA==\n-----END PUBLIC KEY-----\n"},
		{name: "client closed request", status: 499, contentType: "application/json", body: `{"error":"request canceled","details":"client disconnected"}`, wantError: "request canceled", wantDetails: "client disconnected"},
		{name: "service unavailable", status: http.StatusServiceUnavailable, contentType: "application/json; charset=utf-8", body: `{"error":"unavailable","details":"signing key not ready"}`, wantError: "unavailable", wantDetails: "signing key not ready"},
		{name: "plain service unavailable", status: http.StatusServiceUnavailable, contentType: "text/plain", body: "temporarily unavailable\n"},
		{name: "malformed service unavailable", status: http.StatusServiceUnavailable, contentType: "application/json", body: `{"error":`, malformed: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/.well-known/signing-key.pub" || r.URL.RawQuery != "" || r.URL.ForceQuery {
					t.Errorf("request = %s %s", r.Method, r.URL.String())
				}
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(tt.status)
				if _, err := w.Write([]byte(tt.body)); err != nil {
					t.Errorf("write response: %v", err)
				}
			}))
			defer server.Close()
			client, err := NewClientWithResponses(server.URL)
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			response, err := client.GetWellKnownSigningKeyPubWithResponse(context.Background())
			if tt.malformed {
				if err == nil || response != nil {
					t.Errorf("expected decoding error and nil response, got response %+v, error %v", response, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("request: %v", err)
			}
			if response == nil {
				t.Fatal("nil response without error")
			}
			if response.StatusCode() != tt.status || string(response.Body) != tt.body || string(response.GetBody()) != tt.body {
				t.Errorf("status/body/GetBody = %d %q %q, want %d %q", response.StatusCode(), response.Body, response.GetBody(), tt.status, tt.body)
			}
			var typedError *ModelsErrorResponse
			switch {
			case tt.wantError == "":
				if response.JSON499 != nil || response.JSON503 != nil {
					t.Errorf("unexpected typed errors: %+v", response)
				}
			case tt.status == 499:
				typedError = response.JSON499
				if response.JSON503 != nil {
					t.Errorf("unexpected JSON503: %+v", response.JSON503)
				}
			case tt.status == http.StatusServiceUnavailable:
				typedError = response.JSON503
				if response.JSON499 != nil {
					t.Errorf("unexpected JSON499: %+v", response.JSON499)
				}
			}
			if tt.wantError != "" && (typedError == nil || typedError.Error == nil || *typedError.Error != tt.wantError || typedError.Details == nil || *typedError.Details != tt.wantDetails) {
				t.Errorf("typed error = %+v, want error %q and details %q", typedError, tt.wantError, tt.wantDetails)
			}
		})
	}
}

func TestComputeZoneUpdateResponseContract(t *testing.T) {
	zoneType, email, pic := "datacenter", "ops@example.test", "Operator"
	city, region := "Baltimore", "Maryland"
	latitude, longitude := float32(39.5), float32(-76.25)
	request := ModelsUpdateComputeZoneRequest{
		Id: "cz-1", Type: &zoneType,
		Contact:  &ModelsContact{Email: &email, Pic: &pic},
		Location: &ModelsLocation{City: &city, Region: &region, Latitude: &latitude, Longitude: &longitude},
	}
	const rawRequest = `{"id":"cz-1","type":"datacenter","contact":{"email":"ops@example.test","pic":"Operator"},"location":{"city":"Baltimore","region":"Maryland","latitude":39.04581234,"longitude":-76.64131111}}`
	tests := []struct {
		name, contentType, body string
		status                  int
		raw, malformed          bool
	}{
		{name: "typed success", status: http.StatusOK, contentType: "application/json", body: `{"id":"cz-1"}`},
		{name: "raw success", raw: true, status: http.StatusOK, contentType: "application/json", body: `{"id":"cz-1"}`},
		{name: "json forbidden", status: http.StatusForbidden, contentType: "application/json", body: `{"error":"forbidden","details":"missing permission"}`},
		{name: "malformed success", status: http.StatusOK, contentType: "application/json", body: `{"id":`, malformed: true},
		{name: "non-json forbidden", status: http.StatusForbidden, contentType: "text/plain", body: "access denied"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPut || r.URL.Path != "/v1/computezones" || r.URL.RawQuery != "" {
					t.Errorf("request = %s %s", r.Method, r.URL.String())
				}
				if got := r.Header.Get("Content-Type"); got != "application/json" {
					t.Errorf("Content-Type = %q", got)
				}
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("read request: %v", err)
				} else if tt.raw {
					if string(body) != rawRequest {
						t.Errorf("raw body = %q, want %q", body, rawRequest)
					}
				} else {
					var got map[string]interface{}
					want := map[string]interface{}{
						"id": "cz-1", "type": zoneType,
						"contact":  map[string]interface{}{"email": email, "pic": pic},
						"location": map[string]interface{}{"city": city, "region": region, "latitude": float64(latitude), "longitude": float64(longitude)},
					}
					if err := json.Unmarshal(body, &got); err != nil {
						t.Errorf("decode request: %v", err)
					} else if !reflect.DeepEqual(got, want) {
						t.Errorf("request body = %#v, want %#v", got, want)
					}
				}
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(tt.status)
				if _, err := w.Write([]byte(tt.body)); err != nil {
					t.Errorf("write response: %v", err)
				}
			}))
			defer server.Close()
			client, err := NewClientWithResponses(server.URL)
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			var response *PutV1ComputezonesResponse
			if tt.raw {
				response, err = client.PutV1ComputezonesWithBodyWithResponse(context.Background(), "application/json", strings.NewReader(rawRequest))
			} else {
				response, err = client.PutV1ComputezonesWithResponse(context.Background(), request)
			}
			if tt.malformed {
				if err == nil {
					t.Fatalf("expected decoding error, got response %+v", response)
				}
				return
			}
			if err != nil || response == nil {
				t.Fatalf("request: response %+v, error %v", response, err)
			}
			if response.StatusCode() != tt.status || string(response.Body) != tt.body {
				t.Errorf("status/body = %d %q, want %d %q", response.StatusCode(), response.Body, tt.status, tt.body)
			}
			switch {
			case tt.status == http.StatusOK:
				if response.JSON200 == nil || response.JSON200.Id != "cz-1" || response.JSON403 != nil {
					t.Errorf("success response = %+v", response)
				}
			case tt.contentType == "application/json":
				if response.JSON200 != nil || response.JSON403 == nil || response.JSON403.Error == nil || *response.JSON403.Error != "forbidden" || response.JSON403.Details == nil || *response.JSON403.Details != "missing permission" {
					t.Errorf("forbidden response = %+v", response)
				}
			default:
				if response.JSON200 != nil || response.JSON403 != nil {
					t.Errorf("unexpected typed response = %+v", response)
				}
			}
		})
	}
}

func TestAuthStatusResponseContract(t *testing.T) {
	trueValue, falseValue := true, false
	tests := []struct {
		name, contentType, body string
		status                  int
		wantAuthenticated       *bool
		wantError, wantDetails  string
		malformed               bool
	}{
		{name: "authenticated", status: http.StatusOK, contentType: "application/json", body: `{"authenticated":true}`, wantAuthenticated: &trueValue},
		{name: "not authenticated", status: http.StatusOK, contentType: "application/json", body: `{"authenticated":false}`, wantAuthenticated: &falseValue},
		{name: "authenticated omitted", status: http.StatusOK, contentType: "application/json", body: `{}`},
		{name: "unauthorized", status: http.StatusUnauthorized, contentType: "application/json", body: `{"error":"unauthorized","details":"missing credentials"}`, wantError: "unauthorized", wantDetails: "missing credentials"},
		{name: "forbidden", status: http.StatusForbidden, contentType: "application/json", body: `{"error":"forbidden","details":"missing permission"}`, wantError: "forbidden", wantDetails: "missing permission"},
		{name: "non-json forbidden", status: http.StatusForbidden, contentType: "text/plain", body: "access denied"},
		{name: "malformed json", status: http.StatusOK, contentType: "application/json", body: `{"authenticated":`, malformed: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/auth/status" || r.URL.RawQuery != "" || r.URL.ForceQuery {
					t.Errorf("request = %s %s", r.Method, r.URL.String())
				}
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(tt.status)
				if _, err := w.Write([]byte(tt.body)); err != nil {
					t.Errorf("write response: %v", err)
				}
			}))
			defer server.Close()
			client, err := NewClientWithResponses(server.URL)
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			response, err := client.GetV1AuthStatusWithResponse(context.Background())
			if tt.malformed {
				if err == nil || response != nil {
					t.Errorf("expected decoding error and nil response, got response %+v, error %v", response, err)
				}
				return
			}
			if err != nil || response == nil {
				t.Fatalf("request: response %+v, error %v", response, err)
			}
			if response.StatusCode() != tt.status || string(response.Body) != tt.body {
				t.Errorf("status/body = %d %q, want %d %q", response.StatusCode(), response.Body, tt.status, tt.body)
			}
			switch {
			case tt.status == http.StatusOK:
				if response.JSON200 == nil || response.JSON401 != nil || response.JSON403 != nil || response.JSON499 != nil {
					t.Fatalf("unexpected typed response: %+v", response)
				}
				if !reflect.DeepEqual(response.JSON200.Authenticated, tt.wantAuthenticated) {
					t.Errorf("authenticated = %v, want %v", response.JSON200.Authenticated, tt.wantAuthenticated)
				}
			case tt.contentType != "application/json":
				if response.JSON200 != nil || response.JSON401 != nil || response.JSON403 != nil || response.JSON499 != nil {
					t.Errorf("unexpected typed response: %+v", response)
				}
			default:
				var typedError *ModelsErrorResponse
				if tt.status == http.StatusUnauthorized {
					typedError = response.JSON401
					if response.JSON403 != nil {
						t.Errorf("unexpected JSON403: %+v", response.JSON403)
					}
				} else {
					typedError = response.JSON403
					if response.JSON401 != nil {
						t.Errorf("unexpected JSON401: %+v", response.JSON401)
					}
				}
				if response.JSON200 != nil || response.JSON499 != nil || typedError == nil || typedError.Error == nil || *typedError.Error != tt.wantError || typedError.Details == nil || *typedError.Details != tt.wantDetails {
					t.Errorf("typed response = %+v, want error %q and details %q", response, tt.wantError, tt.wantDetails)
				}
			}
		})
	}
}

func TestNodeAlertsResponseContract(t *testing.T) {
	component, state, severity := "GPU & fan", "Triggered", "Critical"
	page, pageSize := 0, 25
	tests := []struct {
		name, contentType, body string
		status                  int
		params                  *GetV1AlertsNodeUuidParams
		query                   url.Values
		wantError, wantDetails  string
		malformed               bool
	}{
		{
			name: "success", status: http.StatusOK, contentType: "application/json",
			params: &GetV1AlertsNodeUuidParams{Component: &component, State: &state, Severity: &severity, Page: &page, PageSize: &pageSize},
			query:  url.Values{"component": {component}, "state": {state}, "severity": {severity}, "page": {"0"}, "pageSize": {"25"}},
			body:   `{"nodeUUID":"node-1","customerId":"customer-1","page":0,"pageSize":25,"pageCursorNext":"next-cursor","alert":[{"component":"GPU & fan","componentDisplayName":"GPU and fan","data":[{"alertUUID":"alert-1","nodeUUID":"node-1"}]}]}`,
		},
		{name: "json not found", status: http.StatusNotFound, contentType: "application/json", body: `{"error":"not found","details":"unknown node"}`, wantError: "not found", wantDetails: "unknown node"},
		{name: "json bad request", status: http.StatusBadRequest, contentType: "application/json", body: `{"error":"bad request","details":"invalid filter"}`, wantError: "bad request", wantDetails: "invalid filter"},
		{name: "malformed json", status: http.StatusOK, contentType: "application/json", body: `{"alert":`, malformed: true},
		{name: "plain not found", status: http.StatusNotFound, contentType: "text/plain", body: "node not found"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/alerts/node-1" {
					t.Errorf("request = %s %s", r.Method, r.URL.Path)
				}
				if tt.params == nil {
					if r.URL.RawQuery != "" || r.URL.ForceQuery {
						t.Errorf("unexpected query: %s", r.URL.String())
					}
				} else if !reflect.DeepEqual(r.URL.Query(), tt.query) {
					t.Errorf("query = %v, want %v", r.URL.Query(), tt.query)
				}
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(tt.status)
				if _, err := w.Write([]byte(tt.body)); err != nil {
					t.Errorf("write response: %v", err)
				}
			}))
			defer server.Close()
			client, err := NewClientWithResponses(server.URL)
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			response, err := client.GetV1AlertsNodeUuidWithResponse(context.Background(), "node-1", tt.params)
			if tt.malformed {
				if err == nil {
					t.Fatalf("expected decoding error, got response %+v", response)
				}
				return
			}
			if err != nil || response == nil {
				t.Fatalf("request: response %+v, error %v", response, err)
			}
			if response.StatusCode() != tt.status || string(response.Body) != tt.body {
				t.Errorf("status/body = %d %q, want %d %q", response.StatusCode(), response.Body, tt.status, tt.body)
			}
			switch {
			case tt.status == http.StatusOK:
				result := response.JSON200
				if result == nil || response.JSON400 != nil || response.JSON404 != nil {
					t.Fatalf("unexpected typed response: %+v", response)
				}
				if result.NodeUUID != "node-1" || result.CustomerId == nil || *result.CustomerId != "customer-1" || result.Page == nil || *result.Page != 0 || result.PageSize == nil || *result.PageSize != 25 || result.PageCursorNext == nil || *result.PageCursorNext != "next-cursor" {
					t.Errorf("node response = %+v", result)
				}
				if result.Alert == nil || len(*result.Alert) != 1 {
					t.Fatalf("alert groups = %+v", result.Alert)
				}
				group := (*result.Alert)[0]
				if group.Component == nil || *group.Component != component || group.ComponentDisplayName == nil || *group.ComponentDisplayName != "GPU and fan" || group.Data == nil || len(*group.Data) != 1 {
					t.Fatalf("alert group = %+v", group)
				}
				alert := (*group.Data)[0]
				if alert.AlertUUID != "alert-1" || alert.NodeUUID != "node-1" {
					t.Errorf("alert = %+v", alert)
				}
			case tt.contentType == "text/plain":
				if response.JSON200 != nil || response.JSON404 != nil || response.JSON400 != nil {
					t.Errorf("unexpected typed response: %+v", response)
				}
			default:
				var typedError *ModelsErrorResponse
				if tt.status == http.StatusNotFound {
					typedError = response.JSON404
					if response.JSON400 != nil {
						t.Errorf("unexpected JSON400: %+v", response.JSON400)
					}
				} else {
					typedError = response.JSON400
					if response.JSON404 != nil {
						t.Errorf("unexpected JSON404: %+v", response.JSON404)
					}
				}
				if response.JSON200 != nil || typedError == nil || typedError.Error == nil || *typedError.Error != tt.wantError || typedError.Details == nil || *typedError.Details != tt.wantDetails {
					t.Errorf("typed response = %+v, want error %q and details %q", response, tt.wantError, tt.wantDetails)
				}
			}
		})
	}
}

func TestMetricRangeResponseContract(t *testing.T) {
	start, end, window := "2024-01-01T01:00:00Z", "2024-01-01T02:30:00Z", "1h30m"
	absolute, relative := GetV1MetricsQueryRangeParamsTimeModeAbsolute, GetV1MetricsQueryRangeParamsTimeModeRelative
	absoluteParams := &GetV1MetricsQueryRangeParams{Start: &start, End: &end, TimeMode: &absolute, Step: "5m", Component: "gpu", Metric: "gpu_temperature", EntityLevel: GetV1MetricsQueryRangeParamsEntityLevelNode, EntityID: "node-1"}
	relativeParams := &GetV1MetricsQueryRangeParams{TimeMode: &relative, Window: &window, Step: "5m", Component: "gpu", Metric: "gpu_temperature", EntityLevel: GetV1MetricsQueryRangeParamsEntityLevelNode, EntityID: "node-1"}
	absoluteQuery := url.Values{"start": {start}, "end": {end}, "timeMode": {"absolute"}, "step": {"5m"}, "component": {"gpu"}, "metric": {"gpu_temperature"}, "entityLevel": {"node"}, "entityID": {"node-1"}}
	relativeQuery := url.Values{"timeMode": {"relative"}, "window": {window}, "step": {"5m"}, "component": {"gpu"}, "metric": {"gpu_temperature"}, "entityLevel": {"node"}, "entityID": {"node-1"}}
	const successBody = `{"result":[],"step":"5m","stepAggregationFunction":"avg","timeRange":{"start":"2024-01-01T01:00:00Z","end":"2024-01-01T02:30:00Z"},"unavailable":{"code":"unsupported","message":"metric unavailable"},"warning":{"code":"partial","message":"points omitted"}}`
	tests := []struct {
		name, contentType, body string
		params                  *GetV1MetricsQueryRangeParams
		query                   url.Values
		status                  int
		malformed               bool
		wantError, wantDetails  string
	}{
		{name: "absolute", params: absoluteParams, query: absoluteQuery, status: http.StatusOK, contentType: "application/json", body: successBody},
		{name: "relative", params: relativeParams, query: relativeQuery, status: http.StatusOK, contentType: "application/json", body: successBody},
		{name: "bad request", params: absoluteParams, query: absoluteQuery, status: http.StatusBadRequest, contentType: "application/json", body: `{"error":"bad request","details":"invalid range"}`, wantError: "bad request", wantDetails: "invalid range"},
		{name: "forbidden", params: absoluteParams, query: absoluteQuery, status: http.StatusForbidden, contentType: "application/json", body: `{"error":"forbidden","details":"missing permission"}`, wantError: "forbidden", wantDetails: "missing permission"},
		{name: "malformed json", params: absoluteParams, query: absoluteQuery, status: http.StatusOK, contentType: "application/json", body: `{"result":`, malformed: true},
		{name: "non-json forbidden", params: absoluteParams, query: absoluteQuery, status: http.StatusForbidden, contentType: "text/plain", body: "access denied"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/metrics/query_range" {
					t.Errorf("request = %s %s", r.Method, r.URL.Path)
				}
				if !reflect.DeepEqual(r.URL.Query(), tt.query) {
					t.Errorf("query = %v, want %v", r.URL.Query(), tt.query)
				}
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(tt.status)
				if _, err := w.Write([]byte(tt.body)); err != nil {
					t.Errorf("write response: %v", err)
				}
			}))
			defer server.Close()
			client, err := NewClientWithResponses(server.URL)
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			response, err := client.GetV1MetricsQueryRangeWithResponse(context.Background(), tt.params)
			if tt.malformed {
				if err == nil {
					t.Fatalf("expected decoding error, got response %+v", response)
				}
				return
			}
			if err != nil || response == nil {
				t.Fatalf("request: response %+v, error %v", response, err)
			}
			if response.StatusCode() != tt.status || string(response.Body) != tt.body {
				t.Errorf("status/body = %d %q, want %d %q", response.StatusCode(), response.Body, tt.status, tt.body)
			}
			if response.JSON401 != nil || response.JSON404 != nil || response.JSON499 != nil || response.JSON500 != nil || response.JSON503 != nil {
				t.Errorf("unexpected error variant: %+v", response)
			}
			switch {
			case tt.status == http.StatusOK:
				result := response.JSON200
				if result == nil || response.JSON400 != nil || response.JSON403 != nil {
					t.Fatalf("unexpected typed response: %+v", response)
				}
				if result.Result == nil || len(*result.Result) != 0 || result.Step == nil || *result.Step != "5m" || result.StepAggregationFunction == nil || *result.StepAggregationFunction != "avg" {
					t.Errorf("result = %+v", result)
				}
				if result.TimeRange == nil || result.TimeRange.Start == nil || result.TimeRange.End == nil || result.TimeRange.Start.Format("2006-01-02T15:04:05Z07:00") != start || result.TimeRange.End.Format("2006-01-02T15:04:05Z07:00") != end {
					t.Errorf("time range = %+v", result.TimeRange)
				}
				if result.Unavailable.Code == nil || *result.Unavailable.Code != "unsupported" || result.Unavailable.Message == nil || *result.Unavailable.Message != "metric unavailable" || result.Warning.Code == nil || *result.Warning.Code != "partial" || result.Warning.Message == nil || *result.Warning.Message != "points omitted" {
					t.Errorf("unavailable/warning = %+v / %+v", result.Unavailable, result.Warning)
				}
			case tt.contentType != "application/json":
				if response.JSON200 != nil || response.JSON400 != nil || response.JSON403 != nil {
					t.Errorf("unexpected typed response: %+v", response)
				}
			default:
				var typedError *ModelsErrorResponse
				if tt.status == http.StatusBadRequest {
					typedError = response.JSON400
					if response.JSON403 != nil {
						t.Errorf("unexpected JSON403: %+v", response.JSON403)
					}
				} else {
					typedError = response.JSON403
					if response.JSON400 != nil {
						t.Errorf("unexpected JSON400: %+v", response.JSON400)
					}
				}
				if response.JSON200 != nil || typedError == nil || typedError.Error == nil || *typedError.Error != tt.wantError || typedError.Details == nil || *typedError.Details != tt.wantDetails {
					t.Errorf("typed response = %+v, want error %q and details %q", response, tt.wantError, tt.wantDetails)
				}
			}
		})
	}
}

func TestV2AggregatedMetricsResponseContract(t *testing.T) {
	start, end, window := "2024-01-01T01:00:00Z", "2024-01-01T02:30:00Z", "1h30m"
	nodeGroup, zone, groupID := "group alpha", "zone west", "node-1"
	absolute, relative := GetV2MetricsParamsTimeModeAbsolute, GetV2MetricsParamsTimeModeRelative
	absoluteParams := &GetV2MetricsParams{GroupBy: GetV2MetricsParamsGroupByNodegroup, NodeGroupName: &nodeGroup, ComputeZoneName: &zone, MetricNames: []string{"gpu_util"}, StartTime: &start, EndTime: &end, TimeMode: &absolute, Step: "5m"}
	relativeParams := &GetV2MetricsParams{GroupBy: GetV2MetricsParamsGroupByNode, GroupId: &groupID, MetricNames: []string{"gpu_util"}, TimeMode: &relative, Window: &window, Step: "5m"}
	absoluteQuery := url.Values{"groupBy": {"nodegroup"}, "nodeGroupName": {nodeGroup}, "computeZoneName": {zone}, "metricNames": {"gpu_util"}, "startTime": {start}, "endTime": {end}, "timeMode": {"absolute"}, "step": {"5m"}}
	relativeQuery := url.Values{"groupBy": {"node"}, "groupId": {groupID}, "metricNames": {"gpu_util"}, "timeMode": {"relative"}, "window": {window}, "step": {"5m"}}
	const successBody = `{"step":"5m","timeRange":{"start":"2024-01-01T01:00:00Z","end":"2024-01-01T02:30:00Z"},"metrics":[{"name":"gpu_util","aggregation":"average","result":[]}]}`
	tests := []struct {
		name, contentType, body string
		params                  *GetV2MetricsParams
		query                   url.Values
		status                  int
		malformed               bool
	}{
		{"absolute node group", "application/json", successBody, absoluteParams, absoluteQuery, http.StatusOK, false},
		{"relative node", "application/json", successBody, relativeParams, relativeQuery, http.StatusOK, false},
		{"json 499", "application/json", `{"error":"cancelled","details":"query cancelled"}`, absoluteParams, absoluteQuery, 499, false},
		{"malformed json", "application/json", `{"metrics":`, absoluteParams, absoluteQuery, http.StatusOK, true},
		{"non-json", "text/plain", "metrics unavailable", relativeParams, relativeQuery, http.StatusOK, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v2/metrics" {
					t.Errorf("request = %s %s", r.Method, r.URL.Path)
				}
				if !reflect.DeepEqual(r.URL.Query(), tt.query) {
					t.Errorf("query = %v, want %v", r.URL.Query(), tt.query)
				}
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(tt.status)
				if _, err := w.Write([]byte(tt.body)); err != nil {
					t.Errorf("write response: %v", err)
				}
			}))
			defer server.Close()
			client, err := NewClientWithResponses(server.URL)
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			response, err := client.GetV2MetricsWithResponse(context.Background(), tt.params)
			if tt.malformed {
				if err == nil || response != nil {
					t.Fatalf("expected decoding error and nil response, got response %+v, error %v", response, err)
				}
				return
			}
			if err != nil || response == nil {
				t.Fatalf("response = %+v, error = %v", response, err)
			}
			if response.StatusCode() != tt.status || string(response.Body) != tt.body {
				t.Errorf("status/body = %d %q, want %d %q", response.StatusCode(), response.Body, tt.status, tt.body)
			}
			if tt.contentType != "application/json" {
				if response.JSON200 != nil || response.JSON499 != nil {
					t.Errorf("unexpected typed response: %+v", response)
				}
				return
			}
			if tt.status == 499 {
				if response.JSON200 != nil || response.JSON499 == nil || response.JSON499.Error == nil || *response.JSON499.Error != "cancelled" || response.JSON499.Details == nil || *response.JSON499.Details != "query cancelled" {
					t.Errorf("error response = %+v", response)
				}
				return
			}
			result := response.JSON200
			if result == nil || response.JSON499 != nil {
				t.Fatalf("unexpected typed response: %+v", response)
			}
			if result.Step == nil || *result.Step != "5m" || result.TimeRange == nil || result.TimeRange.Start == nil || result.TimeRange.End == nil || result.TimeRange.Start.Format("2006-01-02T15:04:05Z07:00") != start || result.TimeRange.End.Format("2006-01-02T15:04:05Z07:00") != end {
				t.Errorf("step/time range = %+v / %+v", result.Step, result.TimeRange)
			}
			if result.Metrics == nil || len(*result.Metrics) != 1 {
				t.Fatalf("metrics = %+v", result.Metrics)
			}
			metric := (*result.Metrics)[0]
			if metric.Name == nil || *metric.Name != "gpu_util" || metric.Aggregation == nil || *metric.Aggregation != AggregationAverage || metric.Result == nil || len(*metric.Result) != 0 {
				t.Errorf("metric = %+v", metric)
			}
		})
	}
}

func TestMetricsCatalogResponseContract(t *testing.T) {
	critical, aggregatable, metricType, entityID := "true", "true", "utilization", "node-1"
	source, level := GetV1MetricsParamsDataSourceOob, GetV1MetricsParamsEntityLevelNode
	params := &GetV1MetricsParams{IsCritical: &critical, Aggregatable: &aggregatable, Type: &metricType, DataSource: &source, EntityLevel: &level, EntityID: &entityID}
	query := url.Values{"isCritical": {"true"}, "aggregatable": {"true"}, "type": {"utilization"}, "dataSource": {"oob"}, "entityLevel": {"node"}, "entityID": {"node-1"}}

	tests := []struct {
		name, contentType, body string
		status                  int
		check                   func(*testing.T, *GetV1MetricsResponse, error)
	}{
		{
			name: "catalog", status: http.StatusOK, contentType: "application/json",
			body: `{"components":[{"name":"bmc","displayName":"BMC","group":"hardware","isBackendComponent":true,"dataSource":"oob","subComponents":["fan-1","fan-2"],"metrics":[{"name":"fan_utilization","displayName":"Fan utilization","type":"utilization","unit":"percent","dataSource":"oob"}]}]}`,
			check: func(t *testing.T, response *GetV1MetricsResponse, err error) {
				if err != nil {
					t.Fatalf("request: %v", err)
				}
				if response.JSON200 == nil || response.JSON200.Components == nil || len(*response.JSON200.Components) != 1 {
					t.Fatalf("missing component: %+v", response.JSON200)
				}
				component := (*response.JSON200.Components)[0]
				if component.Name == nil || *component.Name != "bmc" || component.DisplayName == nil || *component.DisplayName != "BMC" || component.Group == nil || *component.Group != "hardware" || component.IsBackendComponent == nil || !*component.IsBackendComponent || component.DataSource == nil || *component.DataSource != "oob" || component.SubComponents == nil || !reflect.DeepEqual(*component.SubComponents, []string{"fan-1", "fan-2"}) {
					t.Errorf("component = %+v", component)
				}
				if component.Metrics == nil || len(*component.Metrics) != 1 {
					t.Fatalf("missing metric: %+v", component.Metrics)
				}
				metric := (*component.Metrics)[0]
				if metric.Name == nil || *metric.Name != "fan_utilization" || metric.DisplayName == nil || *metric.DisplayName != "Fan utilization" || metric.Type == nil || *metric.Type != "utilization" || metric.Unit == nil || string(*metric.Unit) != "percent" || metric.DataSource == nil || *metric.DataSource != "oob" {
					t.Errorf("metric = %+v", metric)
				}
			},
		},
		{
			name: "empty components", status: http.StatusOK, contentType: "application/json", body: `{"components":[]}`,
			check: func(t *testing.T, response *GetV1MetricsResponse, err error) {
				if err != nil {
					t.Fatalf("request: %v", err)
				}
				if response.JSON200 == nil || response.JSON200.Components == nil || len(*response.JSON200.Components) != 0 {
					t.Errorf("empty catalog = %+v", response.JSON200)
				}
			},
		},
		{
			name: "json forbidden", status: http.StatusForbidden, contentType: "application/json", body: `{"error":"forbidden","details":"missing permission"}`,
			check: func(t *testing.T, response *GetV1MetricsResponse, err error) {
				if err != nil {
					t.Fatalf("request: %v", err)
				}
				if response.JSON200 != nil || response.JSON403 == nil {
					t.Fatalf("unexpected decoded response: %+v", response)
				}
				if response.JSON403.Error == nil || *response.JSON403.Error != "forbidden" || response.JSON403.Details == nil || *response.JSON403.Details != "missing permission" {
					t.Errorf("error response: %+v", response.JSON403)
				}
			},
		},
		{
			name: "malformed json", status: http.StatusOK, contentType: "application/json", body: `{"components":`,
			check: func(t *testing.T, response *GetV1MetricsResponse, err error) {
				if err == nil {
					t.Errorf("expected JSON decoding error, got response %+v", response)
				}
			},
		},
		{
			name: "plain text forbidden", status: http.StatusForbidden, contentType: "text/plain", body: "access denied",
			check: func(t *testing.T, response *GetV1MetricsResponse, err error) {
				if err != nil {
					t.Fatalf("request: %v", err)
				}
				if response.JSON200 != nil || response.JSON401 != nil || response.JSON403 != nil || response.JSON499 != nil || response.JSON500 != nil {
					t.Errorf("unexpected decoded response: %+v", response)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/metrics" {
					t.Errorf("request = %s %s", r.Method, r.URL.Path)
				}
				if !reflect.DeepEqual(r.URL.Query(), query) {
					t.Errorf("query = %v, want %v", r.URL.Query(), query)
				}
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(tt.status)
				if _, err := w.Write([]byte(tt.body)); err != nil {
					t.Errorf("write response: %v", err)
				}
			}))
			defer server.Close()
			client, err := NewClientWithResponses(server.URL)
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			response, err := client.GetV1MetricsWithResponse(context.Background(), params)
			if err == nil {
				if response == nil {
					t.Fatal("nil response without error")
				}
				if response.StatusCode() != tt.status || string(response.Body) != tt.body {
					t.Errorf("status/body = %d %q, want %d %q", response.StatusCode(), response.Body, tt.status, tt.body)
				}
			}
			tt.check(t, response, err)
		})
	}
}

func TestNodeRemovalHistoryResponseContract(t *testing.T) {
	page, pageSize := 0, 2
	actorType := GetV1NodesHistoryParamsActorTypeUser
	const nodeUUID = "1e9c0d2a-0000-4a1b-9c3d-000000000001"

	tests := []struct {
		name, contentType, body string
		status                  int
		wantError, wantDetails  string
		malformed               bool
	}{
		{name: "history", status: http.StatusOK, contentType: "application/json; charset=utf-8", body: `{"history":[{"nodeUuid":"1e9c0d2a-0000-4a1b-9c3d-000000000001","deletedAt":"2026-04-01T00:00:00Z","deletedBy":"alice","actorType":"user","hostname":"node-a","computeZone":"zone-a","nodeGroup":"group-a"},{"nodeUuid":"1e9c0d2a-0000-4a1b-9c3d-000000000001","deletedAt":"2026-04-14T00:00:00Z","deletedBy":"bob","actorType":"user","hostname":"node-b","computeZone":"zone-b","nodeGroup":"group-b"}],"page":0,"pageSize":2,"total":3,"hasMore":true}`},
		{name: "bad request", status: http.StatusBadRequest, contentType: "application/json", body: `{"error":"bad request","details":"invalid page"}`, wantError: "bad request", wantDetails: "invalid page"},
		{name: "forbidden", status: http.StatusForbidden, contentType: "application/json", body: `{"error":"forbidden","details":"access denied"}`, wantError: "forbidden", wantDetails: "access denied"},
		{name: "malformed json", status: http.StatusOK, contentType: "application/json", body: `{"history":`, malformed: true},
		{name: "plain forbidden", status: http.StatusForbidden, contentType: "text/plain", body: "access denied"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/nodes/history" {
					t.Errorf("request = %s %s", r.Method, r.URL.Path)
				}
				wantQuery := url.Values{"page": {"0"}, "pageSize": {"2"}, "actorType": {"user"}}
				if !reflect.DeepEqual(r.URL.Query(), wantQuery) {
					t.Errorf("query = %v, want %v", r.URL.Query(), wantQuery)
				}
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(tt.status)
				if _, err := w.Write([]byte(tt.body)); err != nil {
					t.Errorf("write response: %v", err)
				}
			}))
			defer server.Close()

			client, err := NewClientWithResponses(server.URL)
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			response, err := client.GetV1NodesHistoryWithResponse(context.Background(), &GetV1NodesHistoryParams{Page: &page, PageSize: &pageSize, ActorType: &actorType})
			if tt.malformed {
				if err == nil || response != nil {
					t.Errorf("expected decoding error and nil response, got response %+v, error %v", response, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("request: %v", err)
			}
			if response == nil {
				t.Fatal("nil response without error")
			}
			if response.StatusCode() != tt.status || string(response.Body) != tt.body {
				t.Errorf("status/body = %d %q, want %d %q", response.StatusCode(), response.Body, tt.status, tt.body)
			}
			if tt.status == http.StatusOK {
				history := response.JSON200
				if history == nil {
					t.Fatal("missing JSON200")
				}
				if history.Page == nil || *history.Page != 0 || history.PageSize == nil || *history.PageSize != 2 || history.Total == nil || *history.Total != 3 || history.HasMore == nil || !*history.HasMore {
					t.Errorf("pagination: %+v", history)
				}
				if history.History == nil || len(*history.History) != 2 {
					t.Fatalf("history entries: %+v", history.History)
				}
				for i, want := range []struct{ deletedAt, deletedBy, hostname, zone, group string }{
					{"2026-04-01T00:00:00Z", "alice", "node-a", "zone-a", "group-a"},
					{"2026-04-14T00:00:00Z", "bob", "node-b", "zone-b", "group-b"},
				} {
					entry := (*history.History)[i]
					if entry.NodeUuid == nil || *entry.NodeUuid != nodeUUID || entry.DeletedAt == nil || *entry.DeletedAt != want.deletedAt || entry.DeletedBy == nil || *entry.DeletedBy != want.deletedBy || entry.ActorType == nil || *entry.ActorType != "user" || entry.Hostname == nil || *entry.Hostname != want.hostname || entry.ComputeZone == nil || *entry.ComputeZone != want.zone || entry.NodeGroup == nil || *entry.NodeGroup != want.group {
						t.Errorf("entry %d = %+v, want %+v", i, entry, want)
					}
				}
				return
			}
			if response.JSON200 != nil {
				t.Errorf("unexpected JSON200: %+v", response.JSON200)
			}
			if tt.wantError == "" {
				if response.JSON400 != nil || response.JSON401 != nil || response.JSON403 != nil || response.JSON500 != nil || response.JSON503 != nil {
					t.Errorf("unexpected typed error: %+v", response)
				}
				return
			}
			var typedError *ModelsErrorResponse
			if tt.status == http.StatusBadRequest {
				typedError = response.JSON400
			} else {
				typedError = response.JSON403
			}
			if typedError == nil || typedError.Error == nil || *typedError.Error != tt.wantError || typedError.Details == nil || *typedError.Details != tt.wantDetails {
				t.Errorf("typed error = %+v, want error %q and details %q", typedError, tt.wantError, tt.wantDetails)
			}
		})
	}
}

func TestInfoResponseContract(t *testing.T) {
	tests := []struct {
		name, contentType, body string
		status                  int
		wantError, wantDetails  string
		malformed               bool
	}{
		{name: "info", status: http.StatusOK, contentType: "application/json; charset=utf-8", body: `{"components":{"apiBackend":{"version":"api-1.2","build":"api-build-1"},"agentBackend":{"version":"agent-3.4","build":"agent-build-2"}},"signingKey":{"isRecentlyRotated":false,"lastRotatedAt":"2026-04-01T12:34:56Z"}}`},
		{name: "bad request", status: http.StatusBadRequest, contentType: "application/json", body: `{"error":"bad request","details":"invalid request"}`, wantError: "bad request", wantDetails: "invalid request"},
		{name: "client closed request", status: 499, contentType: "application/json", body: `{"error":"canceled","details":"client disconnected"}`, wantError: "canceled", wantDetails: "client disconnected"},
		{name: "internal error", status: http.StatusInternalServerError, contentType: "application/json", body: `{"error":"internal error","details":"info unavailable"}`, wantError: "internal error", wantDetails: "info unavailable"},
		{name: "plain internal error", status: http.StatusInternalServerError, contentType: "text/plain", body: "info unavailable\n"},
		{name: "malformed json", status: http.StatusOK, contentType: "application/json", body: `{"components":`, malformed: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/info" || r.URL.RawQuery != "" || r.URL.ForceQuery {
					t.Errorf("request = %s %s", r.Method, r.URL.String())
				}
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(tt.status)
				if _, err := w.Write([]byte(tt.body)); err != nil {
					t.Errorf("write response: %v", err)
				}
			}))
			defer server.Close()
			client, err := NewClientWithResponses(server.URL)
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			response, err := client.GetV1InfoWithResponse(context.Background())
			if tt.malformed {
				if err == nil || response != nil {
					t.Errorf("expected decoding error and nil response, got response %+v, error %v", response, err)
				}
				return
			}
			if err != nil || response == nil {
				t.Fatalf("request: response %+v, error %v", response, err)
			}
			if response.StatusCode() != tt.status || string(response.Body) != tt.body {
				t.Errorf("status/body = %d %q, want %d %q", response.StatusCode(), response.Body, tt.status, tt.body)
			}
			isJSON := tt.contentType != "text/plain"
			if (response.JSON200 != nil) != (isJSON && tt.status == http.StatusOK) ||
				(response.JSON400 != nil) != (isJSON && tt.status == http.StatusBadRequest) ||
				(response.JSON499 != nil) != (isJSON && tt.status == 499) ||
				(response.JSON500 != nil) != (isJSON && tt.status == http.StatusInternalServerError) {
				t.Fatalf("unexpected typed response: %+v", response)
			}
			if response.JSON200 != nil {
				info := response.JSON200
				if info.Components == nil || info.Components.ApiBackend == nil || info.Components.AgentBackend == nil || info.SigningKey == nil {
					t.Fatalf("missing info fields: %+v", info)
				}
				api, agent, key := info.Components.ApiBackend, info.Components.AgentBackend, info.SigningKey
				if api.Version == nil || *api.Version != "api-1.2" || api.Build == nil || *api.Build != "api-build-1" || agent.Version == nil || *agent.Version != "agent-3.4" || agent.Build == nil || *agent.Build != "agent-build-2" {
					t.Errorf("components = api %+v, agent %+v", api, agent)
				}
				if key.IsRecentlyRotated == nil || *key.IsRecentlyRotated || key.LastRotatedAt == nil || *key.LastRotatedAt != "2026-04-01T12:34:56Z" {
					t.Errorf("signing key = %+v", key)
				}
			}
			if tt.wantError != "" {
				var typedError *ModelsErrorResponse
				switch tt.status {
				case http.StatusBadRequest:
					typedError = response.JSON400
				case 499:
					typedError = response.JSON499
				case http.StatusInternalServerError:
					typedError = response.JSON500
				}
				if typedError == nil || typedError.Error == nil || *typedError.Error != tt.wantError || typedError.Details == nil || *typedError.Details != tt.wantDetails {
					t.Errorf("typed error = %+v, want error %q and details %q", typedError, tt.wantError, tt.wantDetails)
				}
			}
		})
	}
}

func TestRetentionPolicyResponseContract(t *testing.T) {
	tests := []struct {
		name, method, body, wantError, wantDetails string
		status                                     int
		malformed                                  bool
	}{
		{name: "get policy", method: http.MethodGet, status: http.StatusOK, body: `{"retentionDays":90,"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-02-01T12:00:00Z","updatedBy":"alice"}`},
		{name: "get forbidden", method: http.MethodGet, status: http.StatusForbidden, body: `{"error":"forbidden","details":"read permission required"}`, wantError: "forbidden", wantDetails: "read permission required"},
		{name: "get not found", method: http.MethodGet, status: http.StatusNotFound, body: `{"error":"not found","details":"policy missing"}`, wantError: "not found", wantDetails: "policy missing"},
		{name: "get malformed", method: http.MethodGet, status: http.StatusOK, body: `{"retentionDays":`, malformed: true},
		{name: "put policy", method: http.MethodPut, status: http.StatusNoContent},
		{name: "put bad request", method: http.MethodPut, status: http.StatusBadRequest, body: `{"error":"bad request","details":"invalid retention"}`, wantError: "bad request", wantDetails: "invalid retention"},
		{name: "put forbidden", method: http.MethodPut, status: http.StatusForbidden, body: `{"error":"forbidden","details":"write permission required"}`, wantError: "forbidden", wantDetails: "write permission required"},
		{name: "put malformed", method: http.MethodPut, status: http.StatusBadRequest, body: `{"error":`, malformed: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != tt.method || r.URL.Path != "/v1/retention_policy" || r.URL.RawQuery != "" || r.URL.ForceQuery {
					t.Errorf("request = %s %s, want %s /v1/retention_policy", r.Method, r.URL.String(), tt.method)
				}
				if tt.method == http.MethodPut {
					if got := r.Header.Get("Content-Type"); got != "application/json" {
						t.Errorf("content type = %q, want application/json", got)
					}
					var body ModelsUpdateRetentionPolicyRequest
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Errorf("decode request: %v", err)
					} else if body.RetentionDays != 365 {
						t.Errorf("retentionDays = %d, want 365", body.RetentionDays)
					}
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				if tt.body != "" {
					if _, err := w.Write([]byte(tt.body)); err != nil {
						t.Errorf("write response: %v", err)
					}
				}
			}))
			defer server.Close()
			client, err := NewClientWithResponses(server.URL)
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			var typedError *ModelsErrorResponse
			if tt.method == http.MethodGet {
				response, err := client.GetV1RetentionPolicyWithResponse(context.Background())
				if tt.malformed {
					if err == nil || response != nil {
						t.Errorf("expected decoding error and nil response, got response %+v, error %v", response, err)
					}
					return
				}
				if err != nil || response == nil {
					t.Fatalf("request: response %+v, error %v", response, err)
				}
				if response.StatusCode() != tt.status || string(response.Body) != tt.body {
					t.Errorf("status/body = %d %q, want %d %q", response.StatusCode(), response.Body, tt.status, tt.body)
				}
				if tt.status == http.StatusOK {
					policy := response.JSON200
					if policy == nil || policy.RetentionDays == nil || *policy.RetentionDays != 90 || policy.CreatedAt == nil || *policy.CreatedAt != "2026-01-01T00:00:00Z" || policy.UpdatedAt == nil || *policy.UpdatedAt != "2026-02-01T12:00:00Z" || policy.UpdatedBy == nil || *policy.UpdatedBy != "alice" {
						t.Errorf("policy = %+v", policy)
					}
					return
				}
				if response.JSON200 != nil {
					t.Errorf("unexpected JSON200: %+v", response.JSON200)
				}
				if tt.status == http.StatusForbidden {
					typedError = response.JSON403
				} else {
					typedError = response.JSON404
				}
			} else {
				response, err := client.PutV1RetentionPolicyWithResponse(context.Background(), ModelsUpdateRetentionPolicyRequest{RetentionDays: 365})
				if tt.malformed {
					if err == nil || response != nil {
						t.Errorf("expected decoding error and nil response, got response %+v, error %v", response, err)
					}
					return
				}
				if err != nil || response == nil {
					t.Fatalf("request: response %+v, error %v", response, err)
				}
				if response.StatusCode() != tt.status || string(response.Body) != tt.body {
					t.Errorf("status/body = %d %q, want %d %q", response.StatusCode(), response.Body, tt.status, tt.body)
				}
				if tt.status == http.StatusNoContent {
					return
				}
				if response.StatusCode() >= 200 && response.StatusCode() < 300 {
					t.Errorf("unexpected successful status: %d", response.StatusCode())
				}
				if tt.status == http.StatusBadRequest {
					typedError = response.JSON400
				} else {
					typedError = response.JSON403
				}
			}
			if typedError == nil || typedError.Error == nil || *typedError.Error != tt.wantError || typedError.Details == nil || *typedError.Details != tt.wantDetails {
				t.Errorf("typed error = %+v, want error %q and details %q", typedError, tt.wantError, tt.wantDetails)
			}
		})
	}
}

func TestNodeRemovalPolicyResponseContract(t *testing.T) {
	threshold := 14
	tests := []struct {
		name, method, body, wantError, wantDetails string
		status                                     int
		request                                    ModelsUpdateNodeRemovalPolicyRequest
		malformed                                  bool
	}{
		{name: "get policy", method: http.MethodGet, status: http.StatusOK, body: `{"enabled":false,"offlineThresholdDays":7,"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-02-01T12:00:00Z","updatedBy":"alice"}`},
		{name: "get forbidden", method: http.MethodGet, status: http.StatusForbidden, body: `{"error":"forbidden","details":"read permission required"}`, wantError: "forbidden", wantDetails: "read permission required"},
		{name: "get malformed", method: http.MethodGet, status: http.StatusOK, body: `{"enabled":`, malformed: true},
		{name: "put disabled without threshold", method: http.MethodPut, status: http.StatusNoContent, request: ModelsUpdateNodeRemovalPolicyRequest{Enabled: false}},
		{name: "put enabled with threshold", method: http.MethodPut, status: http.StatusNoContent, request: ModelsUpdateNodeRemovalPolicyRequest{Enabled: true, OfflineThresholdDays: &threshold}},
		{name: "put bad request", method: http.MethodPut, status: http.StatusBadRequest, body: `{"error":"bad request","details":"invalid threshold"}`, wantError: "bad request", wantDetails: "invalid threshold"},
		{name: "put forbidden", method: http.MethodPut, status: http.StatusForbidden, body: `{"error":"forbidden","details":"write permission required"}`, wantError: "forbidden", wantDetails: "write permission required"},
		{name: "put malformed", method: http.MethodPut, status: http.StatusBadRequest, body: `{"error":`, malformed: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != tt.method || r.URL.Path != "/v1/node_removal_policy" || r.URL.RawQuery != "" || r.URL.ForceQuery {
					t.Errorf("request = %s %s, want %s /v1/node_removal_policy", r.Method, r.URL.String(), tt.method)
				}
				if tt.method == http.MethodPut {
					if got := r.Header.Get("Content-Type"); got != "application/json" {
						t.Errorf("content type = %q, want application/json", got)
					}
					var body map[string]interface{}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Errorf("decode request: %v", err)
					} else {
						if enabled, ok := body["enabled"].(bool); !ok || enabled != tt.request.Enabled {
							t.Errorf("enabled = %v, want %v", body["enabled"], tt.request.Enabled)
						}
						got, present := body["offlineThresholdDays"]
						if tt.request.OfflineThresholdDays == nil {
							if present {
								t.Errorf("unexpected offlineThresholdDays = %v", got)
							}
						} else if !present || got != float64(*tt.request.OfflineThresholdDays) {
							t.Errorf("offlineThresholdDays = %v, want %d", got, *tt.request.OfflineThresholdDays)
						}
					}
				}
				if tt.body != "" {
					w.Header().Set("Content-Type", "application/json")
				}
				w.WriteHeader(tt.status)
				if tt.body != "" {
					if _, err := w.Write([]byte(tt.body)); err != nil {
						t.Errorf("write response: %v", err)
					}
				}
			}))
			defer server.Close()
			client, err := NewClientWithResponses(server.URL)
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			var typedError *ModelsErrorResponse
			if tt.method == http.MethodGet {
				response, err := client.GetV1NodeRemovalPolicyWithResponse(context.Background())
				if tt.malformed {
					if err == nil || response != nil {
						t.Errorf("expected decoding error and nil response, got response %+v, error %v", response, err)
					}
					return
				}
				if err != nil || response == nil {
					t.Fatalf("request: response %+v, error %v", response, err)
				}
				if response.StatusCode() != tt.status || string(response.Body) != tt.body {
					t.Errorf("status/body = %d %q, want %d %q", response.StatusCode(), response.Body, tt.status, tt.body)
				}
				if tt.status == http.StatusOK {
					policy := response.JSON200
					if policy == nil || policy.Enabled == nil || *policy.Enabled || policy.OfflineThresholdDays == nil || *policy.OfflineThresholdDays != 7 || policy.CreatedAt == nil || *policy.CreatedAt != "2026-01-01T00:00:00Z" || policy.UpdatedAt == nil || *policy.UpdatedAt != "2026-02-01T12:00:00Z" || policy.UpdatedBy == nil || *policy.UpdatedBy != "alice" || policy.LastCheckedAt != nil {
						t.Errorf("policy = %+v", policy)
					}
					return
				}
				if response.JSON200 != nil {
					t.Errorf("unexpected JSON200: %+v", response.JSON200)
				}
				typedError = response.JSON403
			} else {
				response, err := client.PutV1NodeRemovalPolicyWithResponse(context.Background(), tt.request)
				if tt.malformed {
					if err == nil || response != nil {
						t.Errorf("expected decoding error and nil response, got response %+v, error %v", response, err)
					}
					return
				}
				if err != nil || response == nil {
					t.Fatalf("request: response %+v, error %v", response, err)
				}
				if response.StatusCode() != tt.status || string(response.Body) != tt.body {
					t.Errorf("status/body = %d %q, want %d %q", response.StatusCode(), response.Body, tt.status, tt.body)
				}
				if tt.status == http.StatusNoContent {
					return
				}
				if tt.status == http.StatusBadRequest {
					typedError = response.JSON400
				} else {
					typedError = response.JSON403
				}
			}
			if typedError == nil || typedError.Error == nil || *typedError.Error != tt.wantError || typedError.Details == nil || *typedError.Details != tt.wantDetails {
				t.Errorf("typed error = %+v, want error %q and details %q", typedError, tt.wantError, tt.wantDetails)
			}
		})
	}
}

func TestAlertMuteRulesResponseContract(t *testing.T) {
	reason := "maintenance"
	warning := []string{"Warning"}
	critical := []string{"Critical"}
	tests := []struct {
		name, method, body, wantError, wantDetails string
		status                                     int
		severities                                 *[]string
		malformed                                  bool
	}{
		{name: "get rules", method: http.MethodGet, status: http.StatusOK, body: `{"total":1,"rules":[{"component":"GPU","componentDisplayName":"Graphics processor","reason":"maintenance","severities":["Critical","Warning"]}]}`},
		{name: "get forbidden", method: http.MethodGet, status: http.StatusForbidden, body: `{"error":"forbidden","details":"read permission required"}`, wantError: "forbidden", wantDetails: "read permission required"},
		{name: "get malformed", method: http.MethodGet, status: http.StatusOK, body: `{"rules":`, malformed: true},
		{name: "put warning", method: http.MethodPut, status: http.StatusNoContent, severities: &warning},
		{name: "put without severities", method: http.MethodPut, status: http.StatusNoContent},
		{name: "put forbidden", method: http.MethodPut, status: http.StatusForbidden, body: `{"error":"forbidden","details":"write permission required"}`, wantError: "forbidden", wantDetails: "write permission required", severities: &warning},
		{name: "delete critical", method: http.MethodDelete, status: http.StatusNoContent, severities: &critical},
		{name: "delete not found", method: http.MethodDelete, status: http.StatusNotFound, body: `{"error":"not found","details":"rule missing"}`, wantError: "not found", wantDetails: "rule missing", severities: &critical},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != tt.method || r.URL.Path != "/v1/alert_mute_rules" || r.URL.RawQuery != "" || r.URL.ForceQuery {
					t.Errorf("request = %s %s, want %s /v1/alert_mute_rules", r.Method, r.URL.String(), tt.method)
				}
				if tt.method != http.MethodGet {
					if got := r.Header.Get("Content-Type"); got != "application/json" {
						t.Errorf("content type = %q, want application/json", got)
					}
					var body map[string]interface{}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Errorf("decode request: %v", err)
					} else {
						if body["component"] != "GPU" {
							t.Errorf("component = %v, want GPU", body["component"])
						}
						if tt.method == http.MethodPut && body["reason"] != reason {
							t.Errorf("reason = %v, want %q", body["reason"], reason)
						}
						got, present := body["severities"]
						if tt.severities == nil {
							if present {
								t.Errorf("unexpected severities = %v", got)
							}
						} else if !present || !reflect.DeepEqual(got, []interface{}{(*tt.severities)[0]}) {
							t.Errorf("severities = %v, want %v", got, *tt.severities)
						}
					}
				}
				if tt.body != "" {
					w.Header().Set("Content-Type", "application/json")
				}
				w.WriteHeader(tt.status)
				if tt.body != "" {
					if _, err := w.Write([]byte(tt.body)); err != nil {
						t.Errorf("write response: %v", err)
					}
				}
			}))
			defer server.Close()
			client, err := NewClientWithResponses(server.URL)
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			var typedError *ModelsErrorResponse
			switch tt.method {
			case http.MethodGet:
				response, err := client.GetV1AlertMuteRulesWithResponse(context.Background())
				if tt.malformed {
					if err == nil || response != nil {
						t.Errorf("expected decoding error and nil response, got response %+v, error %v", response, err)
					}
					return
				}
				if err != nil || response == nil {
					t.Fatalf("request: response %+v, error %v", response, err)
				}
				if response.StatusCode() != tt.status || string(response.Body) != tt.body {
					t.Errorf("status/body = %d %q, want %d %q", response.StatusCode(), response.Body, tt.status, tt.body)
				}
				if tt.status == http.StatusOK {
					result := response.JSON200
					if result == nil || result.Total == nil || *result.Total != 1 || result.Rules == nil || len(*result.Rules) != 1 {
						t.Fatalf("rules response = %+v", result)
					}
					rule := (*result.Rules)[0]
					if rule.Component == nil || *rule.Component != "GPU" || rule.ComponentDisplayName == nil || *rule.ComponentDisplayName != "Graphics processor" || rule.Reason == nil || *rule.Reason != reason || rule.Severities == nil || !reflect.DeepEqual(*rule.Severities, []string{"Critical", "Warning"}) {
						t.Errorf("rule = %+v", rule)
					}
					return
				}
				typedError = response.JSON403
			case http.MethodPut:
				response, err := client.PutV1AlertMuteRulesWithResponse(context.Background(), ModelsUpdateMuteRuleRequest{Component: "GPU", Reason: &reason, Severities: tt.severities})
				if err != nil || response == nil {
					t.Fatalf("request: response %+v, error %v", response, err)
				}
				if response.StatusCode() != tt.status || string(response.Body) != tt.body {
					t.Errorf("status/body = %d %q, want %d %q", response.StatusCode(), response.Body, tt.status, tt.body)
				}
				if tt.status == http.StatusNoContent {
					return
				}
				typedError = response.JSON403
			case http.MethodDelete:
				response, err := client.DeleteV1AlertMuteRulesWithResponse(context.Background(), ModelsDeleteMuteRulesRequest{Component: "GPU", Severities: tt.severities})
				if err != nil || response == nil {
					t.Fatalf("request: response %+v, error %v", response, err)
				}
				if response.StatusCode() != tt.status || string(response.Body) != tt.body {
					t.Errorf("status/body = %d %q, want %d %q", response.StatusCode(), response.Body, tt.status, tt.body)
				}
				if tt.status == http.StatusNoContent {
					return
				}
				typedError = response.JSON404
			}
			if typedError == nil || typedError.Error == nil || *typedError.Error != tt.wantError || typedError.Details == nil || *typedError.Details != tt.wantDetails {
				t.Errorf("typed error = %+v, want error %q and details %q", typedError, tt.wantError, tt.wantDetails)
			}
		})
	}
}

func TestNotificationPreferencesClientContract(t *testing.T) {
	const collection = "/v1/notification_preferences"
	const id = "preference-1"
	components := []string{"GPU", "NVLink"}
	notifiedOn := []string{"Critical"}
	tags := []string{"production", "east"}
	scopeType := ScopeTags
	request := ModelsNotificationPreferenceRequest{
		Components: components,
		NotificationTargets: []ModelsNotificationTarget{{Channel: NotificationChannelEmail,
			Config: map[string]interface{}{"recipients": []string{"ops@example.test", "oncall@example.test"}}}},
		NotifiedOn: &notifiedOn,
		Scope:      &ModelsNotificationScope{Type: &scopeType, Values: &tags},
	}
	detailJSON := `{"id":"preference-1","components":["GPU","NVLink"],"active":false,"notificationTargets":[{"channel":"email","config":{"recipients":["ops@example.test","oncall@example.test"]}}],"notifiedOn":["Critical"],"scope":{"type":"tags","values":{"production":"production","east":"east"}}}`
	checkDetail := func(t *testing.T, detail *ModelsNotificationPreferenceDetail) {
		t.Helper()
		if detail == nil || detail.Id == nil || *detail.Id != id || detail.Components == nil || !reflect.DeepEqual(*detail.Components, components) || detail.Active == nil || *detail.Active || detail.NotifiedOn == nil || !reflect.DeepEqual(*detail.NotifiedOn, notifiedOn) {
			t.Fatalf("detail = %+v", detail)
		}
		if detail.Scope == nil || detail.Scope.Type == nil || *detail.Scope.Type != ScopeTags || detail.Scope.Values == nil || !reflect.DeepEqual(*detail.Scope.Values, map[string]string{"production": "production", "east": "east"}) {
			t.Errorf("scope = %+v", detail.Scope)
		}
		if detail.NotificationTargets == nil || len(*detail.NotificationTargets) != 1 || (*detail.NotificationTargets)[0].Channel != NotificationChannelEmail || !reflect.DeepEqual((*detail.NotificationTargets)[0].Config["recipients"], []interface{}{"ops@example.test", "oncall@example.test"}) {
			t.Errorf("targets = %+v", detail.NotificationTargets)
		}
	}
	checkError := func(t *testing.T, got *ModelsErrorResponse, wantError, wantDetails string) {
		t.Helper()
		if got == nil || got.Error == nil || *got.Error != wantError || got.Details == nil || *got.Details != wantDetails {
			t.Errorf("error response = %+v", got)
		}
	}
	tests := []struct {
		name, method, path, body string
		status                   int
	}{
		{"create", http.MethodPost, collection, `{"id":"preference-1"}`, http.StatusCreated},
		{"list", http.MethodGet, collection, `{"preferences":[` + detailJSON + `],"componentPreferenceCounts":{"GPU":1,"NVLink":1},"total":1}`, http.StatusOK},
		{"detail", http.MethodGet, collection + "/" + id, detailJSON, http.StatusOK},
		{"update", http.MethodPut, collection + "/" + id, "", http.StatusNoContent},
		{"delete", http.MethodDelete, collection + "/" + id, "", http.StatusNoContent},
		{"create bad request", http.MethodPost, collection, `{"error":"bad request","details":"invalid configuration"}`, http.StatusBadRequest},
		{"list forbidden", http.MethodGet, collection, `{"error":"forbidden","details":"missing permission"}`, http.StatusForbidden},
		{"detail not found", http.MethodGet, collection + "/" + id, `{"error":"not found","details":"unknown preference"}`, http.StatusNotFound},
		{"malformed list", http.MethodGet, collection, `{"preferences":`, http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != tt.method || r.URL.Path != tt.path || r.URL.RawQuery != "" || r.URL.ForceQuery {
					t.Errorf("request = %s %s, want %s %s without query", r.Method, r.URL.String(), tt.method, tt.path)
				}
				if tt.method == http.MethodPost || tt.method == http.MethodPut {
					if r.Header.Get("Content-Type") != "application/json" {
						t.Errorf("content type = %q", r.Header.Get("Content-Type"))
					}
					var raw map[string]json.RawMessage
					if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
						t.Errorf("decode request: %v", err)
					} else {
						if _, present := raw["active"]; present {
							t.Errorf("active unexpectedly present: %s", raw["active"])
						}
						encoded, err := json.Marshal(raw)
						if err != nil {
							t.Errorf("marshal request: %v", err)
						} else {
							var got ModelsNotificationPreferenceRequest
							if err := json.Unmarshal(encoded, &got); err != nil {
								t.Errorf("decode typed request: %v", err)
							} else if !reflect.DeepEqual(got.Components, components) || got.NotifiedOn == nil || !reflect.DeepEqual(*got.NotifiedOn, notifiedOn) || got.Scope == nil || got.Scope.Type == nil || *got.Scope.Type != ScopeTags || got.Scope.Values == nil || !reflect.DeepEqual(*got.Scope.Values, tags) || len(got.NotificationTargets) != 1 || got.NotificationTargets[0].Channel != NotificationChannelEmail || !reflect.DeepEqual(got.NotificationTargets[0].Config["recipients"], []interface{}{"ops@example.test", "oncall@example.test"}) {
								t.Errorf("request configuration = %+v", got)
							}
						}
					}
				}
				if tt.body != "" {
					w.Header().Set("Content-Type", "application/json")
				}
				w.WriteHeader(tt.status)
				if tt.body != "" {
					if _, err := w.Write([]byte(tt.body)); err != nil {
						t.Errorf("write response: %v", err)
					}
				}
			}))
			defer server.Close()
			client, err := NewClientWithResponses(server.URL)
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			ctx := context.Background()
			switch tt.name {
			case "create", "create bad request":
				response, err := client.PostV1NotificationPreferencesWithResponse(ctx, request)
				if err != nil || response == nil {
					t.Fatalf("create response = %+v, error = %v", response, err)
				}
				if response.StatusCode() != tt.status {
					t.Errorf("status = %d, want %d", response.StatusCode(), tt.status)
				}
				if tt.name == "create" {
					if response.JSON201 == nil || response.JSON201.Id != id {
						t.Errorf("created preference = %+v", response.JSON201)
					}
				} else {
					checkError(t, response.JSON400, "bad request", "invalid configuration")
				}
			case "list", "list forbidden", "malformed list":
				response, err := client.GetV1NotificationPreferencesWithResponse(ctx)
				if tt.name == "malformed list" {
					if err == nil || response != nil {
						t.Fatalf("malformed response = %+v, error = %v", response, err)
					}
					return
				}
				if err != nil || response == nil {
					t.Fatalf("list response = %+v, error = %v", response, err)
				}
				if response.StatusCode() != tt.status {
					t.Errorf("status = %d, want %d", response.StatusCode(), tt.status)
				}
				if tt.name == "list forbidden" {
					checkError(t, response.JSON403, "forbidden", "missing permission")
				} else {
					list := response.JSON200
					if list == nil || list.Preferences == nil || len(*list.Preferences) != 1 || list.ComponentPreferenceCounts == nil || !reflect.DeepEqual(*list.ComponentPreferenceCounts, map[string]int{"GPU": 1, "NVLink": 1}) || list.Total == nil || *list.Total != 1 {
						t.Fatalf("list = %+v", list)
					}
					checkDetail(t, &(*list.Preferences)[0])
				}
			case "detail", "detail not found":
				response, err := client.GetV1NotificationPreferenceByIDWithResponse(ctx, id)
				if err != nil || response == nil {
					t.Fatalf("detail response = %+v, error = %v", response, err)
				}
				if response.StatusCode() != tt.status {
					t.Errorf("status = %d, want %d", response.StatusCode(), tt.status)
				}
				if tt.name == "detail" {
					checkDetail(t, response.JSON200)
				} else {
					checkError(t, response.JSON404, "not found", "unknown preference")
				}
			case "update":
				response, err := client.PutV1NotificationPreferenceByIDWithResponse(ctx, id, request)
				if err != nil || response == nil || response.StatusCode() != http.StatusNoContent {
					t.Fatalf("update response = %+v, error = %v", response, err)
				}
			case "delete":
				response, err := client.DeleteV1NotificationPreferenceByIDWithResponse(ctx, id)
				if err != nil || response == nil || response.StatusCode() != http.StatusNoContent {
					t.Fatalf("delete response = %+v, error = %v", response, err)
				}
			}
		})
	}
}

func TestNotificationDeliveryLogResponseContract(t *testing.T) {
	component, channel, status := "GPU", "email", "failed"
	alertUUID := "alert-123"
	startTime, endTime := "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z"
	order := "asc"
	page, pageSize := 0, 20
	params := &GetV1NotificationDeliveryLogParams{
		Component: &component, Channel: &channel, Status: &status, AlertUUID: &alertUUID,
		StartTime: &startTime, EndTime: &endTime, Order: &order, Page: &page, PageSize: &pageSize,
	}
	query := url.Values{
		"component": {component}, "channel": {channel}, "status": {status}, "alertUUID": {alertUUID},
		"startTime": {startTime}, "endTime": {endTime}, "order": {order}, "page": {"0"}, "pageSize": {"20"},
	}
	tests := []struct {
		name, body string
		params     *GetV1NotificationDeliveryLogParams
		query      url.Values
		status     int
		malformed  bool
	}{
		{name: "filtered success", params: params, query: query, status: http.StatusOK, body: `{"deliveryLog":[{"alertUUID":"alert-123","nodeUUID":"node-456","component":"GPU","channel":"email","recipient":"ops@example.test","alertEventType":"Triggered","status":"failed","createdAt":"2026-01-01T12:00:00Z"}],"page":0,"pageSize":20,"total":21,"hasMore":true}`},
		{name: "forbidden", status: http.StatusForbidden, body: `{"error":"forbidden","details":"missing permission"}`},
		{name: "malformed", status: http.StatusOK, body: `{"deliveryLog":`, malformed: true},
		{name: "omitted parameters", status: http.StatusOK, body: `{"deliveryLog":[],"page":0,"pageSize":20,"total":0,"hasMore":false}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/notification_delivery_log" {
					t.Errorf("request = %s %s", r.Method, r.URL.Path)
				}
				if !reflect.DeepEqual(r.URL.Query(), tt.query) && !(len(r.URL.Query()) == 0 && len(tt.query) == 0) {
					t.Errorf("query = %v, want %v", r.URL.Query(), tt.query)
				}
				if len(tt.query) == 0 && (r.URL.RawQuery != "" || r.URL.ForceQuery) {
					t.Errorf("unexpected raw query: %q", r.URL.String())
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				if _, err := w.Write([]byte(tt.body)); err != nil {
					t.Errorf("write response: %v", err)
				}
			}))
			defer server.Close()
			client, err := NewClientWithResponses(server.URL)
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			response, err := client.GetV1NotificationDeliveryLogWithResponse(context.Background(), tt.params)
			if tt.malformed {
				if err == nil || response != nil {
					t.Errorf("expected decoding error and nil response, got response %+v, error %v", response, err)
				}
				return
			}
			if err != nil || response == nil {
				t.Fatalf("response = %+v, error = %v", response, err)
			}
			if response.StatusCode() != tt.status || string(response.Body) != tt.body {
				t.Errorf("status/body = %d %q, want %d %q", response.StatusCode(), response.Body, tt.status, tt.body)
			}
			if tt.status == http.StatusForbidden {
				if response.JSON200 != nil || response.JSON403 == nil || response.JSON403.Error == nil || *response.JSON403.Error != "forbidden" || response.JSON403.Details == nil || *response.JSON403.Details != "missing permission" {
					t.Errorf("forbidden response = %+v", response)
				}
				return
			}
			result := response.JSON200
			if result == nil || result.DeliveryLog == nil {
				t.Fatalf("missing delivery log: %+v", result)
			}
			if tt.name == "omitted parameters" {
				if len(*result.DeliveryLog) != 0 {
					t.Errorf("delivery log = %+v, want empty", *result.DeliveryLog)
				}
				checkComputeZonePage(t, result.Page, result.PageSize, result.Total, result.HasMore, 0, 20, 0, false)
				return
			}
			if len(*result.DeliveryLog) != 1 {
				t.Fatalf("delivery log = %+v", *result.DeliveryLog)
			}
			entry := (*result.DeliveryLog)[0]
			if entry.AlertUUID == nil || *entry.AlertUUID != alertUUID || entry.NodeUUID == nil || *entry.NodeUUID != "node-456" || entry.Component == nil || *entry.Component != component || entry.Channel == nil || *entry.Channel != channel || entry.Recipient == nil || *entry.Recipient != "ops@example.test" || entry.AlertEventType == nil || string(*entry.AlertEventType) != "Triggered" || entry.Status == nil || string(*entry.Status) != status || entry.CreatedAt == nil || *entry.CreatedAt != "2026-01-01T12:00:00Z" {
				t.Errorf("delivery log entry = %+v", entry)
			}
			checkComputeZonePage(t, result.Page, result.PageSize, result.Total, result.HasMore, 0, 20, 21, true)
		})
	}
}

func TestAgentStatusStatsResponseContract(t *testing.T) {
	oob, inband := GetV1StatsParamsDataSourceOob, GetV1StatsParamsDataSourceInband
	groupID := "group-1"
	groupParams := &GetV1StatsParams{Name: GetV1StatsParamsNameAgentStatus, DataSource: &oob, GroupBy: GetV1StatsParamsGroupByNodegroup, GroupId: &groupID, Window: "72h"}
	customerParams := &GetV1StatsParams{Name: GetV1StatsParamsNameAgentStatus, DataSource: &inband, GroupBy: GetV1StatsParamsGroupByCustomer, Window: "72h"}
	groupQuery := url.Values{"name": {"agentStatus"}, "dataSource": {"oob"}, "groupBy": {"nodegroup"}, "groupId": {groupID}, "window": {"72h"}}
	customerQuery := url.Values{"name": {"agentStatus"}, "dataSource": {"inband"}, "groupBy": {"customer"}, "window": {"72h"}}
	const successBody = `{"timestamps":[1719489600,1719576000,1719662400],"series":[{"name":"Online","values":[7,null,0]},{"name":"Offline","values":[0,null,2]}],"timeRange":{"start":"2024-06-27T12:00:00Z","end":"2024-06-29T12:00:00Z"}}`
	tests := []struct {
		name, body string
		params     *GetV1StatsParams
		query      url.Values
		status     int
		malformed  bool
	}{
		{"node group", successBody, groupParams, groupQuery, http.StatusOK, false},
		{"customer", successBody, customerParams, customerQuery, http.StatusOK, false},
		{"bad request", `{"error":"bad request","details":"invalid window"}`, groupParams, groupQuery, http.StatusBadRequest, false},
		{"forbidden", `{"error":"forbidden","details":"missing permission"}`, groupParams, groupQuery, http.StatusForbidden, false},
		{"malformed", `{"series":`, groupParams, groupQuery, http.StatusOK, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/stats" {
					t.Errorf("request = %s %s", r.Method, r.URL.Path)
				}
				if !reflect.DeepEqual(r.URL.Query(), tt.query) {
					t.Errorf("query = %v, want %v", r.URL.Query(), tt.query)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				if _, err := w.Write([]byte(tt.body)); err != nil {
					t.Errorf("write response: %v", err)
				}
			}))
			defer server.Close()
			client, err := NewClientWithResponses(server.URL)
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			response, err := client.GetV1StatsWithResponse(context.Background(), tt.params)
			if tt.malformed {
				if err == nil || response != nil {
					t.Fatalf("malformed response = %+v, error = %v", response, err)
				}
				return
			}
			if err != nil || response == nil {
				t.Fatalf("response = %+v, error = %v", response, err)
			}
			if response.StatusCode() != tt.status || string(response.Body) != tt.body {
				t.Errorf("status/body = %d %q, want %d %q", response.StatusCode(), response.Body, tt.status, tt.body)
			}
			if tt.status != http.StatusOK {
				var got *ModelsErrorResponse
				wantError, wantDetails := "bad request", "invalid window"
				if tt.status == http.StatusBadRequest {
					got = response.JSON400
					if response.JSON403 != nil {
						t.Errorf("unexpected JSON403: %+v", response.JSON403)
					}
				} else {
					got = response.JSON403
					wantError, wantDetails = "forbidden", "missing permission"
					if response.JSON400 != nil {
						t.Errorf("unexpected JSON400: %+v", response.JSON400)
					}
				}
				if response.JSON200 != nil || got == nil || got.Error == nil || *got.Error != wantError || got.Details == nil || *got.Details != wantDetails {
					t.Errorf("error response = %+v", response)
				}
				return
			}
			result := response.JSON200
			if result == nil || result.Timestamps == nil || !reflect.DeepEqual(*result.Timestamps, []int{1719489600, 1719576000, 1719662400}) {
				t.Fatalf("stats or timestamps = %+v", result)
			}
			if result.Series == nil || len(*result.Series) != 2 {
				t.Fatalf("series = %+v", result.Series)
			}
			seven, zero, two := 7, 0, 2
			wantNames := []string{"Online", "Offline"}
			wantValues := [][]*int{{&seven, nil, &zero}, {&zero, nil, &two}}
			for i, series := range *result.Series {
				if series.Name == nil || *series.Name != wantNames[i] || series.Values == nil || !reflect.DeepEqual(*series.Values, wantValues[i]) {
					t.Errorf("series[%d] = %+v, want %s %v", i, series, wantNames[i], wantValues[i])
				}
			}
			if result.TimeRange == nil || result.TimeRange.Start == nil || result.TimeRange.End == nil || result.TimeRange.Start.Format("2006-01-02T15:04:05Z07:00") != "2024-06-27T12:00:00Z" || result.TimeRange.End.Format("2006-01-02T15:04:05Z07:00") != "2024-06-29T12:00:00Z" {
				t.Errorf("time range = %+v", result.TimeRange)
			}
		})
	}
}

func TestPredictedHealthRankingsResponseContract(t *testing.T) {
	rankUnit := GetV1PredictedHealthRankingsParamsRankUnitNode
	nodeUUIDs := []string{"11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"}
	tags := []string{"zone:west", "env:production"}
	tagMatch := GetV1PredictedHealthRankingsParamsTagMatchAll
	statuses := []ModelsAgentStatus{"Online", "Offline"}
	order := WorstFirst
	page, pageSize := 0, 2
	params := &GetV1PredictedHealthRankingsParams{RankUnit: &rankUnit, NodeUUIDs: &nodeUUIDs, Tags: &tags, TagMatch: &tagMatch, AgentStatuses: &statuses, Order: &order, Page: &page, PageSize: &pageSize}
	query := url.Values{"rankUnit": {"node"}, "nodeUUIDs": nodeUUIDs, "tags": tags, "tagMatch": {"all"}, "agentStatuses": {"Online", "Offline"}, "order": {"worstFirst"}, "page": {"0"}, "pageSize": {"2"}}
	tests := []struct {
		name, body, wantError, wantDetails string
		status                             int
		malformed                          bool
	}{
		{"rankings", `{"rankUnit":"node","items":[{"entityId":"node-1","predictionStatus":"ranked","rank":1},{"entityId":"node-2","predictionStatus":"unranked","unrankedReason":"missing score"}],"total":2,"rankedCount":1,"unrankedCount":1,"allNodesRanked":false,"page":0,"pageSize":2,"hasMore":false}`, "", "", http.StatusOK, false},
		{"forbidden", `{"error":"forbidden","details":"missing permission"}`, "forbidden", "missing permission", http.StatusForbidden, false},
		{"beta disabled", `{"error":"not implemented","details":"beta feature disabled"}`, "not implemented", "beta feature disabled", http.StatusNotImplemented, false},
		{"malformed", `{"items":`, "", "", http.StatusOK, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/predicted_health_rankings" {
					t.Errorf("request = %s %s", r.Method, r.URL.Path)
				}
				if !reflect.DeepEqual(r.URL.Query(), query) {
					t.Errorf("query = %v, want %v", r.URL.Query(), query)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				if _, err := w.Write([]byte(tt.body)); err != nil {
					t.Errorf("write response: %v", err)
				}
			}))
			defer server.Close()
			client, err := NewClientWithResponses(server.URL)
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			response, err := client.GetV1PredictedHealthRankingsWithResponse(context.Background(), params)
			if tt.malformed {
				if err == nil || response != nil {
					t.Fatalf("malformed response = %+v, error = %v", response, err)
				}
				return
			}
			if err != nil || response == nil {
				t.Fatalf("response = %+v, error = %v", response, err)
			}
			if response.StatusCode() != tt.status || string(response.Body) != tt.body {
				t.Errorf("status/body = %d %q, want %d %q", response.StatusCode(), response.Body, tt.status, tt.body)
			}
			if response.JSON400 != nil || response.JSON401 != nil || response.JSON499 != nil || response.JSON500 != nil || response.JSON503 != nil {
				t.Errorf("unexpected typed response: %+v", response)
			}
			if tt.status != http.StatusOK {
				got := response.JSON403
				if tt.status == http.StatusForbidden {
					if response.JSON501 != nil {
						t.Errorf("unexpected JSON501: %+v", response.JSON501)
					}
				} else {
					got = response.JSON501
					if response.JSON403 != nil {
						t.Errorf("unexpected JSON403: %+v", response.JSON403)
					}
				}
				if response.JSON200 != nil || got == nil || got.Error == nil || *got.Error != tt.wantError || got.Details == nil || *got.Details != tt.wantDetails {
					t.Errorf("error response = %+v", response)
				}
				return
			}
			if response.JSON403 != nil || response.JSON501 != nil {
				t.Errorf("unexpected error response: %+v", response)
			}
			result := response.JSON200
			if result == nil || result.Items == nil || len(*result.Items) != 2 {
				t.Fatalf("rankings or items = %+v", result)
			}
			if result.RankUnit == nil || *result.RankUnit != ModelsPredictedHealthRankingsResponseRankUnitNode || result.Total == nil || *result.Total != 2 || result.RankedCount == nil || *result.RankedCount != 1 || result.UnrankedCount == nil || *result.UnrankedCount != 1 || result.AllNodesRanked == nil || *result.AllNodesRanked || result.Page == nil || *result.Page != 0 || result.PageSize == nil || *result.PageSize != 2 || result.HasMore == nil || *result.HasMore {
				t.Errorf("ranking metadata = %+v", result)
			}
			ranked, unranked := (*result.Items)[0], (*result.Items)[1]
			if ranked.EntityId == nil || *ranked.EntityId != "node-1" || ranked.PredictionStatus == nil || *ranked.PredictionStatus != Ranked || ranked.Rank == nil || *ranked.Rank != 1 || ranked.UnrankedReason != nil {
				t.Errorf("ranked item = %+v", ranked)
			}
			if unranked.EntityId == nil || *unranked.EntityId != "node-2" || unranked.PredictionStatus == nil || *unranked.PredictionStatus != Unranked || unranked.Rank != nil || unranked.UnrankedReason == nil || *unranked.UnrankedReason != "missing score" {
				t.Errorf("unranked item = %+v", unranked)
			}
		})
	}
}

func TestBatchUpdateTagsResponseContract(t *testing.T) {
	nodeUUIDs := []string{"11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"}
	add := []string{"zone:west", "env:production"}
	remove := []string{"legacy:old"}
	request := PatchV1TagsJSONRequestBody{NodeUUIDs: nodeUUIDs, Add: &add, Remove: &remove}
	tests := []struct {
		name, body, wantError, wantDetails string
		status                             int
		malformed                          bool
	}{
		{"no content", "", "", "", http.StatusNoContent, false},
		{"bad request", `{"error":"bad request","details":"invalid request"}`, "bad request", "invalid request", http.StatusBadRequest, false},
		{"forbidden", `{"error":"forbidden","details":"missing permission"}`, "forbidden", "missing permission", http.StatusForbidden, false},
		{"malformed error", `{"error":`, "", "", http.StatusBadRequest, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPatch || r.URL.Path != "/v1/tags" || r.URL.RawQuery != "" || r.URL.ForceQuery {
					t.Errorf("request = %s %s", r.Method, r.URL.String())
				}
				if got := r.Header.Get("Content-Type"); got != "application/json" {
					t.Errorf("content type = %q, want application/json", got)
				}
				var got struct {
					NodeUUIDs []string `json:"nodeUUIDs"`
					Add       []string `json:"add"`
					Remove    []string `json:"remove"`
				}
				if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
					t.Errorf("decode request: %v", err)
				} else if !reflect.DeepEqual(got.NodeUUIDs, nodeUUIDs) || !reflect.DeepEqual(got.Add, add) || !reflect.DeepEqual(got.Remove, remove) {
					t.Errorf("request body = %+v, want nodeUUIDs %v, add %v, remove %v", got, nodeUUIDs, add, remove)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				if tt.body != "" {
					if _, err := w.Write([]byte(tt.body)); err != nil {
						t.Errorf("write response: %v", err)
					}
				}
			}))
			defer server.Close()
			client, err := NewClientWithResponses(server.URL)
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			response, err := client.PatchV1TagsWithResponse(context.Background(), request)
			if tt.malformed {
				if err == nil || response != nil {
					t.Fatalf("malformed response = %+v, error = %v", response, err)
				}
				return
			}
			if err != nil || response == nil {
				t.Fatalf("response = %+v, error = %v", response, err)
			}
			if response.StatusCode() != tt.status || string(response.Body) != tt.body {
				t.Errorf("status/body = %d %q, want %d %q", response.StatusCode(), response.Body, tt.status, tt.body)
			}
			if response.JSON401 != nil || response.JSON499 != nil || response.JSON500 != nil {
				t.Errorf("unexpected typed response: %+v", response)
			}
			var got *ModelsErrorResponse
			switch tt.status {
			case http.StatusNoContent:
				if response.JSON400 != nil || response.JSON403 != nil {
					t.Errorf("unexpected error response: %+v", response)
				}
				return
			case http.StatusBadRequest:
				got = response.JSON400
				if response.JSON403 != nil {
					t.Errorf("unexpected JSON403: %+v", response.JSON403)
				}
			case http.StatusForbidden:
				got = response.JSON403
				if response.JSON400 != nil {
					t.Errorf("unexpected JSON400: %+v", response.JSON400)
				}
			}
			if got == nil || got.Error == nil || *got.Error != tt.wantError || got.Details == nil || *got.Details != tt.wantDetails {
				t.Errorf("error response = %+v", got)
			}
		})
	}
}

func TestV2CommandsResponseContract(t *testing.T) {
	tests := []struct {
		name, body, wantError, wantDetails string
		status                             int
		malformed                          bool
	}{
		{name: "catalog", status: http.StatusOK, body: `{"commands":[{"name":"inspect","link":"/commands/inspect","template":"inspect {node} {device}","placeholders":["node","device"],"flavors":[{"name":"detail","link":"/commands/inspect/detail","template":"inspect-detail {device} {node}","placeholders":["device","node"]}]}]}`},
		{name: "bad request", status: http.StatusBadRequest, body: `{"error":"bad request","details":"invalid catalog request"}`, wantError: "bad request", wantDetails: "invalid catalog request"},
		{name: "client closed", status: 499, body: `{"error":"client closed","details":"request cancelled"}`, wantError: "client closed", wantDetails: "request cancelled"},
		{name: "server error", status: http.StatusInternalServerError, body: `{"error":"server error","details":"catalog unavailable"}`, wantError: "server error", wantDetails: "catalog unavailable"},
		{name: "malformed", status: http.StatusOK, body: `{"commands":`, malformed: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v2/commands" || r.URL.RawQuery != "" || r.URL.ForceQuery {
					t.Errorf("request = %s %s", r.Method, r.URL.String())
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				if _, err := w.Write([]byte(tt.body)); err != nil {
					t.Errorf("write response: %v", err)
				}
			}))
			defer server.Close()
			client, err := NewClientWithResponses(server.URL)
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			response, err := client.GetV2CommandsWithResponse(context.Background())
			if tt.malformed {
				if err == nil || response != nil {
					t.Fatalf("malformed response = %+v, error = %v; want nil response and decoding error", response, err)
				}
				return
			}
			if err != nil || response == nil {
				t.Fatalf("response = %+v, error = %v", response, err)
			}
			if response.StatusCode() != tt.status || string(response.Body) != tt.body {
				t.Errorf("status/body = %d %q, want %d %q", response.StatusCode(), response.Body, tt.status, tt.body)
			}
			if tt.status == http.StatusOK {
				if response.JSON200 == nil || response.JSON200.Commands == nil || len(*response.JSON200.Commands) != 1 || response.JSON400 != nil || response.JSON499 != nil || response.JSON500 != nil {
					t.Fatalf("unexpected catalog response: %+v", response)
				}
				command := (*response.JSON200.Commands)[0]
				if command.Name == nil || *command.Name != "inspect" || command.Link == nil || *command.Link != "/commands/inspect" || command.Template == nil || *command.Template != "inspect {node} {device}" || command.Placeholders == nil || !reflect.DeepEqual(*command.Placeholders, []string{"node", "device"}) {
					t.Errorf("command = %+v", command)
				}
				if command.Flavors == nil || len(*command.Flavors) != 1 {
					t.Fatalf("flavors = %+v", command.Flavors)
				}
				flavor := (*command.Flavors)[0]
				if flavor.Name == nil || *flavor.Name != "detail" || flavor.Link == nil || *flavor.Link != "/commands/inspect/detail" || flavor.Template == nil || *flavor.Template != "inspect-detail {device} {node}" || flavor.Placeholders == nil || !reflect.DeepEqual(*flavor.Placeholders, []string{"device", "node"}) {
					t.Errorf("flavor = %+v", flavor)
				}
				return
			}
			if response.JSON200 != nil || (response.JSON400 != nil) != (tt.status == http.StatusBadRequest) || (response.JSON499 != nil) != (tt.status == 499) || (response.JSON500 != nil) != (tt.status == http.StatusInternalServerError) {
				t.Fatalf("unexpected response variants: %+v", response)
			}
			var decoded *ModelsErrorResponse
			switch tt.status {
			case http.StatusBadRequest:
				decoded = response.JSON400
			case 499:
				decoded = response.JSON499
			case http.StatusInternalServerError:
				decoded = response.JSON500
			}
			if decoded == nil || decoded.Error == nil || *decoded.Error != tt.wantError || decoded.Details == nil || *decoded.Details != tt.wantDetails {
				t.Errorf("decoded error = %+v, want %q / %q", decoded, tt.wantError, tt.wantDetails)
			}
		})
	}
}

func TestNodeRemovalRequestResponseContract(t *testing.T) {
	ids := []string{"11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"}
	batchSource, agentType := DeleteV1NodesParamsDataSourceOob, DeleteV1NodesParamsAgentTypeOob
	singleSource := DeleteV1NodesNodeUuidParamsDataSourceInband
	cases := []struct {
		name, body, wantError, wantDetails string
		status                             int
		malformed                          bool
	}{
		{"success", `{"message":"accepted","count":2}`, "", "", http.StatusOK, false},
		{"bad request", `{"error":"bad request","details":"invalid nodes"}`, "bad request", "invalid nodes", http.StatusBadRequest, false},
		{"forbidden", `{"error":"forbidden","details":"missing permission"}`, "forbidden", "missing permission", http.StatusForbidden, false},
		{"malformed", `{"message":`, "", "", http.StatusOK, true},
	}
	for _, batch := range []bool{true, false} {
		operation := "single"
		if batch {
			operation = "batch"
		}
		for _, tt := range cases {
			t.Run(operation+"/"+tt.name, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					path := "/v1/nodes/" + ids[0]
					query := url.Values{"dataSource": {"inband"}}
					if batch {
						path = "/v1/nodes"
						query = url.Values{"dataSource": {"oob"}, "agentType": {"oob"}}
					}
					if r.Method != http.MethodDelete || r.URL.Path != path || !reflect.DeepEqual(r.URL.Query(), query) {
						t.Errorf("request = %s %s, want DELETE %s query %v", r.Method, r.URL.String(), path, query)
					}
					if batch {
						if got := r.Header.Get("Content-Type"); got != "application/json" {
							t.Errorf("content type = %q", got)
						}
						var body ModelsDeleteNodesRequest
						if err := json.NewDecoder(r.Body).Decode(&body); err != nil || !reflect.DeepEqual(body.NodeUUIDs, ids) {
							t.Errorf("request body = %+v, error = %v, want nodeUUIDs %v", body, err, ids)
						}
					} else if body, err := io.ReadAll(r.Body); err != nil || len(body) != 0 {
						t.Errorf("single request body = %q, error = %v", body, err)
					}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(tt.status)
					if _, err := w.Write([]byte(tt.body)); err != nil {
						t.Errorf("write response: %v", err)
					}
				}))
				defer server.Close()
				client, err := NewClientWithResponses(server.URL)
				if err != nil {
					t.Fatalf("new client: %v", err)
				}
				var body []byte
				var success *map[string]interface{}
				var badRequest, unauthorized, forbidden, notFound, closed, serverError, unavailable *ModelsErrorResponse
				var status int
				var responsePresent bool
				if batch {
					response, requestErr := client.DeleteV1NodesWithResponse(context.Background(), &DeleteV1NodesParams{DataSource: &batchSource, AgentType: &agentType}, ModelsDeleteNodesRequest{NodeUUIDs: ids})
					err = requestErr
					responsePresent = response != nil
					if response != nil {
						body, status, success = response.Body, response.StatusCode(), response.JSON200
						badRequest, unauthorized, forbidden, notFound, closed, serverError, unavailable = response.JSON400, response.JSON401, response.JSON403, response.JSON404, response.JSON499, response.JSON500, response.JSON503
					}
				} else {
					response, requestErr := client.DeleteV1NodesNodeUuidWithResponse(context.Background(), ids[0], &DeleteV1NodesNodeUuidParams{DataSource: &singleSource})
					err = requestErr
					responsePresent = response != nil
					if response != nil {
						body, status, success = response.Body, response.StatusCode(), response.JSON200
						badRequest, unauthorized, forbidden, notFound, closed, serverError, unavailable = response.JSON400, response.JSON401, response.JSON403, response.JSON404, response.JSON499, response.JSON500, response.JSON503
					}
				}
				if tt.malformed {
					if err == nil || responsePresent {
						t.Fatalf("malformed response present = %v, error = %v", responsePresent, err)
					}
					return
				}
				if err != nil || !responsePresent {
					t.Fatalf("response present = %v, error = %v", responsePresent, err)
				}
				if status != tt.status || string(body) != tt.body {
					t.Errorf("status/body = %d %q, want %d %q", status, body, tt.status, tt.body)
				}
				if unauthorized != nil || notFound != nil || closed != nil || serverError != nil || unavailable != nil {
					t.Error("unexpected error response variant")
				}
				if tt.status == http.StatusOK {
					if success == nil || (*success)["message"] != "accepted" || (*success)["count"] != float64(2) || badRequest != nil || forbidden != nil {
						t.Errorf("success = %+v, bad request = %+v, forbidden = %+v", success, badRequest, forbidden)
					}
					return
				}
				if success != nil || (badRequest != nil) != (tt.status == http.StatusBadRequest) || (forbidden != nil) != (tt.status == http.StatusForbidden) {
					t.Fatalf("unexpected response variants: success = %+v, bad request = %+v, forbidden = %+v", success, badRequest, forbidden)
				}
				decoded := badRequest
				if tt.status == http.StatusForbidden {
					decoded = forbidden
				}
				if decoded == nil || decoded.Error == nil || *decoded.Error != tt.wantError || decoded.Details == nil || *decoded.Details != tt.wantDetails {
					t.Errorf("decoded error = %+v, want %q / %q", decoded, tt.wantError, tt.wantDetails)
				}
			})
		}
	}
}

type authStatusTestDoer func(*http.Request) (*http.Response, error)

func (d authStatusTestDoer) Do(req *http.Request) (*http.Response, error) {
	return d(req)
}

type authStatusFailingBody struct {
	err    error
	closed bool
}

func (b *authStatusFailingBody) Read(_ []byte) (int, error) { return 0, b.err }
func (b *authStatusFailingBody) Close() error {
	b.closed = true
	return nil
}

func TestAuthStatusRequestEditorAndFailureContract(t *testing.T) {
	t.Run("editor order and response", func(t *testing.T) {
		received := make(chan string, 1)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet || r.URL.Path != "/v1/auth/status" {
				t.Errorf("request = %s %s", r.Method, r.URL.Path)
			}
			received <- r.Header.Get("X-Editor")
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"authenticated":true}`)
		}))
		defer server.Close()

		var order []string
		client, err := NewClientWithResponses(server.URL, WithRequestEditorFn(func(_ context.Context, req *http.Request) error {
			order = append(order, "client")
			req.Header.Set("X-Editor", "client")
			return nil
		}))
		if err != nil {
			t.Fatalf("new client: %v", err)
		}
		response, err := client.GetV1AuthStatusWithResponse(context.Background(), func(_ context.Context, req *http.Request) error {
			order = append(order, "request")
			if got := req.Header.Get("X-Editor"); got != "client" {
				t.Errorf("request editor observed header %q, want client", got)
			}
			req.Header.Set("X-Editor", "client, request")
			return nil
		})
		if err != nil || response == nil {
			t.Fatalf("response = %+v, error = %v", response, err)
		}
		if !reflect.DeepEqual(order, []string{"client", "request"}) {
			t.Errorf("editor order = %v", order)
		}
		select {
		case got := <-received:
			if got != "client, request" {
				t.Errorf("server received header %q", got)
			}
		default:
			t.Error("server did not receive a request")
		}
		if response.StatusCode() != http.StatusOK || response.JSON200 == nil || response.JSON200.Authenticated == nil || !*response.JSON200.Authenticated {
			t.Errorf("decoded response = %+v", response)
		}
	})

	for _, tt := range []struct {
		name       string
		failClient bool
		wantOrder  []string
	}{
		{"client editor", true, []string{"client"}},
		{"request editor", false, []string{"client", "request"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			sentinel := errors.New("editor failure")
			var order []string
			requests := 0
			client, err := NewClientWithResponses("http://example.invalid", WithHTTPClient(authStatusTestDoer(func(*http.Request) (*http.Response, error) {
				requests++
				return nil, errors.New("unexpected request")
			})), WithRequestEditorFn(func(_ context.Context, _ *http.Request) error {
				order = append(order, "client")
				if tt.failClient {
					return sentinel
				}
				return nil
			}))
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			response, err := client.GetV1AuthStatusWithResponse(context.Background(),
				func(_ context.Context, _ *http.Request) error {
					order = append(order, "request")
					if !tt.failClient {
						return sentinel
					}
					return nil
				},
				func(_ context.Context, _ *http.Request) error {
					order = append(order, "later")
					return nil
				},
			)
			if response != nil || !errors.Is(err, sentinel) {
				t.Errorf("response = %+v, error = %v; want nil and sentinel", response, err)
			}
			if !reflect.DeepEqual(order, tt.wantOrder) || requests != 0 {
				t.Errorf("editor order = %v, requests = %d; want %v and zero", order, requests, tt.wantOrder)
			}
		})
	}

	t.Run("response read error closes body", func(t *testing.T) {
		sentinel := errors.New("response read failure")
		body := &authStatusFailingBody{err: sentinel}
		requests := 0
		client, err := NewClientWithResponses("http://example.invalid", WithHTTPClient(authStatusTestDoer(func(*http.Request) (*http.Response, error) {
			requests++
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: body}, nil
		})))
		if err != nil {
			t.Fatalf("new client: %v", err)
		}
		response, err := client.GetV1AuthStatusWithResponse(context.Background())
		if response != nil || !errors.Is(err, sentinel) || !body.closed || requests != 1 {
			t.Errorf("response = %+v, error = %v, body closed = %v, requests = %d", response, err, body.closed, requests)
		}
	})

	t.Run("invalid base URL", func(t *testing.T) {
		client, err := NewClientWithResponses("http://example.invalid", WithBaseURL("http://[::1"))
		if err == nil || client != nil {
			t.Errorf("client = %+v, error = %v; want nil client and error", client, err)
		}
	})
}

func TestNodesResponseUnionJSONPreservation(t *testing.T) {
	detail, basic := GetV1NodesParamsViewDetail, GetV1NodesParamsViewBasic
	inband, oob := GetV1NodesParamsDataSourceInband, GetV1NodesParamsDataSourceOob

	tests := []struct {
		name        string
		view        GetV1NodesParamsView
		source      GetV1NodesParamsDataSource
		variant     string
		invalidPage bool
	}{
		{name: "inband detail", view: detail, source: inband, variant: "inband"},
		{name: "oob detail", view: detail, source: oob, variant: "oob"},
		{name: "basic", view: basic, source: inband, variant: "basic"},
		{name: "incompatible page", view: detail, source: inband, variant: "inband", invalidPage: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := `{"nodes":[{"nodeUUID":"node-1","dataSource":"` + string(tt.source) + `","futureNode":{"sequence":9007199254740993,"enabled":false}}],"page":0,"pageSize":10,"total":1,"hasMore":false,"futureTop":{"values":[0,false,9007199254740993]}}`
			if tt.invalidPage {
				body = strings.Replace(body, `"page":0`, `"page":"zero"`, 1)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/nodes" {
					t.Errorf("request = %s %s", r.Method, r.URL.Path)
				}
				want := url.Values{"view": {string(tt.view)}, "dataSource": {string(tt.source)}}
				if !reflect.DeepEqual(r.URL.Query(), want) {
					t.Errorf("query = %v, want %v", r.URL.Query(), want)
				}
				w.Header().Set("Content-Type", "application/json")
				if _, err := io.WriteString(w, body); err != nil {
					t.Errorf("write response: %v", err)
				}
			}))
			defer server.Close()

			client, err := NewClientWithResponses(server.URL)
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			response, err := client.GetV1NodesWithResponse(context.Background(), &GetV1NodesParams{View: &tt.view, DataSource: &tt.source})
			if err != nil || response == nil || response.JSON200 == nil {
				t.Fatalf("response = %+v, error = %v", response, err)
			}

			var decodeErr error
			switch tt.variant {
			case "inband":
				result, err := response.JSON200.AsModelsInbandNodesResponse()
				decodeErr = err
				if err == nil {
					if result.Nodes == nil || len(*result.Nodes) != 1 || (*result.Nodes)[0].NodeUUID != "node-1" || (*result.Nodes)[0].DataSource != string(tt.source) {
						t.Errorf("inband nodes = %+v", result.Nodes)
					}
					checkComputeZonePage(t, result.Page, result.PageSize, result.Total, result.HasMore, 0, 10, 1, false)
				}
			case "oob":
				result, err := response.JSON200.AsModelsOobNodesResponse()
				decodeErr = err
				if err == nil {
					if result.Nodes == nil || len(*result.Nodes) != 1 || (*result.Nodes)[0].NodeUUID != "node-1" || (*result.Nodes)[0].DataSource != string(tt.source) {
						t.Errorf("oob nodes = %+v", result.Nodes)
					}
					checkComputeZonePage(t, result.Page, result.PageSize, result.Total, result.HasMore, 0, 10, 1, false)
				}
			case "basic":
				result, err := response.JSON200.AsModelsBasicNodesResponse()
				decodeErr = err
				if err == nil {
					if result.Nodes == nil || len(*result.Nodes) != 1 || (*result.Nodes)[0].NodeUUID != "node-1" {
						t.Errorf("basic nodes = %+v", result.Nodes)
					}
					checkComputeZonePage(t, result.Page, result.PageSize, result.Total, result.HasMore, 0, 10, 1, false)
				}
			}
			if (decodeErr != nil) != tt.invalidPage {
				t.Errorf("typed decoding error = %v, want error = %v", decodeErr, tt.invalidPage)
			}

			marshaled, err := json.Marshal(response.JSON200)
			if err != nil {
				t.Fatalf("marshal union: %v", err)
			}
			decodeTree := func(input string) interface{} {
				t.Helper()
				decoder := json.NewDecoder(strings.NewReader(input))
				decoder.UseNumber()
				var tree interface{}
				if err := decoder.Decode(&tree); err != nil {
					t.Fatalf("decode JSON tree: %v", err)
				}
				return tree
			}
			if got, want := decodeTree(string(marshaled)), decodeTree(body); !reflect.DeepEqual(got, want) {
				t.Errorf("marshaled union = %s, want %s", marshaled, body)
			}
		})
	}
}

func TestNodesResponseUnionFromAndMerge(t *testing.T) {
	page, total := 2, 17
	nodes := []ModelsSimpleNode{{NodeUUID: "node-initial"}}
	var constructed GetV1Nodes200JSONResponseBody
	if err := constructed.FromModelsBasicNodesResponse(ModelsBasicNodesResponse{Nodes: &nodes, Page: &page, Total: &total}); err != nil {
		t.Fatalf("construct union: %v", err)
	}
	decoded, err := constructed.AsModelsBasicNodesResponse()
	if err != nil {
		t.Fatalf("decode constructed union: %v", err)
	}
	if decoded.Nodes == nil || len(*decoded.Nodes) != 1 || (*decoded.Nodes)[0].NodeUUID != "node-initial" || decoded.Page == nil || *decoded.Page != 2 || decoded.Total == nil || *decoded.Total != 17 {
		t.Fatalf("constructed response = %+v", decoded)
	}

	const original = `{"nodes":[{"nodeUUID":"node-existing","futureNode":false}],"page":1,"total":9,"futureTop":{"sequence":9007199254740993}}`
	var union GetV1Nodes200JSONResponseBody
	if err := json.Unmarshal([]byte(original), &union); err != nil {
		t.Fatalf("unmarshal union: %v", err)
	}
	newPage := 4
	if err := union.MergeModelsBasicNodesResponse(ModelsBasicNodesResponse{Page: &newPage}); err != nil {
		t.Fatalf("merge page: %v", err)
	}
	decodeTree := func(input string) interface{} {
		t.Helper()
		decoder := json.NewDecoder(strings.NewReader(input))
		decoder.UseNumber()
		var tree interface{}
		if err := decoder.Decode(&tree); err != nil {
			t.Fatalf("decode JSON tree: %v", err)
		}
		return tree
	}
	merged, err := json.Marshal(union)
	if err != nil {
		t.Fatalf("marshal merged union: %v", err)
	}
	const wantMerged = `{"nodes":[{"nodeUUID":"node-existing","futureNode":false}],"page":4,"total":9,"futureTop":{"sequence":9007199254740993}}`
	if got, want := decodeTree(string(merged)), decodeTree(wantMerged); !reflect.DeepEqual(got, want) {
		t.Errorf("merged union = %s, want %s", merged, wantMerged)
	}

	replacementPage, replacementTotal := 6, 1
	replacementNodes := []ModelsSimpleNode{{NodeUUID: "node-replacement"}}
	if err := union.FromModelsBasicNodesResponse(ModelsBasicNodesResponse{Nodes: &replacementNodes, Page: &replacementPage, Total: &replacementTotal}); err != nil {
		t.Fatalf("replace union: %v", err)
	}
	replaced, err := json.Marshal(union)
	if err != nil {
		t.Fatalf("marshal replaced union: %v", err)
	}
	const wantReplacement = `{"nodes":[{"nodeUUID":"node-replacement"}],"page":6,"total":1}`
	if got, want := decodeTree(string(replaced)), decodeTree(wantReplacement); !reflect.DeepEqual(got, want) {
		t.Errorf("replaced union = %s, want %s", replaced, wantReplacement)
	}
	decoded, err = union.AsModelsBasicNodesResponse()
	if err != nil {
		t.Fatalf("decode replaced union: %v", err)
	}
	if decoded.Nodes == nil || len(*decoded.Nodes) != 1 || (*decoded.Nodes)[0].NodeUUID != "node-replacement" || decoded.Page == nil || *decoded.Page != 6 || decoded.Total == nil || *decoded.Total != 1 {
		t.Errorf("replaced response = %+v", decoded)
	}
}

func TestComputeZonesResponseUnionJSONPreservation(t *testing.T) {
	tests := []struct {
		name    string
		view    GetV1ComputezonesParamsView
		badPage bool
	}{
		{name: "basic", view: GetV1ComputezonesParamsViewBasic},
		{name: "detail", view: GetV1ComputezonesParamsViewDetail},
		{name: "basic incompatible page", view: GetV1ComputezonesParamsViewBasic, badPage: true},
		{name: "detail incompatible page", view: GetV1ComputezonesParamsViewDetail, badPage: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page := `2`
			if tt.badPage {
				page = `"two"`
			}
			body := `{"computezones":[{"id":"zone-1","name":"Alpha","extraNested":{"large":9007199254740993,"labels":["kept",true]}}],"page":` + page + `,"pageSize":50,"total":101,"hasMore":true,"extraTop":{"large":9007199254740993}}`
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/computezones" {
					t.Errorf("request = %s %s", r.Method, r.URL.Path)
				}
				if got, want := r.URL.Query(), (url.Values{"view": {string(tt.view)}}); !reflect.DeepEqual(got, want) {
					t.Errorf("query = %v, want %v", got, want)
				}
				w.Header().Set("Content-Type", "application/json")
				if _, err := io.WriteString(w, body); err != nil {
					t.Errorf("write response: %v", err)
				}
			}))
			defer server.Close()

			client, err := NewClientWithResponses(server.URL)
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			response, err := client.GetV1ComputezonesWithResponse(context.Background(), &GetV1ComputezonesParams{View: &tt.view})
			if err != nil {
				t.Fatalf("request: %v", err)
			}
			if response == nil || response.StatusCode() != http.StatusOK || response.JSON200 == nil {
				t.Fatalf("missing successful JSON response: %+v", response)
			}

			if tt.view == GetV1ComputezonesParamsViewBasic {
				result, err := response.JSON200.AsModelsListComputeZonesResponse()
				if tt.badPage {
					if err == nil {
						t.Error("basic accessor accepted a string page")
					}
				} else {
					if err != nil {
						t.Fatalf("decode basic: %v", err)
					}
					if result.Computezones == nil || len(*result.Computezones) != 1 {
						t.Fatalf("basic zones: %+v", result.Computezones)
					}
					zone := (*result.Computezones)[0]
					if zone.Id != "zone-1" || zone.Name == nil || *zone.Name != "Alpha" {
						t.Errorf("basic zone: %+v", zone)
					}
					checkComputeZonePage(t, result.Page, result.PageSize, result.Total, result.HasMore, 2, 50, 101, true)
				}
			} else {
				result, err := response.JSON200.AsModelsComputeZonesResponse()
				if tt.badPage {
					if err == nil {
						t.Error("detail accessor accepted a string page")
					}
				} else {
					if err != nil {
						t.Fatalf("decode detail: %v", err)
					}
					if result.Computezones == nil || len(*result.Computezones) != 1 {
						t.Fatalf("detail zones: %+v", result.Computezones)
					}
					zone := (*result.Computezones)[0]
					if zone.Id != "zone-1" || zone.Name == nil || *zone.Name != "Alpha" {
						t.Errorf("detail zone: %+v", zone)
					}
					checkComputeZonePage(t, result.Page, result.PageSize, result.Total, result.HasMore, 2, 50, 101, true)
				}
			}

			marshaled, err := json.Marshal(response.JSON200)
			if err != nil {
				t.Fatalf("marshal union: %v", err)
			}
			decode := func(input string) interface{} {
				t.Helper()
				decoder := json.NewDecoder(strings.NewReader(input))
				decoder.UseNumber()
				var value interface{}
				if err := decoder.Decode(&value); err != nil {
					t.Fatalf("decode JSON tree: %v", err)
				}
				return value
			}
			if got, want := decode(string(marshaled)), decode(body); !reflect.DeepEqual(got, want) {
				t.Errorf("marshaled union tree = %#v, want %#v", got, want)
			}
		})
	}
}

func TestNodeGroupsResponseUnionJSONPreservation(t *testing.T) {
	tests := []struct {
		name    string
		view    GetV1NodegroupsParamsView
		badPage bool
	}{
		{name: "basic", view: GetV1NodegroupsParamsViewBasic},
		{name: "detail", view: GetV1NodegroupsParamsViewDetail},
		{name: "basic incompatible page", view: GetV1NodegroupsParamsViewBasic, badPage: true},
		{name: "detail incompatible page", view: GetV1NodegroupsParamsViewDetail, badPage: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page := `2`
			if tt.badPage {
				page = `"two"`
			}
			body := `{"nodeGroups":[{"id":"group-1","name":"Alpha","futureGroup":{"enabled":false}}],"page":` + page + `,"pageSize":50,"total":101,"hasMore":true,"futureTop":{"sequence":9007199254740993}}`
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/nodegroups" {
					t.Errorf("request = %s %s", r.Method, r.URL.Path)
				}
				if got, want := r.URL.Query(), (url.Values{"view": {string(tt.view)}}); !reflect.DeepEqual(got, want) {
					t.Errorf("query = %v, want %v", got, want)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				if _, err := io.WriteString(w, body); err != nil {
					t.Errorf("write response: %v", err)
				}
			}))
			defer server.Close()

			client, err := NewClientWithResponses(server.URL)
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			response, err := client.GetV1NodegroupsWithResponse(context.Background(), &GetV1NodegroupsParams{View: &tt.view})
			if err != nil || response == nil || response.StatusCode() != http.StatusOK || response.JSON200 == nil {
				t.Fatalf("response = %+v, error = %v", response, err)
			}

			if tt.view == GetV1NodegroupsParamsViewBasic {
				result, err := response.JSON200.AsModelsListNodeGroupsResponse()
				if tt.badPage {
					if err == nil {
						t.Error("basic accessor accepted a string page")
					}
				} else {
					if err != nil {
						t.Fatalf("decode basic: %v", err)
					}
					if result.NodeGroups == nil || len(*result.NodeGroups) != 1 {
						t.Fatalf("basic groups: %+v", result.NodeGroups)
					}
					group := (*result.NodeGroups)[0]
					if group.Id != "group-1" || group.Name == nil || *group.Name != "Alpha" {
						t.Errorf("basic group: %+v", group)
					}
					checkComputeZonePage(t, result.Page, result.PageSize, result.Total, result.HasMore, 2, 50, 101, true)
				}
			} else {
				result, err := response.JSON200.AsModelsNodeGroupsResponse()
				if tt.badPage {
					if err == nil {
						t.Error("detail accessor accepted a string page")
					}
				} else {
					if err != nil {
						t.Fatalf("decode detail: %v", err)
					}
					if result.NodeGroups == nil || len(*result.NodeGroups) != 1 {
						t.Fatalf("detail groups: %+v", result.NodeGroups)
					}
					group := (*result.NodeGroups)[0]
					if group.Id != "group-1" || group.Name == nil || *group.Name != "Alpha" {
						t.Errorf("detail group: %+v", group)
					}
					checkComputeZonePage(t, result.Page, result.PageSize, result.Total, result.HasMore, 2, 50, 101, true)
				}
			}

			decodeTree := func(input string) interface{} {
				t.Helper()
				decoder := json.NewDecoder(strings.NewReader(input))
				decoder.UseNumber()
				var tree interface{}
				if err := decoder.Decode(&tree); err != nil {
					t.Fatalf("decode JSON tree: %v", err)
				}
				return tree
			}
			marshaled, err := json.Marshal(response.JSON200)
			if err != nil {
				t.Fatalf("marshal union: %v", err)
			}
			if got, want := decodeTree(string(marshaled)), decodeTree(body); !reflect.DeepEqual(got, want) {
				t.Errorf("marshaled union = %s, want %s", marshaled, body)
			}

			newPage := 4
			if tt.view == GetV1NodegroupsParamsViewBasic {
				err = response.JSON200.MergeModelsListNodeGroupsResponse(ModelsListNodeGroupsResponse{Page: &newPage})
			} else {
				err = response.JSON200.MergeModelsNodeGroupsResponse(ModelsNodeGroupsResponse{Page: &newPage})
			}
			if err != nil {
				t.Fatalf("merge page: %v", err)
			}
			merged, err := json.Marshal(response.JSON200)
			if err != nil {
				t.Fatalf("marshal merged union: %v", err)
			}
			wantMerged := strings.Replace(body, `"page":`+page, `"page":4`, 1)
			if got, want := decodeTree(string(merged)), decodeTree(wantMerged); !reflect.DeepEqual(got, want) {
				t.Errorf("merged union = %s, want %s", merged, wantMerged)
			}
		})
	}
}

func TestSecurityVulnerabilityOptionsResponseContract(t *testing.T) {
	tests := []struct {
		name, body string
		status     int
		wantError  bool
		check      func(*testing.T, *GetV1ReportsSecurityVulnerabilityOptionsResponse)
	}{
		{
			name: "options", status: http.StatusOK,
			body: `{"filters":{"computeZoneIds":[{"id":"zone-1","name":"East"}],"nodeGroupIds":[{"id":"group-1","name":"Training"}],"componentTypes":[{"id":"gpuDriver","name":"GPU Driver"},{"id":"vbios","name":"VBIOS"}],"gpuTypes":["H100","A100"],"severities":["CRITICAL","HIGH"]},"sorting":{"fields":["vulnerability","affectedNodes"],"orders":["asc","desc"],"defaults":{"field":"affectedNodes","order":"desc"}}}`,
			check: func(t *testing.T, response *GetV1ReportsSecurityVulnerabilityOptionsResponse) {
				if response.JSON200 == nil {
					t.Fatal("missing JSON200")
				}
				options := response.JSON200
				want := ModelsSecurityVulnerabilityFilterOptions{
					ComputeZoneIds: []ModelsIDNameOption{{Id: "zone-1", Name: "East"}},
					NodeGroupIds:   []ModelsIDNameOption{{Id: "group-1", Name: "Training"}},
					ComponentTypes: []ModelsIDNameOption{{Id: "gpuDriver", Name: "GPU Driver"}, {Id: "vbios", Name: "VBIOS"}},
					GpuTypes:       []string{"H100", "A100"},
					Severities:     []ModelsSecurityVulnerabilitySeverity{SecurityVulnerabilitySeverityCritical, SecurityVulnerabilitySeverityHigh},
				}
				if !reflect.DeepEqual(options.Filters, want) {
					t.Errorf("filters = %+v, want %+v", options.Filters, want)
				}
				s := options.Sorting
				if s.Fields == nil || !reflect.DeepEqual(*s.Fields, []string{"vulnerability", "affectedNodes"}) || s.Orders == nil || !reflect.DeepEqual(*s.Orders, []string{"asc", "desc"}) || s.Defaults == nil || s.Defaults.Field == nil || *s.Defaults.Field != "affectedNodes" || s.Defaults.Order == nil || *s.Defaults.Order != "desc" {
					t.Errorf("sorting = %+v", s)
				}
			},
		},
		{
			name: "unprocessable", status: http.StatusUnprocessableEntity, body: `{"error":"invalid request","details":"invalid filters"}`,
			check: func(t *testing.T, response *GetV1ReportsSecurityVulnerabilityOptionsResponse) {
				if response.JSON200 != nil || response.JSON422 == nil {
					t.Fatalf("unexpected response: %+v", response)
				}
				if response.JSON422.Error == nil || *response.JSON422.Error != "invalid request" || response.JSON422.Details == nil || *response.JSON422.Details != "invalid filters" {
					t.Errorf("error = %+v", response.JSON422)
				}
			},
		},
		{name: "malformed json", status: http.StatusOK, body: `{"filters":`, wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/reports/security/vulnerabilities/options" || r.URL.RawQuery != "" {
					t.Errorf("request = %s %s", r.Method, r.URL.RequestURI())
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				if _, err := w.Write([]byte(tt.body)); err != nil {
					t.Errorf("write response: %v", err)
				}
			}))
			defer server.Close()
			client, err := NewClientWithResponses(server.URL)
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			response, err := client.GetV1ReportsSecurityVulnerabilityOptionsWithResponse(context.Background())
			if tt.wantError {
				if err == nil || response != nil {
					t.Fatalf("expected decoding error and nil response, got response=%+v err=%v", response, err)
				}
				return
			}
			if err != nil || response == nil {
				t.Fatalf("request: response=%+v err=%v", response, err)
			}
			if response.StatusCode() != tt.status || string(response.Body) != tt.body {
				t.Errorf("status/body = %d %q, want %d %q", response.StatusCode(), response.Body, tt.status, tt.body)
			}
			tt.check(t, response)
		})
	}
}

func TestSecurityVulnerabilityNodesResponseContract(t *testing.T) {
	zones, groups := []string{"zone-1", "zone-2"}, []string{"group-1", "group-2"}
	gpus := []string{"H100", "A100"}
	severities := []ModelsSecurityVulnerabilitySeverity{SecurityVulnerabilitySeverityCritical, SecurityVulnerabilitySeverityHigh}
	components := []ModelsVulnerabilityComponent{VulnerabilityComponentGPUDriver, VulnerabilityComponentVBIOS}
	search := "CVE-2026"
	format := GetV1ReportsSecurityVulnerabilityNodesParamsFormatCsv
	params := &GetV1ReportsSecurityVulnerabilityNodesParams{ComputeZoneIds: &zones, NodeGroupIds: &groups, GpuTypes: &gpus, Severities: &severities, ComponentTypes: &components, VulnerabilitySearch: &search, Format: &format}
	query := url.Values{"computeZoneIds": {"zone-1", "zone-2"}, "nodeGroupIds": {"group-1", "group-2"}, "gpuTypes": {"H100", "A100"}, "severities": {"CRITICAL", "HIGH"}, "componentTypes": {"gpuDriver", "vbios"}, "vulnerabilitySearch": {"CVE-2026"}, "format": {"csv"}}
	const disposition = `attachment; filename="nodes.csv"`
	tests := []struct {
		name, contentType, body string
		status                  int
		headers                 http.Header
		check                   func(*testing.T, *GetV1ReportsSecurityVulnerabilityNodesResponse)
	}{
		{
			name: "csv", contentType: "text/csv", status: http.StatusOK,
			body: "node,description\r\nnode-1,\"first line\nsecond line\"\r\n", headers: http.Header{"Content-Disposition": {disposition}, "X-Total-Count": {"2"}},
			check: func(t *testing.T, response *GetV1ReportsSecurityVulnerabilityNodesResponse) {
				if response.JSON404 != nil || response.Headers200 == nil || response.Headers200.ContentDisposition == nil || *response.Headers200.ContentDisposition != disposition || response.Headers200.XTotalCount == nil || *response.Headers200.XTotalCount != 2 {
					t.Errorf("csv headers = %+v, JSON404 = %+v", response.Headers200, response.JSON404)
				}
			},
		},
		{
			name: "not found", contentType: "application/json", status: http.StatusNotFound, body: `{"error":"not found","details":"unknown vulnerability"}`,
			check: func(t *testing.T, response *GetV1ReportsSecurityVulnerabilityNodesResponse) {
				if response.Headers200 != nil || response.JSON404 == nil {
					t.Fatalf("unexpected response: %+v", response)
				}
				if response.JSON404.Error == nil || *response.JSON404.Error != "not found" || response.JSON404.Details == nil || *response.JSON404.Details != "unknown vulnerability" {
					t.Errorf("error = %+v", response.JSON404)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/reports/security/vulnerabilities/CVE-2026-1234/nodes" {
					t.Errorf("request = %s %s", r.Method, r.URL.Path)
				}
				if !reflect.DeepEqual(r.URL.Query(), query) {
					t.Errorf("query = %v, want %v", r.URL.Query(), query)
				}
				w.Header().Set("Content-Type", tt.contentType)
				for key, values := range tt.headers {
					for _, value := range values {
						w.Header().Add(key, value)
					}
				}
				w.WriteHeader(tt.status)
				if _, err := w.Write([]byte(tt.body)); err != nil {
					t.Errorf("write response: %v", err)
				}
			}))
			defer server.Close()
			client, err := NewClientWithResponses(server.URL)
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			response, err := client.GetV1ReportsSecurityVulnerabilityNodesWithResponse(context.Background(), "CVE-2026-1234", params)
			if err != nil || response == nil {
				t.Fatalf("request: response=%+v err=%v", response, err)
			}
			if response.StatusCode() != tt.status || string(response.Body) != tt.body {
				t.Errorf("status/body = %d %q, want %d %q", response.StatusCode(), response.Body, tt.status, tt.body)
			}
			tt.check(t, response)
		})
	}
}
