// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package fleetapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"
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

func TestRound2ErrorGraphInvalidTimestampMerge(t *testing.T) {
	start := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC)
	errorName := "xid_154"
	values := "[[1716153600,5]]"
	result := []ModelsErrorTimeSeriesByError{{
		Label:  &ModelsErrorLabelByError{Error: &errorName},
		Values: &values,
	}}
	original := ModelsErrorReportGraphByErrorResponse{
		Result:    &result,
		TimeRange: &ModelsTimeRange{Start: &start, End: &end},
	}
	var union GetV1ReportsError200JSONResponseBody
	if err := union.FromModelsErrorReportGraphByErrorResponse(original); err != nil {
		t.Fatalf("From: %v", err)
	}

	invalidEnd := time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)
	invalid := ModelsErrorReportGraphByErrorResponse{TimeRange: &ModelsTimeRange{End: &invalidEnd}}
	err := union.MergeModelsErrorReportGraphByErrorResponse(invalid)
	var marshalerError *json.MarshalerError
	if !errors.As(err, &marshalerError) {
		t.Errorf("Merge error = %v, want *json.MarshalerError", err)
	}
	got, err := union.AsModelsErrorReportGraphByErrorResponse()
	if err != nil {
		t.Fatalf("As: %v", err)
	}
	if !reflect.DeepEqual(got, original) {
		t.Errorf("model after failed merge = %+v, want %+v", got, original)
	}
}

func testRound2ResourceUnionNonfiniteMerge[M any, U any](t *testing.T, original, invalid M,
	from func(*U, M) error, merge func(*U, M) error, as func(*U) (M, error)) {
	t.Helper()
	var union U
	if err := from(&union, original); err != nil {
		t.Fatalf("From: %v", err)
	}
	err := merge(&union, invalid)
	var unsupported *json.UnsupportedValueError
	if !errors.As(err, &unsupported) {
		t.Errorf("Merge error = %v, want *json.UnsupportedValueError", err)
	}
	got, err := as(&union)
	if err != nil {
		t.Fatalf("As: %v", err)
	}
	if !reflect.DeepEqual(got, original) {
		t.Errorf("model after failed merge = %+v, want %+v", got, original)
	}
}

func TestRound2ResourceUnionNonfiniteMergeContracts(t *testing.T) {
	finite := float32(42.5)
	nonfinite := float32(math.NaN())
	t.Run("compute_zones", func(t *testing.T) {
		originalZones := []ModelsComputeZoneOverview{{Id: "zone-1", HealthPercentage: &finite}}
		invalidZones := []ModelsComputeZoneOverview{{Id: "zone-1", HealthPercentage: &nonfinite}}
		testRound2ResourceUnionNonfiniteMerge(t,
			ModelsComputeZonesResponse{Computezones: &originalZones}, ModelsComputeZonesResponse{Computezones: &invalidZones},
			(*GetV1Computezones200JSONResponseBody).FromModelsComputeZonesResponse,
			(*GetV1Computezones200JSONResponseBody).MergeModelsComputeZonesResponse,
			(*GetV1Computezones200JSONResponseBody).AsModelsComputeZonesResponse)
	})
	t.Run("node_groups", func(t *testing.T) {
		originalGroups := []ModelsNodeGroupOverview{{Id: "group-1", HealthPercentage: &finite}}
		invalidGroups := []ModelsNodeGroupOverview{{Id: "group-1", HealthPercentage: &nonfinite}}
		testRound2ResourceUnionNonfiniteMerge(t,
			ModelsNodeGroupsResponse{NodeGroups: &originalGroups}, ModelsNodeGroupsResponse{NodeGroups: &invalidGroups},
			(*GetV1Nodegroups200JSONResponseBody).FromModelsNodeGroupsResponse,
			(*GetV1Nodegroups200JSONResponseBody).MergeModelsNodeGroupsResponse,
			(*GetV1Nodegroups200JSONResponseBody).AsModelsNodeGroupsResponse)
	})
	t.Run("inband_node", func(t *testing.T) {
		original := ModelsInbandNodeDetailsResponse{NodeUUID: "node-1", DataSource: "inband", Location: &ModelsLocation{Latitude: &finite}}
		invalid := ModelsInbandNodeDetailsResponse{NodeUUID: "node-1", DataSource: "inband", Location: &ModelsLocation{Latitude: &nonfinite}}
		testRound2ResourceUnionNonfiniteMerge(t, original, invalid,
			(*GetV1NodesNodeUuid200JSONResponseBody).FromModelsInbandNodeDetailsResponse,
			(*GetV1NodesNodeUuid200JSONResponseBody).MergeModelsInbandNodeDetailsResponse,
			(*GetV1NodesNodeUuid200JSONResponseBody).AsModelsInbandNodeDetailsResponse)
	})
	t.Run("oob_node", func(t *testing.T) {
		original := ModelsOobNodeDetailsResponse{NodeUUID: "node-2", DataSource: "oob", Location: &ModelsLocation{Latitude: &finite}}
		invalid := ModelsOobNodeDetailsResponse{NodeUUID: "node-2", DataSource: "oob", Location: &ModelsLocation{Latitude: &nonfinite}}
		testRound2ResourceUnionNonfiniteMerge(t, original, invalid,
			(*GetV1NodesNodeUuid200JSONResponseBody).FromModelsOobNodeDetailsResponse,
			(*GetV1NodesNodeUuid200JSONResponseBody).MergeModelsOobNodeDetailsResponse,
			(*GetV1NodesNodeUuid200JSONResponseBody).AsModelsOobNodeDetailsResponse)
	})
}

func TestRound2ListUnionNonfiniteMergeContracts(t *testing.T) {
	finite := float32(39.0458)
	nonfinite := float32(math.NaN())
	t.Run("compute_zones", func(t *testing.T) {
		originalZones := []ModelsSimpleComputeZone{{Id: "zone-1", Location: &ModelsLocation{Latitude: &finite}}}
		invalidZones := []ModelsSimpleComputeZone{{Id: "zone-1", Location: &ModelsLocation{Latitude: &nonfinite}}}
		testRound2ResourceUnionNonfiniteMerge(t,
			ModelsListComputeZonesResponse{Computezones: &originalZones}, ModelsListComputeZonesResponse{Computezones: &invalidZones},
			(*GetV1Computezones200JSONResponseBody).FromModelsListComputeZonesResponse,
			(*GetV1Computezones200JSONResponseBody).MergeModelsListComputeZonesResponse,
			(*GetV1Computezones200JSONResponseBody).AsModelsListComputeZonesResponse)
	})
	t.Run("inband_nodes", func(t *testing.T) {
		originalEar := map[string]interface{}{"value": float64(39.0458)}
		invalidEar := map[string]interface{}{"value": math.NaN()}
		originalNodes := []ModelsInbandNode{{NodeUUID: "node-1", DataSource: "inband", VerificationCheckExtraInfo: &ModelsVerificationCheckExtraInfo{Ear: &originalEar}}}
		invalidNodes := []ModelsInbandNode{{NodeUUID: "node-1", DataSource: "inband", VerificationCheckExtraInfo: &ModelsVerificationCheckExtraInfo{Ear: &invalidEar}}}
		testRound2ResourceUnionNonfiniteMerge(t,
			ModelsInbandNodesResponse{Nodes: &originalNodes}, ModelsInbandNodesResponse{Nodes: &invalidNodes},
			(*GetV1Nodes200JSONResponseBody).FromModelsInbandNodesResponse,
			(*GetV1Nodes200JSONResponseBody).MergeModelsInbandNodesResponse,
			(*GetV1Nodes200JSONResponseBody).AsModelsInbandNodesResponse)
	})
	t.Run("oob_nodes", func(t *testing.T) {
		originalEar := map[string]interface{}{"value": float64(39.0458)}
		invalidEar := map[string]interface{}{"value": math.NaN()}
		originalNodes := []ModelsOobNode{{NodeUUID: "node-2", DataSource: "oob", VerificationCheckExtraInfo: &ModelsVerificationCheckExtraInfo{Ear: &originalEar}}}
		invalidNodes := []ModelsOobNode{{NodeUUID: "node-2", DataSource: "oob", VerificationCheckExtraInfo: &ModelsVerificationCheckExtraInfo{Ear: &invalidEar}}}
		testRound2ResourceUnionNonfiniteMerge(t,
			ModelsOobNodesResponse{Nodes: &originalNodes}, ModelsOobNodesResponse{Nodes: &invalidNodes},
			(*GetV1Nodes200JSONResponseBody).FromModelsOobNodesResponse,
			(*GetV1Nodes200JSONResponseBody).MergeModelsOobNodesResponse,
			(*GetV1Nodes200JSONResponseBody).AsModelsOobNodesResponse)
	})
}

func TestRound2InventoryUnionNonfiniteMergeContracts(t *testing.T) {
	finite := float32(39.0458)
	nonfinite := float32(math.NaN())
	t.Run("inband", func(t *testing.T) {
		originalNodes := []ModelsInbandInventoryNode{{Location: &ModelsLocation{Latitude: &finite}}}
		invalidNodes := []ModelsInbandInventoryNode{{Location: &ModelsLocation{Latitude: &nonfinite}}}
		testRound2ResourceUnionNonfiniteMerge(t,
			ModelsInbandInventoryReportResponse{Nodes: &originalNodes}, ModelsInbandInventoryReportResponse{Nodes: &invalidNodes},
			(*GetV1ReportsInventory200JSONResponseBody).FromModelsInbandInventoryReportResponse,
			(*GetV1ReportsInventory200JSONResponseBody).MergeModelsInbandInventoryReportResponse,
			(*GetV1ReportsInventory200JSONResponseBody).AsModelsInbandInventoryReportResponse)
	})
	t.Run("oob", func(t *testing.T) {
		originalNodes := []ModelsOobInventoryNode{{Location: &ModelsLocation{Latitude: &finite}}}
		invalidNodes := []ModelsOobInventoryNode{{Location: &ModelsLocation{Latitude: &nonfinite}}}
		testRound2ResourceUnionNonfiniteMerge(t,
			ModelsOobInventoryReportResponse{Nodes: &originalNodes}, ModelsOobInventoryReportResponse{Nodes: &invalidNodes},
			(*GetV1ReportsInventory200JSONResponseBody).FromModelsOobInventoryReportResponse,
			(*GetV1ReportsInventory200JSONResponseBody).MergeModelsOobInventoryReportResponse,
			(*GetV1ReportsInventory200JSONResponseBody).AsModelsOobInventoryReportResponse)
	})
}

type round2Enum interface {
	~string
	Valid() bool
}

type round2EnumValue[T round2Enum] struct {
	constant T
	literal  string
}

func testRound2EnumContract[T round2Enum](t *testing.T, values []round2EnumValue[T], wrongCase string) {
	t.Helper()
	for _, value := range values {
		t.Run(value.literal, func(t *testing.T) {
			if got := string(value.constant); got != value.literal {
				t.Errorf("constant = %q, want schema value %q", got, value.literal)
			}
			if !value.constant.Valid() {
				t.Errorf("constant %q is not valid", value.constant)
			}
			if literal := T(value.literal); !literal.Valid() {
				t.Errorf("schema literal %q is not valid when converted to enum", value.literal)
			}
		})
	}
	for _, invalid := range []string{"", "definitely_unknown_value", wrongCase} {
		if T(invalid).Valid() {
			t.Errorf("invalid value %q was accepted", invalid)
		}
	}
}

func TestRound2ModelEnumValidContracts(t *testing.T) {
	// Values and constant names are specified by the models schemas in api/openapi/openapi.yaml.
	t.Run("ActionPersona", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[ModelsActionPersona]{
			{ActionPersonaTenant, "tenant"}, {ActionPersonaDCAdmin, "dc_admin"},
		}, "Tenant")
	})
	t.Run("ActionType", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[ModelsActionType]{
			{ActionTypeImmediate, "immediate"}, {ActionTypeInvestigatory, "investigatory"},
		}, "Immediate")
	})
	t.Run("AgentStatus", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[ModelsAgentStatus]{
			{AgentStatusOnline, "Online"}, {AgentStatusOffline, "Offline"},
		}, "online")
	})
	t.Run("Aggregation", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[ModelsAggregation]{
			{AggregationAverage, "average"}, {AggregationSum, "sum"}, {AggregationMin, "min"},
			{AggregationMax, "max"}, {AggregationNone, "none"},
		}, "Average")
	})
	t.Run("AlertAgentStatus", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[ModelsAlertAgentStatus]{
			{AlertAgentStatusOnline, "online"}, {AlertAgentStatusOffline, "offline"},
			{AlertAgentStatusNotEnrolled, "not_enrolled"},
		}, "Online")
	})
	t.Run("AlertEventType", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[ModelsAlertEventType]{
			{AlertEventDetected, "detected"}, {AlertEventTriggered, "triggered"},
			{AlertEventResolved, "resolved"}, {AlertEventRetriggered, "retriggered"},
			{AlertEventSeverityChanged, "severity_changed"}, {AlertEventMetadataUpdated, "metadata_updated"},
			{AlertEventNotifySent, "notify_sent"}, {AlertEventNotifyFailed, "notify_failed"},
			{AlertEventMuted, "muted"}, {AlertEventUnmuted, "unmuted"},
			{AlertEventBurstAnalyzed, "burst_analyzed"},
		}, "Detected")
	})
	t.Run("AlertSeverity", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[ModelsAlertSeverity]{
			{AlertSeverityCritical, "Critical"}, {AlertSeverityWarning, "Warning"},
		}, "critical")
	})
	t.Run("AlertState", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[ModelsAlertState]{
			{AlertStateDetected, "Detected"}, {AlertStateTriggered, "Triggered"}, {AlertStateResolved, "Resolved"},
		}, "detected")
	})
	t.Run("Audience", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[ModelsAudience]{
			{AudienceDefault, "default"}, {AudienceExternalCloudProvider, "external_cloud_provider"},
			{AudienceForbidden, "forbidden"},
		}, "Default")
	})
	t.Run("DeliveryStatus", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[ModelsDeliveryStatus]{
			{DeliveryStatusSent, "sent"}, {DeliveryStatusFailed, "failed"},
		}, "Sent")
	})
	t.Run("HealthState", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[ModelsHealthState]{
			{HealthStateHealthy, "Healthy"}, {HealthStateDegraded, "Degraded"},
			{HealthStateUnhealthy, "Unhealthy"}, {HealthStateUnknown, "Unknown"},
			{HealthStateUnsupported, "Unsupported"}, {HealthStatePending, "Pending"},
		}, "healthy")
	})
	t.Run("NotificationChannel", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[ModelsNotificationChannel]{
			{NotificationChannelEmail, "email"}, {NotificationChannelSlack, "slack"},
			{NotificationChannelWebhook, "webhook"},
		}, "Email")
	})
	t.Run("NotificationScopeType", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[ModelsNotificationScopeType]{
			{ScopeAll, "all"}, {ScopeComputeZones, "compute_zones"}, {ScopeNodeGroups, "node_groups"},
			{ScopeTags, "tags"}, {ScopeNodes, "nodes"},
		}, "All")
	})
}

func TestRound2QueryEnumValidContracts(t *testing.T) {
	// Literals come from the operation query schemas in api/openapi/openapi.yaml.
	t.Run("inventory_sort_by", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1ReportsInventoryParamsSortBy]{
			{GetV1ReportsInventoryParamsSortByHostname, "hostname"},
			{GetV1ReportsInventoryParamsSortByNodeUUID, "nodeUUID"},
			{GetV1ReportsInventoryParamsSortByNodeGroup, "nodeGroup"},
			{GetV1ReportsInventoryParamsSortByComputeZone, "computeZone"},
			{GetV1ReportsInventoryParamsSortByGpuType, "gpuType"},
			{GetV1ReportsInventoryParamsSortByGpuCount, "gpuCount"},
			{GetV1ReportsInventoryParamsSortByPublicIP, "publicIP"},
			{GetV1ReportsInventoryParamsSortByPrivateIP, "privateIP"},
			{GetV1ReportsInventoryParamsSortByNodeName, "nodeName"},
			{GetV1ReportsInventoryParamsSortByBmcMac, "bmcMac"},
			{GetV1ReportsInventoryParamsSortByBmcIP, "bmcIP"},
			{GetV1ReportsInventoryParamsSortByVerificationCheck, "verificationCheck"},
			{GetV1ReportsInventoryParamsSortByLocation, "location"},
			{GetV1ReportsInventoryParamsSortByIntegrityCheck, "integrityCheck"},
			{GetV1ReportsInventoryParamsSortByGeoLocation, "geoLocation"},
		}, "Hostname")
	})
	t.Run("security_vulnerabilities_sort_by", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1ReportsSecurityVulnerabilitiesParamsSortBy]{
			{GetV1ReportsSecurityVulnerabilitiesParamsSortByVulnerability, "vulnerability"},
			{GetV1ReportsSecurityVulnerabilitiesParamsSortBySeverity, "severity"},
			{GetV1ReportsSecurityVulnerabilitiesParamsSortByAffectedNodes, "affectedNodes"},
			{GetV1ReportsSecurityVulnerabilitiesParamsSortByComponent, "component"},
		}, "Vulnerability")
	})
	t.Run("stats_group_by", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1StatsParamsGroupBy]{
			{GetV1StatsParamsGroupByCustomer, "customer"},
			{GetV1StatsParamsGroupByNodegroup, "nodegroup"},
			{GetV1StatsParamsGroupByComputezone, "computezone"},
		}, "Customer")
	})
	t.Run("xid_bursts_time_mode", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1XIDBurstsParamsTimeMode]{
			{GetV1XIDBurstsParamsTimeModeAbsolute, "absolute"},
			{GetV1XIDBurstsParamsTimeModeRelative, "relative"},
		}, "Absolute")
	})
	t.Run("xid_burst_options_time_mode", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1XIDBurstOptionsParamsTimeMode]{
			{GetV1XIDBurstOptionsParamsTimeModeAbsolute, "absolute"},
			{GetV1XIDBurstOptionsParamsTimeModeRelative, "relative"},
		}, "Absolute")
	})
	t.Run("xid_burst_options_data_source", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1XIDBurstOptionsParamsDataSource]{
			{GetV1XIDBurstOptionsParamsDataSourceInband, "inband"},
			{GetV1XIDBurstOptionsParamsDataSourceOob, "oob"},
			{GetV1XIDBurstOptionsParamsDataSourceAll, "all"},
		}, "Inband")
	})
	t.Run("v2_metrics_group_by", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV2MetricsParamsGroupBy]{
			{GetV2MetricsParamsGroupByCustomer, "customer"},
			{GetV2MetricsParamsGroupByNodegroup, "nodegroup"},
			{GetV2MetricsParamsGroupByComputezone, "computezone"},
			{GetV2MetricsParamsGroupByNode, "node"},
		}, "Customer")
	})
	t.Run("v2_metrics_time_mode", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV2MetricsParamsTimeMode]{
			{GetV2MetricsParamsTimeModeAbsolute, "absolute"},
			{GetV2MetricsParamsTimeModeRelative, "relative"},
		}, "Absolute")
	})
}

type round2ResponseMetadata interface {
	GetBody() []byte
	Status() string
	StatusCode() int
	ContentType() string
}

func assertRound2ResponseMetadata(t *testing.T, response round2ResponseMetadata, body []byte, status string, code int, contentType string) {
	t.Helper()
	if body == nil && response.GetBody() != nil {
		t.Errorf("GetBody() = %v, want nil", response.GetBody())
	} else if !bytes.Equal(response.GetBody(), body) {
		t.Errorf("GetBody() = %q, want %q", response.GetBody(), body)
	}
	if got := response.Status(); got != status {
		t.Errorf("Status() = %q, want %q", got, status)
	}
	if got := response.StatusCode(); got != code {
		t.Errorf("StatusCode() = %d, want %d", got, code)
	}
	if got := response.ContentType(); got != contentType {
		t.Errorf("ContentType() = %q, want %q", got, contentType)
	}
}

func assertRound2ResponsePointer[T any](t *testing.T, name string, got, want *T) {
	t.Helper()
	if got != want {
		t.Errorf("%s() = %p, want %p", name, got, want)
	}
}

func TestRound2ResponseAccessorContracts(t *testing.T) {
	t.Run("signing_key", func(t *testing.T) {
		var zero GetWellKnownSigningKeyPubResponse
		assertRound2ResponseMetadata(t, zero, nil, "", 0, "")
		assertRound2ResponsePointer(t, "GetJSON499", zero.GetJSON499(), nil)
		assertRound2ResponsePointer(t, "GetJSON503", zero.GetJSON503(), nil)
		body := []byte("signing_key response body")
		httpResponse := &http.Response{Status: "403 Forbidden", StatusCode: 403, Header: http.Header{"Content-Type": []string{"application/json; charset=utf-8"}}}
		value499 := &ModelsErrorResponse{}
		value503 := &ModelsErrorResponse{}
		populated := GetWellKnownSigningKeyPubResponse{Body: body, HTTPResponse: httpResponse,
			JSON499: value499,
			JSON503: value503,
		}
		assertRound2ResponseMetadata(t, populated, body, "403 Forbidden", 403, "application/json; charset=utf-8")
		assertRound2ResponsePointer(t, "GetJSON499", populated.GetJSON499(), value499)
		assertRound2ResponsePointer(t, "GetJSON503", populated.GetJSON503(), value503)
		singleError := GetWellKnownSigningKeyPubResponse{JSON499: value499}
		assertRound2ResponsePointer(t, "GetJSON499", singleError.GetJSON499(), value499)
		assertRound2ResponsePointer(t, "GetJSON503", singleError.GetJSON503(), nil)
	})
	t.Run("delete", func(t *testing.T) {
		var zero DeleteV1AlertMuteRulesResponse
		assertRound2ResponseMetadata(t, zero, nil, "", 0, "")
		assertRound2ResponsePointer(t, "GetJSON400", zero.GetJSON400(), nil)
		assertRound2ResponsePointer(t, "GetJSON401", zero.GetJSON401(), nil)
		assertRound2ResponsePointer(t, "GetJSON403", zero.GetJSON403(), nil)
		assertRound2ResponsePointer(t, "GetJSON404", zero.GetJSON404(), nil)
		assertRound2ResponsePointer(t, "GetJSON499", zero.GetJSON499(), nil)
		assertRound2ResponsePointer(t, "GetJSON500", zero.GetJSON500(), nil)
		body := []byte("delete response body")
		httpResponse := &http.Response{Status: "403 Forbidden", StatusCode: 403, Header: http.Header{"Content-Type": []string{"application/json; charset=utf-8"}}}
		value400 := &ModelsErrorResponse{}
		value401 := &ModelsErrorResponse{}
		value403 := &ModelsErrorResponse{}
		value404 := &ModelsErrorResponse{}
		value499 := &ModelsErrorResponse{}
		value500 := &ModelsErrorResponse{}
		populated := DeleteV1AlertMuteRulesResponse{Body: body, HTTPResponse: httpResponse,
			JSON400: value400,
			JSON401: value401,
			JSON403: value403,
			JSON404: value404,
			JSON499: value499,
			JSON500: value500,
		}
		assertRound2ResponseMetadata(t, populated, body, "403 Forbidden", 403, "application/json; charset=utf-8")
		assertRound2ResponsePointer(t, "GetJSON400", populated.GetJSON400(), value400)
		assertRound2ResponsePointer(t, "GetJSON401", populated.GetJSON401(), value401)
		assertRound2ResponsePointer(t, "GetJSON403", populated.GetJSON403(), value403)
		assertRound2ResponsePointer(t, "GetJSON404", populated.GetJSON404(), value404)
		assertRound2ResponsePointer(t, "GetJSON499", populated.GetJSON499(), value499)
		assertRound2ResponsePointer(t, "GetJSON500", populated.GetJSON500(), value500)
		singleError := DeleteV1AlertMuteRulesResponse{JSON403: value403}
		assertRound2ResponsePointer(t, "GetJSON400", singleError.GetJSON400(), nil)
		assertRound2ResponsePointer(t, "GetJSON401", singleError.GetJSON401(), nil)
		assertRound2ResponsePointer(t, "GetJSON403", singleError.GetJSON403(), value403)
		assertRound2ResponsePointer(t, "GetJSON404", singleError.GetJSON404(), nil)
		assertRound2ResponsePointer(t, "GetJSON499", singleError.GetJSON499(), nil)
		assertRound2ResponsePointer(t, "GetJSON500", singleError.GetJSON500(), nil)
	})
	t.Run("list", func(t *testing.T) {
		var zero GetV1AlertMuteRulesResponse
		assertRound2ResponseMetadata(t, zero, nil, "", 0, "")
		assertRound2ResponsePointer(t, "GetJSON200", zero.GetJSON200(), nil)
		assertRound2ResponsePointer(t, "GetJSON401", zero.GetJSON401(), nil)
		assertRound2ResponsePointer(t, "GetJSON403", zero.GetJSON403(), nil)
		assertRound2ResponsePointer(t, "GetJSON499", zero.GetJSON499(), nil)
		assertRound2ResponsePointer(t, "GetJSON500", zero.GetJSON500(), nil)
		body := []byte("list response body")
		httpResponse := &http.Response{Status: "403 Forbidden", StatusCode: 403, Header: http.Header{"Content-Type": []string{"application/json; charset=utf-8"}}}
		value200 := &ModelsListMuteRulesResponse{}
		value401 := &ModelsErrorResponse{}
		value403 := &ModelsErrorResponse{}
		value499 := &ModelsErrorResponse{}
		value500 := &ModelsErrorResponse{}
		populated := GetV1AlertMuteRulesResponse{Body: body, HTTPResponse: httpResponse,
			JSON200: value200,
			JSON401: value401,
			JSON403: value403,
			JSON499: value499,
			JSON500: value500,
		}
		assertRound2ResponseMetadata(t, populated, body, "403 Forbidden", 403, "application/json; charset=utf-8")
		assertRound2ResponsePointer(t, "GetJSON200", populated.GetJSON200(), value200)
		assertRound2ResponsePointer(t, "GetJSON401", populated.GetJSON401(), value401)
		assertRound2ResponsePointer(t, "GetJSON403", populated.GetJSON403(), value403)
		assertRound2ResponsePointer(t, "GetJSON499", populated.GetJSON499(), value499)
		assertRound2ResponsePointer(t, "GetJSON500", populated.GetJSON500(), value500)
		singleError := GetV1AlertMuteRulesResponse{JSON403: value403}
		assertRound2ResponsePointer(t, "GetJSON200", singleError.GetJSON200(), nil)
		assertRound2ResponsePointer(t, "GetJSON401", singleError.GetJSON401(), nil)
		assertRound2ResponsePointer(t, "GetJSON403", singleError.GetJSON403(), value403)
		assertRound2ResponsePointer(t, "GetJSON499", singleError.GetJSON499(), nil)
		assertRound2ResponsePointer(t, "GetJSON500", singleError.GetJSON500(), nil)
	})
	t.Run("put", func(t *testing.T) {
		var zero PutV1AlertMuteRulesResponse
		assertRound2ResponseMetadata(t, zero, nil, "", 0, "")
		assertRound2ResponsePointer(t, "GetJSON400", zero.GetJSON400(), nil)
		assertRound2ResponsePointer(t, "GetJSON401", zero.GetJSON401(), nil)
		assertRound2ResponsePointer(t, "GetJSON403", zero.GetJSON403(), nil)
		assertRound2ResponsePointer(t, "GetJSON499", zero.GetJSON499(), nil)
		assertRound2ResponsePointer(t, "GetJSON500", zero.GetJSON500(), nil)
		body := []byte("put response body")
		httpResponse := &http.Response{Status: "403 Forbidden", StatusCode: 403, Header: http.Header{"Content-Type": []string{"application/json; charset=utf-8"}}}
		value400 := &ModelsErrorResponse{}
		value401 := &ModelsErrorResponse{}
		value403 := &ModelsErrorResponse{}
		value499 := &ModelsErrorResponse{}
		value500 := &ModelsErrorResponse{}
		populated := PutV1AlertMuteRulesResponse{Body: body, HTTPResponse: httpResponse,
			JSON400: value400,
			JSON401: value401,
			JSON403: value403,
			JSON499: value499,
			JSON500: value500,
		}
		assertRound2ResponseMetadata(t, populated, body, "403 Forbidden", 403, "application/json; charset=utf-8")
		assertRound2ResponsePointer(t, "GetJSON400", populated.GetJSON400(), value400)
		assertRound2ResponsePointer(t, "GetJSON401", populated.GetJSON401(), value401)
		assertRound2ResponsePointer(t, "GetJSON403", populated.GetJSON403(), value403)
		assertRound2ResponsePointer(t, "GetJSON499", populated.GetJSON499(), value499)
		assertRound2ResponsePointer(t, "GetJSON500", populated.GetJSON500(), value500)
		singleError := PutV1AlertMuteRulesResponse{JSON403: value403}
		assertRound2ResponsePointer(t, "GetJSON400", singleError.GetJSON400(), nil)
		assertRound2ResponsePointer(t, "GetJSON401", singleError.GetJSON401(), nil)
		assertRound2ResponsePointer(t, "GetJSON403", singleError.GetJSON403(), value403)
		assertRound2ResponsePointer(t, "GetJSON499", singleError.GetJSON499(), nil)
		assertRound2ResponsePointer(t, "GetJSON500", singleError.GetJSON500(), nil)
	})
}

type round2AlertTimelineErrorReader struct {
	err    error
	closed bool
}

func (r *round2AlertTimelineErrorReader) Read(_ []byte) (int, error) {
	return 0, r.err
}

func (r *round2AlertTimelineErrorReader) Close() error {
	r.closed = true
	return nil
}

func assertRound2AlertTimelineError(t *testing.T, status int, getters map[int]*ModelsErrorResponse) {
	t.Helper()
	for code, value := range getters {
		if code != status {
			assertRound2ResponsePointer(t, fmt.Sprintf("GetJSON%d", code), value, nil)
			continue
		}
		if value == nil {
			t.Errorf("GetJSON%d() = nil, want decoded error", code)
			continue
		}
		if value.Error == nil || *value.Error != "denied" {
			t.Errorf("GetJSON%d().Error = %v, want denied", code, value.Error)
		}
		if value.Details == nil || *value.Details != "specific detail" {
			t.Errorf("GetJSON%d().Details = %v, want specific detail", code, value.Details)
		}
		if value.RequestStatus != nil {
			t.Errorf("GetJSON%d().RequestStatus = %v, want nil", code, value.RequestStatus)
		}
	}
}

func TestRound2AlertTimelineResponseContracts(t *testing.T) {
	const contentType = "application/json; charset=utf-8"
	body := []byte(`{"error":"denied","details":"specific detail"}`)
	response := func(status int, reader io.ReadCloser) *http.Response {
		return &http.Response{Status: fmt.Sprintf("%d supplied status", status), StatusCode: status,
			Header: http.Header{"Content-Type": []string{contentType}}, Body: reader}
	}

	t.Run("filter_options", func(t *testing.T) {
		var zero GetV1AlertTimelineFilterOptionsResponse
		assertRound2ResponseMetadata(t, zero, nil, "", 0, "")
		assertRound2ResponsePointer(t, "GetJSON200", zero.GetJSON200(), nil)
		assertRound2ResponsePointer(t, "GetJSON401", zero.GetJSON401(), nil)
		assertRound2ResponsePointer(t, "GetJSON499", zero.GetJSON499(), nil)
		assertRound2ResponsePointer(t, "GetJSON500", zero.GetJSON500(), nil)
		for _, status := range []int{499, 500} {
			t.Run(fmt.Sprint(status), func(t *testing.T) {
				rsp, err := ParseGetV1AlertTimelineFilterOptionsResponse(response(status, io.NopCloser(bytes.NewReader(body))))
				if err != nil || rsp == nil {
					t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
				}
				assertRound2ResponseMetadata(t, rsp, body, fmt.Sprintf("%d supplied status", status), status, contentType)
				assertRound2ResponsePointer(t, "GetJSON200", rsp.GetJSON200(), nil)
				assertRound2AlertTimelineError(t, status, map[int]*ModelsErrorResponse{401: rsp.GetJSON401(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500()})
			})
		}
		t.Run("malformed", func(t *testing.T) {
			rsp, err := ParseGetV1AlertTimelineFilterOptionsResponse(response(499, io.NopCloser(bytes.NewBufferString(`{"error":`))))
			if err == nil || rsp != nil {
				t.Errorf("parse = %v, %v; want nil response and decoding error", rsp, err)
			}
		})
		t.Run("read_error", func(t *testing.T) {
			sentinel := errors.New("filter options read error")
			reader := &round2AlertTimelineErrorReader{err: sentinel}
			rsp, err := ParseGetV1AlertTimelineFilterOptionsResponse(response(499, reader))
			if !errors.Is(err, sentinel) || rsp != nil || !reader.closed {
				t.Errorf("parse = %v, %v; closed = %t; want nil response, sentinel error and closed body", rsp, err, reader.closed)
			}
		})
	})

	t.Run("nodes", func(t *testing.T) {
		var zero GetV1AlertTimelineNodesResponse
		assertRound2ResponseMetadata(t, zero, nil, "", 0, "")
		assertRound2ResponsePointer(t, "GetJSON200", zero.GetJSON200(), nil)
		assertRound2ResponsePointer(t, "GetJSON400", zero.GetJSON400(), nil)
		assertRound2ResponsePointer(t, "GetJSON401", zero.GetJSON401(), nil)
		assertRound2ResponsePointer(t, "GetJSON499", zero.GetJSON499(), nil)
		assertRound2ResponsePointer(t, "GetJSON500", zero.GetJSON500(), nil)
		for _, status := range []int{400, 499, 500} {
			t.Run(fmt.Sprint(status), func(t *testing.T) {
				rsp, err := ParseGetV1AlertTimelineNodesResponse(response(status, io.NopCloser(bytes.NewReader(body))))
				if err != nil || rsp == nil {
					t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
				}
				assertRound2ResponseMetadata(t, rsp, body, fmt.Sprintf("%d supplied status", status), status, contentType)
				assertRound2ResponsePointer(t, "GetJSON200", rsp.GetJSON200(), nil)
				assertRound2AlertTimelineError(t, status, map[int]*ModelsErrorResponse{400: rsp.GetJSON400(), 401: rsp.GetJSON401(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500()})
			})
		}
		t.Run("malformed", func(t *testing.T) {
			rsp, err := ParseGetV1AlertTimelineNodesResponse(response(400, io.NopCloser(bytes.NewBufferString(`{"error":`))))
			if err == nil || rsp != nil {
				t.Errorf("parse = %v, %v; want nil response and decoding error", rsp, err)
			}
		})
		t.Run("read_error", func(t *testing.T) {
			sentinel := errors.New("nodes read error")
			reader := &round2AlertTimelineErrorReader{err: sentinel}
			rsp, err := ParseGetV1AlertTimelineNodesResponse(response(400, reader))
			if !errors.Is(err, sentinel) || rsp != nil || !reader.closed {
				t.Errorf("parse = %v, %v; closed = %t; want nil response, sentinel error and closed body", rsp, err, reader.closed)
			}
		})
	})
}

// testRound2ReportUnion checks the typed conversion and wire representation at
// each stage, including that a partial merge preserves the remaining fields.
func testRound2ReportUnion[M any, U any](t *testing.T, initial, patch, merged, invalid string,
	from func(*U, M) error, as func(*U) (M, error), merge func(*U, M) error) {
	t.Helper()
	decodeModel := func(input string) M {
		t.Helper()
		var model M
		if err := json.Unmarshal([]byte(input), &model); err != nil {
			t.Fatalf("unmarshal model: %v", err)
		}
		return model
	}
	check := func(stage string, union *U, expected string) {
		t.Helper()
		got, err := as(union)
		if err != nil {
			t.Fatalf("%s As: %v", stage, err)
		}
		want := decodeModel(expected)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s decoded model = %+v, want %+v", stage, got, want)
		}
		encoded, err := json.Marshal(got)
		if err != nil {
			t.Fatalf("%s marshal model: %v", stage, err)
		}
		var gotJSON, wantJSON any
		if err := json.Unmarshal(encoded, &gotJSON); err != nil {
			t.Fatalf("%s unmarshal encoded model: %v", stage, err)
		}
		if err := json.Unmarshal([]byte(expected), &wantJSON); err != nil {
			t.Fatalf("%s unmarshal expected JSON: %v", stage, err)
		}
		if !reflect.DeepEqual(gotJSON, wantJSON) {
			t.Errorf("%s fields = %s, want %s", stage, encoded, expected)
		}
	}

	var union U
	if err := from(&union, decodeModel(initial)); err != nil {
		t.Fatalf("From: %v", err)
	}
	check("From", &union, initial)
	if err := merge(&union, decodeModel(patch)); err != nil {
		t.Fatalf("Merge: %v", err)
	}
	check("Merge", &union, merged)
	encoded, err := json.Marshal(union)
	if err != nil {
		t.Fatalf("marshal union: %v", err)
	}
	var roundTrip U
	if err := json.Unmarshal(encoded, &roundTrip); err != nil {
		t.Fatalf("unmarshal union: %v", err)
	}
	check("round trip", &roundTrip, merged)

	var incompatible U
	if err := json.Unmarshal([]byte(invalid), &incompatible); err != nil {
		t.Fatalf("unmarshal incompatible union: %v", err)
	}
	if _, err := as(&incompatible); err == nil {
		t.Error("As accepted an incompatible field type")
	}
}

func TestRound2ReportResponseUnionContracts(t *testing.T) {
	t.Run("inband", func(t *testing.T) {
		testRound2ReportUnion(t,
			`{"nodes":[{"nodeUUID":"inband-1","hostname":"host-1"}],"page":2,"pageSize":25,"total":51,"hasMore":true}`,
			`{"page":3}`,
			`{"nodes":[{"nodeUUID":"inband-1","hostname":"host-1"}],"page":3,"pageSize":25,"total":51,"hasMore":true}`,
			`{"page":"invalid"}`,
			(*GetV1ReportsInventory200JSONResponseBody).FromModelsInbandInventoryReportResponse,
			(*GetV1ReportsInventory200JSONResponseBody).AsModelsInbandInventoryReportResponse,
			(*GetV1ReportsInventory200JSONResponseBody).MergeModelsInbandInventoryReportResponse)
	})
	t.Run("oob", func(t *testing.T) {
		testRound2ReportUnion(t,
			`{"nodes":[{"nodeUUID":"oob-1","nodeName":"server-1","bmcMac":"00:11:22:33:44:55"}],"page":2,"pageSize":25,"total":51,"hasMore":true}`,
			`{"page":3}`,
			`{"nodes":[{"nodeUUID":"oob-1","nodeName":"server-1","bmcMac":"00:11:22:33:44:55"}],"page":3,"pageSize":25,"total":51,"hasMore":true}`,
			`{"page":"invalid"}`,
			(*GetV1ReportsInventory200JSONResponseBody).FromModelsOobInventoryReportResponse,
			(*GetV1ReportsInventory200JSONResponseBody).AsModelsOobInventoryReportResponse,
			(*GetV1ReportsInventory200JSONResponseBody).MergeModelsOobInventoryReportResponse)
	})
	t.Run("by_error", func(t *testing.T) {
		testRound2ReportUnion(t,
			`{"errors":[{"name":"xid_154","count":7}],"page":2,"pageSize":25,"total":51,"hasMore":true}`,
			`{"page":3}`,
			`{"errors":[{"name":"xid_154","count":7}],"page":3,"pageSize":25,"total":51,"hasMore":true}`,
			`{"page":"invalid"}`,
			(*GetV1ReportsError200JSONResponseBody).FromModelsByErrorResponse,
			(*GetV1ReportsError200JSONResponseBody).AsModelsByErrorResponse,
			(*GetV1ReportsError200JSONResponseBody).MergeModelsByErrorResponse)
	})
	t.Run("by_node", func(t *testing.T) {
		testRound2ReportUnion(t,
			`{"nodes":[{"nodeUUID":"node-1","errors":["xid_154"]}],"page":2,"pageSize":25,"total":51,"hasMore":true}`,
			`{"page":3}`,
			`{"nodes":[{"nodeUUID":"node-1","errors":["xid_154"]}],"page":3,"pageSize":25,"total":51,"hasMore":true}`,
			`{"page":"invalid"}`,
			(*GetV1ReportsError200JSONResponseBody).FromModelsByNodeResponse,
			(*GetV1ReportsError200JSONResponseBody).AsModelsByNodeResponse,
			(*GetV1ReportsError200JSONResponseBody).MergeModelsByNodeResponse)
	})
	t.Run("overview", func(t *testing.T) {
		testRound2ReportUnion(t,
			`{"totalErrors":12,"totalErrorNodes":4,"totalErrorTypes":3}`,
			`{"totalErrors":13}`,
			`{"totalErrors":13,"totalErrorNodes":4,"totalErrorTypes":3}`,
			`{"totalErrors":"invalid"}`,
			(*GetV1ReportsError200JSONResponseBody).FromModelsErrorOverviewResponse,
			(*GetV1ReportsError200JSONResponseBody).AsModelsErrorOverviewResponse,
			(*GetV1ReportsError200JSONResponseBody).MergeModelsErrorOverviewResponse)
	})
	t.Run("graph", func(t *testing.T) {
		testRound2ReportUnion(t,
			`{"result":[{"label":{"error":"xid_154"},"values":"[[1716153600,5]]"}],"timeRange":{"start":"2026-05-01T00:00:00Z","end":"2026-05-02T00:00:00Z"}}`,
			`{"timeRange":{"end":"2026-05-03T00:00:00Z"}}`,
			`{"result":[{"label":{"error":"xid_154"},"values":"[[1716153600,5]]"}],"timeRange":{"start":"2026-05-01T00:00:00Z","end":"2026-05-03T00:00:00Z"}}`,
			`{"result":"invalid"}`,
			(*GetV1ReportsError200JSONResponseBody).FromModelsErrorReportGraphByErrorResponse,
			(*GetV1ReportsError200JSONResponseBody).AsModelsErrorReportGraphByErrorResponse,
			(*GetV1ReportsError200JSONResponseBody).MergeModelsErrorReportGraphByErrorResponse)
	})
}

func TestRound2ResourceResponseUnionContracts(t *testing.T) {
	t.Run("compute_zones", func(t *testing.T) {
		testRound2ReportUnion(t,
			`{"computezones":[{"id":"zone-1","name":"Alpha"}],"page":2,"pageSize":25,"total":51,"hasMore":true}`,
			`{"page":3}`,
			`{"computezones":[{"id":"zone-1","name":"Alpha"}],"page":3,"pageSize":25,"total":51,"hasMore":true}`,
			`{"page":"invalid"}`,
			(*GetV1Computezones200JSONResponseBody).FromModelsComputeZonesResponse,
			(*GetV1Computezones200JSONResponseBody).AsModelsComputeZonesResponse,
			(*GetV1Computezones200JSONResponseBody).MergeModelsComputeZonesResponse)
	})
	t.Run("list_compute_zones", func(t *testing.T) {
		testRound2ReportUnion(t,
			`{"computezones":[{"id":"zone-1","name":"Alpha"}],"page":2,"pageSize":25,"total":51,"hasMore":true}`,
			`{"page":3}`,
			`{"computezones":[{"id":"zone-1","name":"Alpha"}],"page":3,"pageSize":25,"total":51,"hasMore":true}`,
			`{"page":"invalid"}`,
			(*GetV1Computezones200JSONResponseBody).FromModelsListComputeZonesResponse,
			(*GetV1Computezones200JSONResponseBody).AsModelsListComputeZonesResponse,
			(*GetV1Computezones200JSONResponseBody).MergeModelsListComputeZonesResponse)
	})
	t.Run("node_groups", func(t *testing.T) {
		testRound2ReportUnion(t,
			`{"nodeGroups":[{"id":"group-1","name":"Alpha"}],"page":2,"pageSize":25,"total":51,"hasMore":true}`,
			`{"page":3}`,
			`{"nodeGroups":[{"id":"group-1","name":"Alpha"}],"page":3,"pageSize":25,"total":51,"hasMore":true}`,
			`{"page":"invalid"}`,
			(*GetV1Nodegroups200JSONResponseBody).FromModelsNodeGroupsResponse,
			(*GetV1Nodegroups200JSONResponseBody).AsModelsNodeGroupsResponse,
			(*GetV1Nodegroups200JSONResponseBody).MergeModelsNodeGroupsResponse)
	})
	t.Run("list_node_groups", func(t *testing.T) {
		testRound2ReportUnion(t,
			`{"nodeGroups":[{"id":"group-1","name":"Alpha"}],"page":2,"pageSize":25,"total":51,"hasMore":true}`,
			`{"page":3}`,
			`{"nodeGroups":[{"id":"group-1","name":"Alpha"}],"page":3,"pageSize":25,"total":51,"hasMore":true}`,
			`{"page":"invalid"}`,
			(*GetV1Nodegroups200JSONResponseBody).FromModelsListNodeGroupsResponse,
			(*GetV1Nodegroups200JSONResponseBody).AsModelsListNodeGroupsResponse,
			(*GetV1Nodegroups200JSONResponseBody).MergeModelsListNodeGroupsResponse)
	})
	t.Run("inband", func(t *testing.T) {
		testRound2ReportUnion(t,
			`{"nodeUUID":"node-1","dataSource":"inband","hostname":"host-1","gpuCount":4}`,
			`{"nodeUUID":"node-1","dataSource":"inband","hostname":"host-2"}`,
			`{"nodeUUID":"node-1","dataSource":"inband","hostname":"host-2","gpuCount":4}`,
			`{"gpuCount":"invalid"}`,
			(*GetV1NodesNodeUuid200JSONResponseBody).FromModelsInbandNodeDetailsResponse,
			(*GetV1NodesNodeUuid200JSONResponseBody).AsModelsInbandNodeDetailsResponse,
			(*GetV1NodesNodeUuid200JSONResponseBody).MergeModelsInbandNodeDetailsResponse)
	})
	t.Run("oob", func(t *testing.T) {
		testRound2ReportUnion(t,
			`{"nodeUUID":"node-1","dataSource":"oob","nodeName":"machine-1","bmcIP":"192.0.2.1"}`,
			`{"nodeUUID":"node-1","dataSource":"oob","nodeName":"machine-2"}`,
			`{"nodeUUID":"node-1","dataSource":"oob","nodeName":"machine-2","bmcIP":"192.0.2.1"}`,
			`{"gpuCount":"invalid"}`,
			(*GetV1NodesNodeUuid200JSONResponseBody).FromModelsOobNodeDetailsResponse,
			(*GetV1NodesNodeUuid200JSONResponseBody).AsModelsOobNodeDetailsResponse,
			(*GetV1NodesNodeUuid200JSONResponseBody).MergeModelsOobNodeDetailsResponse)
	})
}

func TestRound2NodesAndSecurityResponseUnionContracts(t *testing.T) {
	t.Run("nodes_inband", func(t *testing.T) {
		testRound2ReportUnion(t,
			`{"nodes":[{"nodeUUID":"node-inband","dataSource":"inband","hostname":"host-1","gpuCount":0}],"page":2,"pageSize":25,"total":51,"hasMore":true}`,
			`{"page":3}`,
			`{"nodes":[{"nodeUUID":"node-inband","dataSource":"inband","hostname":"host-1","gpuCount":0}],"page":3,"pageSize":25,"total":51,"hasMore":true}`,
			`{"page":"invalid"}`,
			(*GetV1Nodes200JSONResponseBody).FromModelsInbandNodesResponse,
			(*GetV1Nodes200JSONResponseBody).AsModelsInbandNodesResponse,
			(*GetV1Nodes200JSONResponseBody).MergeModelsInbandNodesResponse)
	})
	t.Run("nodes_oob", func(t *testing.T) {
		testRound2ReportUnion(t,
			`{"nodes":[{"nodeUUID":"node-oob","dataSource":"oob","nodeName":"rack-1","bmcIP":"192.0.2.1","gpuCount":8}],"page":2,"pageSize":25,"total":51,"hasMore":true}`,
			`{"page":3}`,
			`{"nodes":[{"nodeUUID":"node-oob","dataSource":"oob","nodeName":"rack-1","bmcIP":"192.0.2.1","gpuCount":8}],"page":3,"pageSize":25,"total":51,"hasMore":true}`,
			`{"page":"invalid"}`,
			(*GetV1Nodes200JSONResponseBody).FromModelsOobNodesResponse,
			(*GetV1Nodes200JSONResponseBody).AsModelsOobNodesResponse,
			(*GetV1Nodes200JSONResponseBody).MergeModelsOobNodesResponse)
	})
	t.Run("security_full", func(t *testing.T) {
		const initial = `{"vulnerabilities":[{"vulnerability":"CVE-2026-1234","vulnerabilityUrl":"https://example.com/CVE-2026-1234","component":"gpuDriver","severity":"CRITICAL","affectedNodes":7,"fixVersions":["550.1","560.2"],"description":"Driver issue"}],"summary":{"affectedNodes":9,"distinctComponentCount":2,"totalNodes":20,"severityCounts":{"critical":3,"high":4,"medium":5,"low":6}},"page":0,"pageSize":25,"total":30,"hasMore":true}`
		const merged = `{"vulnerabilities":[{"vulnerability":"CVE-2026-1234","vulnerabilityUrl":"https://example.com/CVE-2026-1234","component":"gpuDriver","severity":"CRITICAL","affectedNodes":7,"fixVersions":["550.1","560.2"],"description":"Driver issue"}],"summary":{"affectedNodes":9,"distinctComponentCount":2,"totalNodes":20,"severityCounts":{"critical":3,"high":4,"medium":5,"low":6}},"page":0,"pageSize":25,"total":31,"hasMore":true}`
		testRound2ReportUnion(t, initial, merged, merged, `{"total":"invalid"}`,
			(*GetV1ReportsSecurityVulnerabilities200JSONResponseBody).FromModelsSecurityVulnerabilitiesResponse,
			(*GetV1ReportsSecurityVulnerabilities200JSONResponseBody).AsModelsSecurityVulnerabilitiesResponse,
			(*GetV1ReportsSecurityVulnerabilities200JSONResponseBody).MergeModelsSecurityVulnerabilitiesResponse)
	})
	t.Run("security_count", func(t *testing.T) {
		testRound2ReportUnion(t, `{"total":3}`, `{"total":4}`, `{"total":4}`, `{"total":"invalid"}`,
			(*GetV1ReportsSecurityVulnerabilities200JSONResponseBody).FromGetV1ReportsSecurityVulnerabilities200JSONResponseBody1,
			(*GetV1ReportsSecurityVulnerabilities200JSONResponseBody).AsGetV1ReportsSecurityVulnerabilities200JSONResponseBody1,
			(*GetV1ReportsSecurityVulnerabilities200JSONResponseBody).MergeGetV1ReportsSecurityVulnerabilities200JSONResponseBody1)
	})
}

func testRound2AlertTimelineNodeDetailResponse[T any, R interface {
	comparable
	round2ResponseMetadata
	GetJSON200() *T
	GetJSON400() *ModelsErrorResponse
	GetJSON401() *ModelsErrorResponse
	GetJSON404() *ModelsErrorResponse
	GetJSON499() *ModelsErrorResponse
	GetJSON500() *ModelsErrorResponse
}](t *testing.T, zero R, parse func(*http.Response) (R, error), successBody string, checkSuccess func(*testing.T, *T)) {
	t.Helper()
	const jsonType = "application/json; charset=utf-8"
	makeResponse := func(status int, contentType string, body io.ReadCloser) *http.Response {
		return &http.Response{Status: fmt.Sprintf("%d supplied status", status), StatusCode: status,
			Header: http.Header{"Content-Type": []string{contentType}}, Body: body}
	}
	errorsFor := func(rsp R) map[int]*ModelsErrorResponse {
		return map[int]*ModelsErrorResponse{400: rsp.GetJSON400(), 401: rsp.GetJSON401(), 404: rsp.GetJSON404(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500()}
	}
	assertRound2ResponseMetadata(t, zero, nil, "", 0, "")
	assertRound2ResponsePointer(t, "GetJSON200", zero.GetJSON200(), nil)
	assertRound2AlertTimelineError(t, 0, errorsFor(zero))

	body := []byte(`{"error":"denied","details":"specific detail"}`)
	for _, status := range []int{400, 401, 404, 499, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			rsp, err := parse(makeResponse(status, jsonType, io.NopCloser(bytes.NewReader(body))))
			var nilResponse R
			if err != nil || rsp == nilResponse {
				t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
			}
			assertRound2ResponseMetadata(t, rsp, body, fmt.Sprintf("%d supplied status", status), status, jsonType)
			assertRound2ResponsePointer(t, "GetJSON200", rsp.GetJSON200(), nil)
			assertRound2AlertTimelineError(t, status, errorsFor(rsp))
		})
	}
	t.Run("success", func(t *testing.T) {
		body := []byte(successBody)
		rsp, err := parse(makeResponse(http.StatusOK, jsonType, io.NopCloser(bytes.NewReader(body))))
		var nilResponse R
		if err != nil || rsp == nilResponse {
			t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
		}
		assertRound2ResponseMetadata(t, rsp, body, "200 supplied status", 200, jsonType)
		assertRound2AlertTimelineError(t, 0, errorsFor(rsp))
		checkSuccess(t, rsp.GetJSON200())
	})
	t.Run("malformed", func(t *testing.T) {
		rsp, err := parse(makeResponse(400, jsonType, io.NopCloser(bytes.NewBufferString(`{"error":`))))
		var nilResponse R
		if err == nil || rsp != nilResponse {
			t.Errorf("parse = %v, %v; want nil response and decoding error", rsp, err)
		}
	})
	t.Run("read_error", func(t *testing.T) {
		sentinel := errors.New("node detail read error")
		reader := &round2AlertTimelineErrorReader{err: sentinel}
		rsp, err := parse(makeResponse(400, jsonType, reader))
		var nilResponse R
		if !errors.Is(err, sentinel) || rsp != nilResponse || !reader.closed {
			t.Errorf("parse = %v, %v; closed = %t; want nil response, sentinel error and closed body", rsp, err, reader.closed)
		}
	})
	t.Run("text_plain", func(t *testing.T) {
		rsp, err := parse(makeResponse(404, "text/plain", io.NopCloser(bytes.NewReader(body))))
		var nilResponse R
		if err != nil || rsp == nilResponse {
			t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
		}
		assertRound2ResponseMetadata(t, rsp, body, "404 supplied status", 404, "text/plain")
		assertRound2ResponsePointer(t, "GetJSON200", rsp.GetJSON200(), nil)
		assertRound2AlertTimelineError(t, 0, errorsFor(rsp))
	})
}

func TestRound2AlertTimelineNodeDetailResponseContracts(t *testing.T) {
	t.Run("node_alerts", func(t *testing.T) {
		testRound2AlertTimelineNodeDetailResponse(t, &GetV1AlertTimelineNodesNodeUuidAlertsResponse{}, ParseGetV1AlertTimelineNodesNodeUuidAlertsResponse,
			`{"nodeUuid":"node-1","hostname":"host-1","alerts":[{"alertUuid":"alert-1","component":"gpu","dataSource":"inband","alertStatus":"Critical"}],"page":1,"pageSize":10,"total":12,"hasMore":true}`,
			func(t *testing.T, result *ModelsAlertTimelineNodeAlertsResponse) {
				if result == nil || result.Alerts == nil || len(*result.Alerts) != 1 {
					t.Fatalf("missing alerts: %+v", result)
				}
				alert := (*result.Alerts)[0]
				if result.NodeUuid != "node-1" || result.Hostname == nil || *result.Hostname != "host-1" || alert.AlertUuid != "alert-1" || alert.Component == nil || *alert.Component != "gpu" || alert.DataSource == nil || *alert.DataSource != "inband" || alert.AlertStatus == nil || *alert.AlertStatus != "Critical" {
					t.Errorf("node alerts = %+v, alert = %+v", result, alert)
				}
				if result.Page == nil || *result.Page != 1 || result.PageSize == nil || *result.PageSize != 10 || result.Total == nil || *result.Total != 12 || result.HasMore == nil || !*result.HasMore {
					t.Errorf("node alerts pagination = %+v", result)
				}
			})
	})
	t.Run("alert_detail", func(t *testing.T) {
		testRound2AlertTimelineNodeDetailResponse(t, &GetV1AlertTimelineNodesNodeUuidAlertsAlertUuidResponse{}, ParseGetV1AlertTimelineNodesNodeUuidAlertsAlertUuidResponse,
			`{"alertUuid":"alert-1","nodeUuid":"node-1","component":"gpu","page":1,"pageSize":10,"total":12,"hasMore":true,"timeline":[{"eventType":"triggered","message":"GPU critical","extraInfo":{"gpu":"0"}}]}`,
			func(t *testing.T, result *ModelsAlertTimelineDetailResponse) {
				if result == nil || result.Timeline == nil || len(*result.Timeline) != 1 {
					t.Fatalf("missing timeline: %+v", result)
				}
				event := (*result.Timeline)[0]
				if result.AlertUuid != "alert-1" || result.NodeUuid != "node-1" || result.Component == nil || *result.Component != "gpu" || event.EventType == nil || string(*event.EventType) != "triggered" || event.Message == nil || *event.Message != "GPU critical" || event.ExtraInfo == nil || !reflect.DeepEqual(*event.ExtraInfo, map[string]interface{}{"gpu": "0"}) {
					t.Errorf("detail = %+v, event = %+v", result, event)
				}
				if result.Page == nil || *result.Page != 1 || result.PageSize == nil || *result.PageSize != 10 || result.Total == nil || *result.Total != 12 || result.HasMore == nil || !*result.HasMore {
					t.Errorf("detail pagination = %+v", result)
				}
			})
	})
}

func testRound2OverviewEventBucketsResponse[T any, R interface {
	comparable
	round2ResponseMetadata
	GetJSON200() *T
	GetJSON400() *ModelsErrorResponse
	GetJSON401() *ModelsErrorResponse
	GetJSON403() *ModelsErrorResponse
	GetJSON499() *ModelsErrorResponse
	GetJSON500() *ModelsErrorResponse
	GetJSON503() *ModelsErrorResponse
}](t *testing.T, zero R, parse func(*http.Response) (R, error), successBody string, checkSuccess func(*testing.T, *T)) {
	t.Helper()
	const jsonType = "application/json; charset=utf-8"
	makeResponse := func(status int, contentType string, body io.ReadCloser) *http.Response {
		return &http.Response{Status: fmt.Sprintf("%d supplied status", status), StatusCode: status,
			Header: http.Header{"Content-Type": []string{contentType}}, Body: body}
	}
	errorsFor := func(rsp R) map[int]*ModelsErrorResponse {
		return map[int]*ModelsErrorResponse{400: rsp.GetJSON400(), 401: rsp.GetJSON401(), 403: rsp.GetJSON403(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500(), 503: rsp.GetJSON503()}
	}
	assertRound2ResponseMetadata(t, zero, nil, "", 0, "")
	assertRound2ResponsePointer(t, "GetJSON200", zero.GetJSON200(), nil)
	assertRound2AlertTimelineError(t, 0, errorsFor(zero))

	body := []byte(`{"error":"denied","details":"specific detail"}`)
	for _, status := range []int{400, 401, 403, 499, 500, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			rsp, err := parse(makeResponse(status, jsonType, io.NopCloser(bytes.NewReader(body))))
			var nilResponse R
			if err != nil || rsp == nilResponse {
				t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
			}
			assertRound2ResponseMetadata(t, rsp, body, fmt.Sprintf("%d supplied status", status), status, jsonType)
			assertRound2ResponsePointer(t, "GetJSON200", rsp.GetJSON200(), nil)
			assertRound2AlertTimelineError(t, status, errorsFor(rsp))
		})
	}
	t.Run("success", func(t *testing.T) {
		body := []byte(successBody)
		rsp, err := parse(makeResponse(200, jsonType, io.NopCloser(bytes.NewReader(body))))
		var nilResponse R
		if err != nil || rsp == nilResponse {
			t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
		}
		assertRound2ResponseMetadata(t, rsp, body, "200 supplied status", 200, jsonType)
		assertRound2AlertTimelineError(t, 0, errorsFor(rsp))
		checkSuccess(t, rsp.GetJSON200())
	})
	for _, status := range []int{200, 400, 401, 403, 499, 500, 503} {
		t.Run(fmt.Sprintf("malformed_%d", status), func(t *testing.T) {
			rsp, err := parse(makeResponse(status, jsonType, io.NopCloser(bytes.NewBufferString(`{"error":`))))
			var nilResponse R
			if err == nil || rsp != nilResponse {
				t.Errorf("parse = %v, %v; want nil response and decoding error", rsp, err)
			}
		})
	}
	t.Run("read_error", func(t *testing.T) {
		sentinel := errors.New("overview or event buckets read error")
		reader := &round2AlertTimelineErrorReader{err: sentinel}
		rsp, err := parse(makeResponse(400, jsonType, reader))
		var nilResponse R
		if !errors.Is(err, sentinel) || rsp != nilResponse || !reader.closed {
			t.Errorf("parse = %v, %v; closed = %t; want nil response, sentinel error and closed body", rsp, err, reader.closed)
		}
	})
	t.Run("text_plain", func(t *testing.T) {
		rsp, err := parse(makeResponse(403, "text/plain", io.NopCloser(bytes.NewReader(body))))
		var nilResponse R
		if err != nil || rsp == nilResponse {
			t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
		}
		assertRound2ResponseMetadata(t, rsp, body, "403 supplied status", 403, "text/plain")
		assertRound2ResponsePointer(t, "GetJSON200", rsp.GetJSON200(), nil)
		assertRound2AlertTimelineError(t, 0, errorsFor(rsp))
	})
}

func TestRound2OverviewAndEventBucketsResponseContracts(t *testing.T) {
	t.Run("overview", func(t *testing.T) {
		testRound2OverviewEventBucketsResponse(t, &GetV1OverviewResponse{}, ParseGetV1OverviewResponse,
			`{"nodesCount":12,"computeZoneCount":2}`, func(t *testing.T, result *ModelsOverviewResponse) {
				if result == nil || result.NodesCount == nil || *result.NodesCount != 12 || result.ComputeZoneCount == nil || *result.ComputeZoneCount != 2 {
					t.Errorf("GetJSON200() = %+v; want nodesCount 12 and computeZoneCount 2", result)
				}
			})
	})
	t.Run("event_buckets", func(t *testing.T) {
		testRound2OverviewEventBucketsResponse(t, &GetV1EventsBucketsResponse{}, ParseGetV1EventsBucketsResponse,
			`{"bucketInterval":"1h","buckets":[{"count":3}]}`, func(t *testing.T, result *ModelsEventBucketsResponse) {
				if result == nil || result.BucketInterval == nil || *result.BucketInterval != "1h" || result.Buckets == nil || len(*result.Buckets) != 1 || (*result.Buckets)[0].Count == nil || *(*result.Buckets)[0].Count != 3 {
					t.Errorf("GetJSON200() = %+v; want bucketInterval 1h and one bucket with count 3", result)
				}
			})
	})
}

func testRound2AlertsResponse[T any, R interface {
	comparable
	round2ResponseMetadata
	GetJSON200() *T
}](t *testing.T, zero R, parse func(*http.Response) (R, error), statuses []int, errorsFor func(R) map[int]*ModelsErrorResponse, successBody string, checkSuccess func(*testing.T, *T)) {
	t.Helper()
	const jsonType = "application/json; charset=utf-8"
	makeResponse := func(status int, contentType string, body io.ReadCloser) *http.Response {
		return &http.Response{Status: fmt.Sprintf("%d supplied status", status), StatusCode: status,
			Header: http.Header{"Content-Type": []string{contentType}}, Body: body}
	}
	assertRound2ResponseMetadata(t, zero, nil, "", 0, "")
	assertRound2ResponsePointer(t, "GetJSON200", zero.GetJSON200(), nil)
	assertRound2AlertTimelineError(t, 0, errorsFor(zero))

	body := []byte(`{"error":"denied","details":"specific detail"}`)
	for _, status := range statuses {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			rsp, err := parse(makeResponse(status, jsonType, io.NopCloser(bytes.NewReader(body))))
			var nilResponse R
			if err != nil || rsp == nilResponse {
				t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
			}
			assertRound2ResponseMetadata(t, rsp, body, fmt.Sprintf("%d supplied status", status), status, jsonType)
			assertRound2ResponsePointer(t, "GetJSON200", rsp.GetJSON200(), nil)
			assertRound2AlertTimelineError(t, status, errorsFor(rsp))
		})
	}
	t.Run("success", func(t *testing.T) {
		body := []byte(successBody)
		rsp, err := parse(makeResponse(200, jsonType, io.NopCloser(bytes.NewReader(body))))
		var nilResponse R
		if err != nil || rsp == nilResponse {
			t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
		}
		assertRound2ResponseMetadata(t, rsp, body, "200 supplied status", 200, jsonType)
		assertRound2AlertTimelineError(t, 0, errorsFor(rsp))
		checkSuccess(t, rsp.GetJSON200())
	})
	t.Run("malformed", func(t *testing.T) {
		rsp, err := parse(makeResponse(400, jsonType, io.NopCloser(bytes.NewBufferString(`{"error":`))))
		var nilResponse R
		if err == nil || rsp != nilResponse {
			t.Errorf("parse = %v, %v; want nil response and decoding error", rsp, err)
		}
	})
	t.Run("read_error", func(t *testing.T) {
		sentinel := errors.New("alerts read error")
		reader := &round2AlertTimelineErrorReader{err: sentinel}
		rsp, err := parse(makeResponse(400, jsonType, reader))
		var nilResponse R
		if !errors.Is(err, sentinel) || rsp != nilResponse || !reader.closed {
			t.Errorf("parse = %v, %v; closed = %t; want nil response, sentinel error and closed body", rsp, err, reader.closed)
		}
	})
	t.Run("text_plain", func(t *testing.T) {
		rsp, err := parse(makeResponse(400, "text/plain", io.NopCloser(bytes.NewReader(body))))
		var nilResponse R
		if err != nil || rsp == nilResponse {
			t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
		}
		assertRound2ResponseMetadata(t, rsp, body, "400 supplied status", 400, "text/plain")
		assertRound2ResponsePointer(t, "GetJSON200", rsp.GetJSON200(), nil)
		assertRound2AlertTimelineError(t, 0, errorsFor(rsp))
	})
}

func TestRound2AlertsResponseContracts(t *testing.T) {
	// Statuses are declared by the /v1/alerts and /v1/alerts/{nodeUUID} OpenAPI responses.
	t.Run("alerts", func(t *testing.T) {
		testRound2AlertsResponse(t, &GetV1AlertsResponse{}, ParseGetV1AlertsResponse, []int{400, 499, 500},
			func(rsp *GetV1AlertsResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: rsp.GetJSON400(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500()}
			}, `{"alerts":[{"alertUUID":"alert-1","nodeUUID":"node-1"}],"page":2,"total":12}`,
			func(t *testing.T, result *ModelsAlertResponse) {
				t.Helper()
				if result == nil || result.Alerts == nil || len(*result.Alerts) != 1 {
					t.Fatalf("GetJSON200() = %+v; want one alert", result)
				}
				alert := (*result.Alerts)[0]
				if alert.AlertUUID != "alert-1" || alert.NodeUUID != "node-1" || result.Page == nil || *result.Page != 2 || result.Total == nil || *result.Total != 12 {
					t.Errorf("GetJSON200() = %+v, alert = %+v; want alert-1, node-1, page 2 and total 12", result, alert)
				}
			})
	})
	t.Run("node_alerts", func(t *testing.T) {
		testRound2AlertsResponse(t, &GetV1AlertsNodeUuidResponse{}, ParseGetV1AlertsNodeUuidResponse, []int{400, 404, 499, 500},
			func(rsp *GetV1AlertsNodeUuidResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: rsp.GetJSON400(), 404: rsp.GetJSON404(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500()}
			}, `{"nodeUUID":"node-1","alert":[{"component":"gpu","data":[{"alertUUID":"alert-1","nodeUUID":"node-1"}]}]}`,
			func(t *testing.T, result *ModelsNodeAlertResponse) {
				t.Helper()
				if result == nil || result.Alert == nil || len(*result.Alert) != 1 {
					t.Fatalf("GetJSON200() = %+v; want one alert group", result)
				}
				group := (*result.Alert)[0]
				if group.Data == nil || len(*group.Data) != 1 {
					t.Fatalf("alert group = %+v; want one alert", group)
				}
				alert := (*group.Data)[0]
				if result.NodeUUID != "node-1" || group.Component == nil || *group.Component != "gpu" || alert.AlertUUID != "alert-1" || alert.NodeUUID != "node-1" {
					t.Errorf("GetJSON200() = %+v, group = %+v, alert = %+v; want node-1, gpu and alert-1", result, group, alert)
				}
			})
	})
}

func TestRound2NodeAndNodeGroupResponseContracts(t *testing.T) {
	t.Run("nodegroups", func(t *testing.T) {
		testRound2AlertsResponse(t, &GetV1NodegroupsResponse{}, ParseGetV1NodegroupsResponse, []int{400, 401, 403, 499, 500, 503},
			func(rsp *GetV1NodegroupsResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: rsp.GetJSON400(), 401: rsp.GetJSON401(), 403: rsp.GetJSON403(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500(), 503: rsp.GetJSON503()}
			}, `{"nodeGroups":[{"id":"group-1","name":"Alpha","computeZoneId":"zone-1","nodesCount":7}],"page":2,"pageSize":50,"total":101,"hasMore":true}`,
			func(t *testing.T, result *GetV1Nodegroups200JSONResponseBody) {
				t.Helper()
				if result == nil {
					t.Fatal("GetJSON200() = nil, want node groups")
				}
				groups, err := result.AsModelsNodeGroupsResponse()
				if err != nil {
					t.Fatalf("AsModelsNodeGroupsResponse() error = %v", err)
				}
				if groups.NodeGroups == nil || len(*groups.NodeGroups) != 1 {
					t.Fatalf("node groups = %+v, want one group", groups)
				}
				group := (*groups.NodeGroups)[0]
				if group.Id != "group-1" || group.Name == nil || *group.Name != "Alpha" || group.ComputeZoneId == nil || *group.ComputeZoneId != "zone-1" || group.NodesCount == nil || *group.NodesCount != 7 || groups.Page == nil || *groups.Page != 2 || groups.PageSize == nil || *groups.PageSize != 50 || groups.Total == nil || *groups.Total != 101 || groups.HasMore == nil || !*groups.HasMore {
					t.Errorf("node groups = %+v, group = %+v; want group-1, Alpha, zone-1, 7 nodes, page 2, page size 50, total 101 and hasMore true", groups, group)
				}
			})
	})
	t.Run("nodes", func(t *testing.T) {
		testRound2AlertsResponse(t, &GetV1NodesResponse{}, ParseGetV1NodesResponse, []int{400, 401, 403, 499, 500, 503},
			func(rsp *GetV1NodesResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: rsp.GetJSON400(), 401: rsp.GetJSON401(), 403: rsp.GetJSON403(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500(), 503: rsp.GetJSON503()}
			}, `{"nodes":[{"nodeUUID":"node-inband","dataSource":"inband","agentUUID":"agent-1","hostname":"host-1","gpuCount":0}],"page":2,"pageSize":50,"total":101,"hasMore":true}`,
			func(t *testing.T, result *GetV1Nodes200JSONResponseBody) {
				t.Helper()
				if result == nil {
					t.Fatal("GetJSON200() = nil, want nodes")
				}
				nodes, err := result.AsModelsInbandNodesResponse()
				if err != nil {
					t.Fatalf("AsModelsInbandNodesResponse() error = %v", err)
				}
				if nodes.Nodes == nil || len(*nodes.Nodes) != 1 {
					t.Fatalf("nodes = %+v, want one node", nodes)
				}
				node := (*nodes.Nodes)[0]
				if node.NodeUUID != "node-inband" || node.DataSource != "inband" || node.AgentUUID == nil || *node.AgentUUID != "agent-1" || node.Hostname == nil || *node.Hostname != "host-1" || node.GpuCount == nil || *node.GpuCount != 0 || nodes.Page == nil || *nodes.Page != 2 || nodes.PageSize == nil || *nodes.PageSize != 50 || nodes.Total == nil || *nodes.Total != 101 || nodes.HasMore == nil || !*nodes.HasMore {
					t.Errorf("nodes = %+v, node = %+v; want node-inband, inband, agent-1, host-1, 0 GPUs, page 2, page size 50, total 101 and hasMore true", nodes, node)
				}
			})
	})
}

func TestRound2AuthAndComputeZoneResponseContracts(t *testing.T) {
	t.Run("get_computezones", func(t *testing.T) {
		testRound2AlertsResponse(t, &GetV1ComputezonesResponse{}, ParseGetV1ComputezonesResponse, []int{400, 401, 403, 499, 500, 503},
			func(rsp *GetV1ComputezonesResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: rsp.GetJSON400(), 401: rsp.GetJSON401(), 403: rsp.GetJSON403(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500(), 503: rsp.GetJSON503()}
			}, `{"computezones":[{"id":"zone-1","name":"Alpha"}],"page":2,"pageSize":50,"total":101,"hasMore":true}`,
			func(t *testing.T, result *GetV1Computezones200JSONResponseBody) {
				t.Helper()
				if result == nil {
					t.Fatal("GetJSON200() = nil, want compute zones")
				}
				zones, err := result.AsModelsComputeZonesResponse()
				if err != nil {
					t.Fatalf("AsModelsComputeZonesResponse() error = %v", err)
				}
				if zones.Computezones == nil || len(*zones.Computezones) != 1 {
					t.Fatalf("compute zones = %+v, want one zone", zones)
				}
				zone := (*zones.Computezones)[0]
				if zone.Id != "zone-1" || zone.Name == nil || *zone.Name != "Alpha" || zones.Page == nil || *zones.Page != 2 || zones.PageSize == nil || *zones.PageSize != 50 || zones.Total == nil || *zones.Total != 101 || zones.HasMore == nil || !*zones.HasMore {
					t.Errorf("compute zones = %+v, zone = %+v; want zone-1, Alpha, page 2, page size 50, total 101 and hasMore true", zones, zone)
				}
			})
	})
	t.Run("put_computezones", func(t *testing.T) {
		testRound2AlertsResponse(t, &PutV1ComputezonesResponse{}, ParsePutV1ComputezonesResponse, []int{400, 401, 403, 499, 500},
			func(rsp *PutV1ComputezonesResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: rsp.GetJSON400(), 401: rsp.GetJSON401(), 403: rsp.GetJSON403(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500()}
			}, `{"id":"550e8400-e29b-41d4-a716-446655440000"}`,
			func(t *testing.T, result *ModelsUpdateComputeZoneResponse) {
				t.Helper()
				if result == nil || result.Id != "550e8400-e29b-41d4-a716-446655440000" {
					t.Errorf("GetJSON200() = %+v, want update ID 550e8400-e29b-41d4-a716-446655440000", result)
				}
			})
	})
	t.Run("auth_status", func(t *testing.T) {
		const jsonType = "application/json; charset=utf-8"
		makeResponse := func(status int, contentType string, body io.ReadCloser) *http.Response {
			return &http.Response{Status: fmt.Sprintf("%d supplied status", status), StatusCode: status,
				Header: http.Header{"Content-Type": []string{contentType}}, Body: body}
		}
		errorsFor := func(rsp *GetV1AuthStatusResponse) map[int]*ModelsErrorResponse {
			return map[int]*ModelsErrorResponse{401: rsp.GetJSON401(), 403: rsp.GetJSON403(), 499: rsp.GetJSON499()}
		}
		zero := &GetV1AuthStatusResponse{}
		assertRound2ResponseMetadata(t, zero, nil, "", 0, "")
		assertRound2ResponsePointer(t, "GetJSON200", zero.GetJSON200(), nil)
		assertRound2AlertTimelineError(t, 0, errorsFor(zero))

		t.Run("success", func(t *testing.T) {
			body := []byte(`{"authenticated":true}`)
			rsp, err := ParseGetV1AuthStatusResponse(makeResponse(200, jsonType, io.NopCloser(bytes.NewReader(body))))
			if err != nil || rsp == nil {
				t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
			}
			assertRound2ResponseMetadata(t, rsp, body, "200 supplied status", 200, jsonType)
			assertRound2AlertTimelineError(t, 0, errorsFor(rsp))
			result := rsp.GetJSON200()
			if result == nil || result.Authenticated == nil || !*result.Authenticated {
				t.Errorf("GetJSON200() = %+v, want authenticated true", result)
			}
		})
		body := []byte(`{"error":"denied","details":"specific detail"}`)
		for _, status := range []int{401, 403, 499} {
			t.Run(fmt.Sprint(status), func(t *testing.T) {
				rsp, err := ParseGetV1AuthStatusResponse(makeResponse(status, jsonType, io.NopCloser(bytes.NewReader(body))))
				if err != nil || rsp == nil {
					t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
				}
				assertRound2ResponseMetadata(t, rsp, body, fmt.Sprintf("%d supplied status", status), status, jsonType)
				assertRound2ResponsePointer(t, "GetJSON200", rsp.GetJSON200(), nil)
				assertRound2AlertTimelineError(t, status, errorsFor(rsp))
			})
		}
		for _, status := range []int{200, 401, 403, 499} {
			t.Run(fmt.Sprintf("malformed_%d", status), func(t *testing.T) {
				rsp, err := ParseGetV1AuthStatusResponse(makeResponse(status, jsonType, io.NopCloser(bytes.NewBufferString(`{"error":`))))
				if err == nil || rsp != nil {
					t.Errorf("parse = %v, %v; want nil response and decoding error", rsp, err)
				}
			})
		}
		t.Run("read_error", func(t *testing.T) {
			sentinel := errors.New("auth status read error")
			reader := &round2AlertTimelineErrorReader{err: sentinel}
			rsp, err := ParseGetV1AuthStatusResponse(makeResponse(403, jsonType, reader))
			if !errors.Is(err, sentinel) || rsp != nil || !reader.closed {
				t.Errorf("parse = %v, %v; closed = %t; want nil response, sentinel error and closed body", rsp, err, reader.closed)
			}
		})
		t.Run("text_plain", func(t *testing.T) {
			rsp, err := ParseGetV1AuthStatusResponse(makeResponse(403, "text/plain", io.NopCloser(bytes.NewReader(body))))
			if err != nil || rsp == nil {
				t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
			}
			assertRound2ResponseMetadata(t, rsp, body, "403 supplied status", 403, "text/plain")
			assertRound2ResponsePointer(t, "GetJSON200", rsp.GetJSON200(), nil)
			assertRound2AlertTimelineError(t, 0, errorsFor(rsp))
		})
	})
}

func TestRound2EventsOptionsAndInfoResponseContracts(t *testing.T) {
	t.Run("events", func(t *testing.T) {
		testRound2AlertsResponse(t, &GetV1EventsResponse{}, ParseGetV1EventsResponse, []int{400, 401, 403, 499, 500, 503},
			func(rsp *GetV1EventsResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: rsp.GetJSON400(), 401: rsp.GetJSON401(), 403: rsp.GetJSON403(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500(), 503: rsp.GetJSON503()}
			}, `{"events":[{"eventId":"event-1","nodeUUID":"node-1","name":"overheat"}],"page":0,"total":42}`,
			func(t *testing.T, result *ModelsEventsResponse) {
				t.Helper()
				if result == nil || result.Events == nil || len(*result.Events) != 1 {
					t.Fatalf("GetJSON200() = %+v; want one event", result)
				}
				event := (*result.Events)[0]
				if event.EventId != "event-1" || event.NodeUUID != "node-1" || event.Name == nil || *event.Name != "overheat" || result.Page == nil || *result.Page != 0 || result.Total == nil || *result.Total != 42 {
					t.Errorf("GetJSON200() = %+v, event = %+v; want event-1, node-1, overheat, page 0 and total 42", result, event)
				}
			})
	})
	t.Run("info", func(t *testing.T) {
		testRound2AlertsResponse(t, &GetV1InfoResponse{}, ParseGetV1InfoResponse, []int{400, 499, 500},
			func(rsp *GetV1InfoResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: rsp.GetJSON400(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500()}
			}, `{"components":{"apiBackend":{"version":"api-1.2","build":"api-build-1"}}}`,
			func(t *testing.T, result *ModelsInfoResponse) {
				t.Helper()
				if result == nil || result.Components == nil || result.Components.ApiBackend == nil {
					t.Fatalf("GetJSON200() = %+v; want API backend", result)
				}
				backend := result.Components.ApiBackend
				if backend.Version == nil || *backend.Version != "api-1.2" || backend.Build == nil || *backend.Build != "api-build-1" {
					t.Errorf("API backend = %+v; want version api-1.2 and build api-build-1", backend)
				}
			})
	})
	t.Run("options", func(t *testing.T) {
		const jsonType = "application/json"
		makeResponse := func(status int, contentType string, body io.ReadCloser) *http.Response {
			return &http.Response{Status: fmt.Sprintf("%d supplied status", status), StatusCode: status,
				Header: http.Header{"Content-Type": []string{contentType}}, Body: body}
		}
		errorsFor := func(rsp *GetV1EventsOptionsResponse) map[int]*ModelsErrorResponse {
			return map[int]*ModelsErrorResponse{401: rsp.GetJSON401(), 403: rsp.GetJSON403(), 500: rsp.GetJSON500(), 503: rsp.GetJSON503()}
		}
		zero := &GetV1EventsOptionsResponse{}
		assertRound2ResponseMetadata(t, zero, nil, "", 0, "")
		assertRound2ResponsePointer(t, "GetJSON200", zero.GetJSON200(), nil)
		assertRound2AlertTimelineError(t, 0, errorsFor(zero))

		body := []byte(`{"error":"denied","details":"specific detail"}`)
		for _, status := range []int{401, 403, 500, 503} {
			t.Run(fmt.Sprint(status), func(t *testing.T) {
				rsp, err := ParseGetV1EventsOptionsResponse(makeResponse(status, jsonType, io.NopCloser(bytes.NewReader(body))))
				if err != nil || rsp == nil {
					t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
				}
				assertRound2ResponseMetadata(t, rsp, body, fmt.Sprintf("%d supplied status", status), status, jsonType)
				assertRound2ResponsePointer(t, "GetJSON200", rsp.GetJSON200(), nil)
				assertRound2AlertTimelineError(t, status, errorsFor(rsp))
			})
		}
		t.Run("success", func(t *testing.T) {
			body := []byte(`{"eventNames":["XID","Others"],"dataSources":["inband","oob"]}`)
			rsp, err := ParseGetV1EventsOptionsResponse(makeResponse(200, jsonType, io.NopCloser(bytes.NewReader(body))))
			if err != nil || rsp == nil {
				t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
			}
			assertRound2ResponseMetadata(t, rsp, body, "200 supplied status", 200, jsonType)
			assertRound2AlertTimelineError(t, 0, errorsFor(rsp))
			result := rsp.GetJSON200()
			if result == nil || result.EventNames == nil || result.DataSources == nil {
				t.Fatalf("GetJSON200() = %+v; want event names and data sources", result)
			}
			if !reflect.DeepEqual(*result.EventNames, []string{"XID", "Others"}) || !reflect.DeepEqual(*result.DataSources, []string{"inband", "oob"}) {
				t.Errorf("GetJSON200() = %+v; want event names [XID Others] and data sources [inband oob]", result)
			}
		})
		t.Run("malformed", func(t *testing.T) {
			rsp, err := ParseGetV1EventsOptionsResponse(makeResponse(401, jsonType, io.NopCloser(bytes.NewBufferString(`{"error":`))))
			if err == nil || rsp != nil {
				t.Errorf("parse = %v, %v; want nil response and decoding error", rsp, err)
			}
		})
		t.Run("read_error", func(t *testing.T) {
			sentinel := errors.New("options read error")
			reader := &round2AlertTimelineErrorReader{err: sentinel}
			rsp, err := ParseGetV1EventsOptionsResponse(makeResponse(401, jsonType, reader))
			if !errors.Is(err, sentinel) || rsp != nil || !reader.closed {
				t.Errorf("parse = %v, %v; closed = %t; want nil response, sentinel error and closed body", rsp, err, reader.closed)
			}
		})
		t.Run("text_plain", func(t *testing.T) {
			rsp, err := ParseGetV1EventsOptionsResponse(makeResponse(401, "text/plain", io.NopCloser(bytes.NewReader(body))))
			if err != nil || rsp == nil {
				t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
			}
			assertRound2ResponseMetadata(t, rsp, body, "401 supplied status", 401, "text/plain")
			assertRound2ResponsePointer(t, "GetJSON200", rsp.GetJSON200(), nil)
			assertRound2AlertTimelineError(t, 0, errorsFor(rsp))
		})
	})
}

func testRound2MetricsResponse[T any, R interface {
	comparable
	round2ResponseMetadata
	GetJSON200() *T
}](t *testing.T, zero R, parse func(*http.Response) (R, error), statuses []int, supportedStatus int, errorsFor func(R) map[int]*ModelsErrorResponse, successBody string, checkSuccess func(*testing.T, *T)) {
	t.Helper()
	const jsonType = "application/json; charset=utf-8"
	makeResponse := func(status int, contentType string, body io.ReadCloser) *http.Response {
		return &http.Response{Status: fmt.Sprintf("%d supplied status", status), StatusCode: status,
			Header: http.Header{"Content-Type": []string{contentType}}, Body: body}
	}
	assertRound2ResponseMetadata(t, zero, nil, "", 0, "")
	assertRound2ResponsePointer(t, "GetJSON200", zero.GetJSON200(), nil)
	assertRound2AlertTimelineError(t, 0, errorsFor(zero))

	body := []byte(`{"error":"denied","details":"specific detail"}`)
	for _, status := range statuses {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			rsp, err := parse(makeResponse(status, jsonType, io.NopCloser(bytes.NewReader(body))))
			var nilResponse R
			if err != nil || rsp == nilResponse {
				t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
			}
			assertRound2ResponseMetadata(t, rsp, body, fmt.Sprintf("%d supplied status", status), status, jsonType)
			assertRound2ResponsePointer(t, "GetJSON200", rsp.GetJSON200(), nil)
			assertRound2AlertTimelineError(t, status, errorsFor(rsp))
		})
	}
	t.Run("success", func(t *testing.T) {
		body := []byte(successBody)
		rsp, err := parse(makeResponse(200, jsonType, io.NopCloser(bytes.NewReader(body))))
		var nilResponse R
		if err != nil || rsp == nilResponse {
			t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
		}
		assertRound2ResponseMetadata(t, rsp, body, "200 supplied status", 200, jsonType)
		assertRound2AlertTimelineError(t, 0, errorsFor(rsp))
		checkSuccess(t, rsp.GetJSON200())
	})
	t.Run("malformed", func(t *testing.T) {
		rsp, err := parse(makeResponse(supportedStatus, jsonType, io.NopCloser(bytes.NewBufferString(`{"error":`))))
		var nilResponse R
		if err == nil || rsp != nilResponse {
			t.Errorf("parse = %v, %v; want nil response and decoding error", rsp, err)
		}
	})
	t.Run("read_error", func(t *testing.T) {
		sentinel := errors.New("metrics read error")
		reader := &round2AlertTimelineErrorReader{err: sentinel}
		rsp, err := parse(makeResponse(supportedStatus, jsonType, reader))
		var nilResponse R
		if !errors.Is(err, sentinel) || rsp != nilResponse || !reader.closed {
			t.Errorf("parse = %v, %v; closed = %t; want nil response, sentinel error and closed body", rsp, err, reader.closed)
		}
	})
	t.Run("text_plain", func(t *testing.T) {
		rsp, err := parse(makeResponse(supportedStatus, "text/plain", io.NopCloser(bytes.NewReader(body))))
		var nilResponse R
		if err != nil || rsp == nilResponse {
			t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
		}
		assertRound2ResponseMetadata(t, rsp, body, fmt.Sprintf("%d supplied status", supportedStatus), supportedStatus, "text/plain")
		assertRound2ResponsePointer(t, "GetJSON200", rsp.GetJSON200(), nil)
		assertRound2AlertTimelineError(t, 0, errorsFor(rsp))
	})
}

func TestRound2MetricsResponseContracts(t *testing.T) {
	t.Run("GetV1MetricsResponse", func(t *testing.T) {
		testRound2MetricsResponse(t, &GetV1MetricsResponse{}, ParseGetV1MetricsResponse, []int{401, 403, 499, 500}, 401,
			func(rsp *GetV1MetricsResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{401: rsp.GetJSON401(), 403: rsp.GetJSON403(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500()}
			}, `{"components":[{"name":"bmc"}]}`, func(t *testing.T, result *ModelsGetMetricsResponse) {
				t.Helper()
				if result == nil || result.Components == nil || len(*result.Components) != 1 {
					t.Fatalf("GetJSON200() = %+v; want one component", result)
				}
				component := (*result.Components)[0]
				if component.Name == nil || *component.Name != "bmc" {
					t.Errorf("component name = %v; want bmc", component.Name)
				}
			})
	})
	t.Run("GetV1MetricsQueryRangeResponse", func(t *testing.T) {
		testRound2AlertsResponse(t, &GetV1MetricsQueryRangeResponse{}, ParseGetV1MetricsQueryRangeResponse, []int{400, 401, 403, 404, 499, 500, 503},
			func(rsp *GetV1MetricsQueryRangeResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: rsp.GetJSON400(), 401: rsp.GetJSON401(), 403: rsp.GetJSON403(), 404: rsp.GetJSON404(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500(), 503: rsp.GetJSON503()}
			}, `{"result":[],"step":"5m"}`, func(t *testing.T, result *ModelsQueryMetricRangeResponse) {
				t.Helper()
				if result == nil || result.Result == nil || len(*result.Result) != 0 || result.Step == nil || *result.Step != "5m" {
					t.Errorf("GetJSON200() = %+v; want empty result and step 5m", result)
				}
			})
	})
	t.Run("GetV2MetricsResponse", func(t *testing.T) {
		testRound2MetricsResponse(t, &GetV2MetricsResponse{}, ParseGetV2MetricsResponse, []int{499}, 499,
			func(rsp *GetV2MetricsResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{499: rsp.GetJSON499()}
			}, `{"step":"5m","metrics":[{"name":"gpu_util","aggregation":"average","result":[]}]}`, func(t *testing.T, result *ModelsMetricsAggregationResponse) {
				t.Helper()
				if result == nil || result.Step == nil || *result.Step != "5m" || result.Metrics == nil || len(*result.Metrics) != 1 {
					t.Fatalf("GetJSON200() = %+v; want step 5m and one metric", result)
				}
				metric := (*result.Metrics)[0]
				if metric.Name == nil || *metric.Name != "gpu_util" || metric.Aggregation == nil || *metric.Aggregation != AggregationAverage || metric.Result == nil || len(*metric.Result) != 0 {
					t.Errorf("metric = %+v; want gpu_util, average and empty result", metric)
				}
			})
	})
}

func TestRound2NodeOptionsHistoryMembershipResponseContracts(t *testing.T) {
	t.Run("GetV1NodegroupsOptionsResponse", func(t *testing.T) {
		testRound2MetricsResponse(t, &GetV1NodegroupsOptionsResponse{}, ParseGetV1NodegroupsOptionsResponse, []int{401, 403, 499, 500}, 401,
			func(rsp *GetV1NodegroupsOptionsResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{401: rsp.GetJSON401(), 403: rsp.GetJSON403(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500()}
			}, `{"filters":{"fields":[{"name":"computeZones","options":[{"id":"cz-1","value":"East"}]}]}}`, func(t *testing.T, result *ModelsOptions) {
				t.Helper()
				if result == nil || result.Filters == nil || result.Filters.Fields == nil || len(*result.Filters.Fields) != 1 {
					t.Fatalf("GetJSON200() = %+v; want one filter field", result)
				}
				field := (*result.Filters.Fields)[0]
				if field.Name == nil || *field.Name != "computeZones" || field.Options == nil || len(*field.Options) != 1 {
					t.Fatalf("filter field = %+v; want computeZones and one option", field)
				}
				option := (*field.Options)[0]
				if option["id"] != "cz-1" || option["value"] != "East" {
					t.Errorf("option = %+v; want id cz-1 and value East", option)
				}
			})
	})
	t.Run("GetV1NodesOptionsResponse", func(t *testing.T) {
		testRound2MetricsResponse(t, &GetV1NodesOptionsResponse{}, ParseGetV1NodesOptionsResponse, []int{400, 401, 403, 499, 500}, 400,
			func(rsp *GetV1NodesOptionsResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: rsp.GetJSON400(), 401: rsp.GetJSON401(), 403: rsp.GetJSON403(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500()}
			}, `{"sorting":{"fields":["name","health"],"orders":["asc","desc"],"defaults":{"field":"name","order":"asc"}}}`, func(t *testing.T, result *ModelsOptions) {
				t.Helper()
				if result == nil || result.Sorting == nil {
					t.Fatalf("GetJSON200() = %+v; want sorting options", result)
				}
				sorting := result.Sorting
				if sorting.Fields == nil || !reflect.DeepEqual(*sorting.Fields, []string{"name", "health"}) || sorting.Orders == nil || !reflect.DeepEqual(*sorting.Orders, []string{"asc", "desc"}) || sorting.Defaults == nil || sorting.Defaults.Field == nil || *sorting.Defaults.Field != "name" || sorting.Defaults.Order == nil || *sorting.Defaults.Order != "asc" {
					t.Errorf("sorting = %+v; want fields name, health; orders asc, desc; defaults name, asc", sorting)
				}
			})
	})
	t.Run("GetV1NodeGroupMembershipsResponse", func(t *testing.T) {
		testRound2MetricsResponse(t, &GetV1NodeGroupMembershipsResponse{}, ParseGetV1NodeGroupMembershipsResponse, []int{400, 401, 403, 499, 500, 503}, 400,
			func(rsp *GetV1NodeGroupMembershipsResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: rsp.GetJSON400(), 401: rsp.GetJSON401(), 403: rsp.GetJSON403(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500(), 503: rsp.GetJSON503()}
			}, `{"memberships":[{"nodeGroupId":"group-1","nodeUUID":"node-1"}],"page":0,"pageSize":20,"total":21,"hasMore":true}`, func(t *testing.T, result *ModelsNodeGroupMembershipsResponse) {
				t.Helper()
				if result == nil || result.Memberships == nil || len(*result.Memberships) != 1 {
					t.Fatalf("GetJSON200() = %+v; want one membership", result)
				}
				membership := (*result.Memberships)[0]
				if membership.NodeGroupId == nil || *membership.NodeGroupId != "group-1" || membership.NodeUUID == nil || *membership.NodeUUID != "node-1" {
					t.Errorf("membership = %+v; want group-1 and node-1", membership)
				}
				if result.Page == nil || *result.Page != 0 || result.PageSize == nil || *result.PageSize != 20 || result.Total == nil || *result.Total != 21 || result.HasMore == nil || !*result.HasMore {
					t.Errorf("pagination = %+v; want page 0, pageSize 20, total 21, hasMore true", result)
				}
			})
	})
	t.Run("GetV1NodesNodeUuidHealthHistoryResponse", func(t *testing.T) {
		testRound2MetricsResponse(t, &GetV1NodesNodeUuidHealthHistoryResponse{}, ParseGetV1NodesNodeUuidHealthHistoryResponse, []int{400, 401, 404, 499, 500}, 400,
			func(rsp *GetV1NodesNodeUuidHealthHistoryResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: rsp.GetJSON400(), 401: rsp.GetJSON401(), 404: rsp.GetJSON404(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500()}
			}, `{"enrolledAt":"2026-04-01T00:00:00Z","healthSummary":{"healthyPercentage":99.5,"degradedDurationSeconds":3000}}`, func(t *testing.T, result *ModelsNodeHealthHistoryResponse) {
				t.Helper()
				if result == nil || result.EnrolledAt == nil || *result.EnrolledAt != "2026-04-01T00:00:00Z" || result.HealthSummary == nil || result.HealthSummary.HealthyPercentage == nil || *result.HealthSummary.HealthyPercentage != 99.5 || result.HealthSummary.DegradedDurationSeconds == nil || *result.HealthSummary.DegradedDurationSeconds != 3000 {
					t.Errorf("GetJSON200() = %+v; want enrolledAt and health summary values", result)
				}
			})
	})
}

func testRound2PolicyPutResponse[R interface {
	comparable
	round2ResponseMetadata
}](t *testing.T, zero R, parse func(*http.Response) (R, error), statuses []int, errorsFor func(R) map[int]*ModelsErrorResponse) {
	t.Helper()
	const jsonType = "application/json; charset=utf-8"
	makeResponse := func(status int, contentType string, body io.ReadCloser) *http.Response {
		return &http.Response{Status: fmt.Sprintf("%d supplied status", status), StatusCode: status,
			Header: http.Header{"Content-Type": []string{contentType}}, Body: body}
	}
	assertRound2ResponseMetadata(t, zero, nil, "", 0, "")
	assertRound2AlertTimelineError(t, 0, errorsFor(zero))
	body := []byte(`{"error":"denied","details":"specific detail"}`)
	for _, status := range statuses {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			rsp, err := parse(makeResponse(status, jsonType, io.NopCloser(bytes.NewReader(body))))
			var nilResponse R
			if err != nil || rsp == nilResponse {
				t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
			}
			assertRound2ResponseMetadata(t, rsp, body, fmt.Sprintf("%d supplied status", status), status, jsonType)
			assertRound2AlertTimelineError(t, status, errorsFor(rsp))
		})
	}
	t.Run("no_content", func(t *testing.T) {
		rsp, err := parse(makeResponse(204, "", io.NopCloser(bytes.NewReader(nil))))
		var nilResponse R
		if err != nil || rsp == nilResponse {
			t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
		}
		assertRound2ResponseMetadata(t, rsp, []byte{}, "204 supplied status", 204, "")
		assertRound2AlertTimelineError(t, 0, errorsFor(rsp))
	})
	t.Run("text_plain", func(t *testing.T) {
		status := statuses[0]
		rsp, err := parse(makeResponse(status, "text/plain", io.NopCloser(bytes.NewReader(body))))
		var nilResponse R
		if err != nil || rsp == nilResponse {
			t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
		}
		assertRound2ResponseMetadata(t, rsp, body, fmt.Sprintf("%d supplied status", status), status, "text/plain")
		assertRound2AlertTimelineError(t, 0, errorsFor(rsp))
	})
	t.Run("malformed", func(t *testing.T) {
		rsp, err := parse(makeResponse(statuses[0], jsonType, io.NopCloser(bytes.NewBufferString(`{"error":`))))
		var nilResponse R
		if err == nil || rsp != nilResponse {
			t.Errorf("parse = %v, %v; want nil response and decoding error", rsp, err)
		}
	})
	t.Run("read_error", func(t *testing.T) {
		sentinel := errors.New("policy read error")
		reader := &round2AlertTimelineErrorReader{err: sentinel}
		rsp, err := parse(makeResponse(statuses[0], jsonType, reader))
		var nilResponse R
		if err != sentinel || rsp != nilResponse || !reader.closed {
			t.Errorf("parse = %v, %v; closed = %t; want nil response, original error and closed body", rsp, err, reader.closed)
		}
	})
}

func TestRound2PolicyResponseContracts(t *testing.T) {
	t.Run("GetV1NodeRemovalPolicy", func(t *testing.T) {
		testRound2MetricsResponse(t, &GetV1NodeRemovalPolicyResponse{}, ParseGetV1NodeRemovalPolicyResponse, []int{401, 403, 500, 503}, 401,
			func(rsp *GetV1NodeRemovalPolicyResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{401: rsp.GetJSON401(), 403: rsp.GetJSON403(), 500: rsp.GetJSON500(), 503: rsp.GetJSON503()}
			}, `{"enabled":true,"offlineThresholdDays":7,"lastCheckedAt":"2024-01-15T10:30:00Z","updatedBy":"user@example.com"}`, func(t *testing.T, result *ModelsNodeRemovalPolicy) {
				t.Helper()
				if result == nil || result.Enabled == nil || !*result.Enabled || result.OfflineThresholdDays == nil || *result.OfflineThresholdDays != 7 || result.LastCheckedAt == nil || *result.LastCheckedAt != "2024-01-15T10:30:00Z" || result.UpdatedBy == nil || *result.UpdatedBy != "user@example.com" {
					t.Errorf("GetJSON200() = %+v; want exact node-removal policy fields", result)
				}
			})
	})
	t.Run("GetV1RetentionPolicy", func(t *testing.T) {
		testRound2MetricsResponse(t, &GetV1RetentionPolicyResponse{}, ParseGetV1RetentionPolicyResponse, []int{401, 403, 404, 499, 500, 503}, 401,
			func(rsp *GetV1RetentionPolicyResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{401: rsp.GetJSON401(), 403: rsp.GetJSON403(), 404: rsp.GetJSON404(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500(), 503: rsp.GetJSON503()}
			}, `{"retentionDays":90,"createdAt":"2026-01-01T00:00:00Z","updatedBy":"alice"}`, func(t *testing.T, result *ModelsGetRetentionPolicyResponse) {
				t.Helper()
				if result == nil || result.RetentionDays == nil || *result.RetentionDays != 90 || result.CreatedAt == nil || *result.CreatedAt != "2026-01-01T00:00:00Z" || result.UpdatedBy == nil || *result.UpdatedBy != "alice" {
					t.Errorf("GetJSON200() = %+v; want exact retention policy fields", result)
				}
			})
	})
	t.Run("PutV1NodeRemovalPolicy", func(t *testing.T) {
		testRound2PolicyPutResponse(t, &PutV1NodeRemovalPolicyResponse{}, ParsePutV1NodeRemovalPolicyResponse, []int{400, 401, 403, 500, 503},
			func(rsp *PutV1NodeRemovalPolicyResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: rsp.GetJSON400(), 401: rsp.GetJSON401(), 403: rsp.GetJSON403(), 500: rsp.GetJSON500(), 503: rsp.GetJSON503()}
			})
	})
	t.Run("PutV1RetentionPolicy", func(t *testing.T) {
		testRound2PolicyPutResponse(t, &PutV1RetentionPolicyResponse{}, ParsePutV1RetentionPolicyResponse, []int{400, 401, 403, 499, 500, 503},
			func(rsp *PutV1RetentionPolicyResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: rsp.GetJSON400(), 401: rsp.GetJSON401(), 403: rsp.GetJSON403(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500(), 503: rsp.GetJSON503()}
			})
	})
}

func TestRound2RankingsAndCommandsResponseContracts(t *testing.T) {
	t.Run("GetV1PredictedHealthRankingsResponse", func(t *testing.T) {
		testRound2MetricsResponse(t, &GetV1PredictedHealthRankingsResponse{}, ParseGetV1PredictedHealthRankingsResponse, []int{400, 401, 403, 499, 500, 501, 503}, 400,
			func(rsp *GetV1PredictedHealthRankingsResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: rsp.GetJSON400(), 401: rsp.GetJSON401(), 403: rsp.GetJSON403(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500(), 501: rsp.GetJSON501(), 503: rsp.GetJSON503()}
			}, `{"rankUnit":"rack","complete":true,"items":[],"total":2,"rankedCount":1,"unrankedCount":1,"unmappedNodeCount":0,"page":1,"pageSize":2,"hasMore":false}`, func(t *testing.T, result *ModelsPredictedHealthRankingsResponse) {
				t.Helper()
				if result == nil || result.RankUnit == nil || *result.RankUnit != ModelsPredictedHealthRankingsResponseRankUnitRack || result.Complete == nil || !*result.Complete || result.Items == nil || len(*result.Items) != 0 || result.Total == nil || *result.Total != 2 || result.RankedCount == nil || *result.RankedCount != 1 || result.UnrankedCount == nil || *result.UnrankedCount != 1 || result.UnmappedNodeCount == nil || *result.UnmappedNodeCount != 0 || result.Page == nil || *result.Page != 1 || result.PageSize == nil || *result.PageSize != 2 || result.HasMore == nil || *result.HasMore {
					t.Errorf("GetJSON200() = %+v; want exact ranking fields and empty items", result)
				}
			})
	})
	t.Run("GetV2CommandsResponse", func(t *testing.T) {
		testRound2MetricsResponse(t, &GetV2CommandsResponse{}, ParseGetV2CommandsResponse, []int{400, 499, 500}, 400,
			func(rsp *GetV2CommandsResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: rsp.GetJSON400(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500()}
			}, `{"commands":[{"name":"inspect","link":"/commands/inspect","template":"inspect {node}","placeholders":["node"]}]}`, func(t *testing.T, result *ModelsCommandsResponse) {
				t.Helper()
				if result == nil || result.Commands == nil || len(*result.Commands) != 1 {
					t.Fatalf("GetJSON200() = %+v; want one command", result)
				}
				command := (*result.Commands)[0]
				if command.Name == nil || *command.Name != "inspect" || command.Link == nil || *command.Link != "/commands/inspect" || command.Template == nil || *command.Template != "inspect {node}" || command.Placeholders == nil || len(*command.Placeholders) != 1 || (*command.Placeholders)[0] != "node" {
					t.Errorf("command = %+v; want exact inspect command fields", command)
				}
			})
	})
}

func TestRound2TagResponseContracts(t *testing.T) {
	t.Run("GetV1TagsResponse", func(t *testing.T) {
		testRound2MetricsResponse(t, &GetV1TagsResponse{}, ParseGetV1TagsResponse, []int{400, 401, 403, 499, 500}, 400,
			func(rsp *GetV1TagsResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: rsp.GetJSON400(), 401: rsp.GetJSON401(), 403: rsp.GetJSON403(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500()}
			}, `{"tags":["rack-blue","production"]}`, func(t *testing.T, result *ModelsListTagsResponse) {
				t.Helper()
				if result == nil || result.Tags == nil || len(*result.Tags) != 2 || (*result.Tags)[0] != "rack-blue" || (*result.Tags)[1] != "production" {
					t.Errorf("GetJSON200() = %+v; want tags [rack-blue production]", result)
				}
			})
	})
	t.Run("PutV1NodesNodeUuidTagsResponse", func(t *testing.T) {
		testRound2MetricsResponse(t, &PutV1NodesNodeUuidTagsResponse{}, ParsePutV1NodesNodeUuidTagsResponse, []int{400, 404, 499, 500}, 400,
			func(rsp *PutV1NodesNodeUuidTagsResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: rsp.GetJSON400(), 404: rsp.GetJSON404(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500()}
			}, `{"nodeUUID":"node-1","tags":["rack-blue","production"]}`, func(t *testing.T, result *ModelsSetNodeTagsResponse) {
				t.Helper()
				if result == nil || result.NodeUUID == nil || *result.NodeUUID != "node-1" || result.Tags == nil || len(*result.Tags) != 2 || (*result.Tags)[0] != "rack-blue" || (*result.Tags)[1] != "production" {
					t.Errorf("GetJSON200() = %+v; want node-1 and tags [rack-blue production]", result)
				}
			})
	})
	t.Run("PatchV1TagsResponse", func(t *testing.T) {
		testRound2PolicyPutResponse(t, &PatchV1TagsResponse{}, ParsePatchV1TagsResponse, []int{400, 401, 403, 499, 500},
			func(rsp *PatchV1TagsResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: rsp.GetJSON400(), 401: rsp.GetJSON401(), 403: rsp.GetJSON403(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500()}
			})
	})
}

func TestRound2NotificationResponseContracts(t *testing.T) {
	t.Run("GetV1NotificationDeliveryLogResponse", func(t *testing.T) {
		testRound2MetricsResponse(t, &GetV1NotificationDeliveryLogResponse{}, ParseGetV1NotificationDeliveryLogResponse, []int{400, 401, 403, 499, 500, 503}, 400,
			func(rsp *GetV1NotificationDeliveryLogResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: rsp.GetJSON400(), 401: rsp.GetJSON401(), 403: rsp.GetJSON403(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500(), 503: rsp.GetJSON503()}
			}, `{"deliveryLog":[{"alertUUID":"alert-123","component":"GPU"}],"page":0,"pageSize":20,"total":1,"hasMore":false}`, func(t *testing.T, result *ModelsDeliveryLogResponse) {
				t.Helper()
				if result == nil || result.DeliveryLog == nil || len(*result.DeliveryLog) != 1 || (*result.DeliveryLog)[0].AlertUUID == nil || *(*result.DeliveryLog)[0].AlertUUID != "alert-123" || (*result.DeliveryLog)[0].Component == nil || *(*result.DeliveryLog)[0].Component != "GPU" || result.Page == nil || *result.Page != 0 || result.PageSize == nil || *result.PageSize != 20 || result.Total == nil || *result.Total != 1 || result.HasMore == nil || *result.HasMore {
					t.Errorf("GetJSON200() = %+v; want delivery log and pagination values", result)
				}
			})
	})
	t.Run("GetV1NotificationPreferencesResponse", func(t *testing.T) {
		testRound2MetricsResponse(t, &GetV1NotificationPreferencesResponse{}, ParseGetV1NotificationPreferencesResponse, []int{401, 403, 499, 500, 503}, 401,
			func(rsp *GetV1NotificationPreferencesResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{401: rsp.GetJSON401(), 403: rsp.GetJSON403(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500(), 503: rsp.GetJSON503()}
			}, `{"preferences":[{"id":"preference-1","components":["GPU"]}],"total":1,"componentPreferenceCounts":{"GPU":1}}`, func(t *testing.T, result *ModelsListNotificationPreferencesResponse) {
				t.Helper()
				if result == nil || result.Preferences == nil || len(*result.Preferences) != 1 || (*result.Preferences)[0].Id == nil || *(*result.Preferences)[0].Id != "preference-1" || (*result.Preferences)[0].Components == nil || !reflect.DeepEqual(*(*result.Preferences)[0].Components, []string{"GPU"}) || result.Total == nil || *result.Total != 1 || result.ComponentPreferenceCounts == nil || !reflect.DeepEqual(*result.ComponentPreferenceCounts, map[string]int{"GPU": 1}) {
					t.Errorf("GetJSON200() = %+v; want preference, total and component counts", result)
				}
			})
	})
	t.Run("GetV1NotificationPreferenceByIDResponse", func(t *testing.T) {
		testRound2MetricsResponse(t, &GetV1NotificationPreferenceByIDResponse{}, ParseGetV1NotificationPreferenceByIDResponse, []int{401, 403, 404, 499, 500, 503}, 401,
			func(rsp *GetV1NotificationPreferenceByIDResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{401: rsp.GetJSON401(), 403: rsp.GetJSON403(), 404: rsp.GetJSON404(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500(), 503: rsp.GetJSON503()}
			}, `{"id":"preference-1","components":["GPU"],"active":false}`, func(t *testing.T, result *ModelsNotificationPreferenceDetail) {
				t.Helper()
				if result == nil || result.Id == nil || *result.Id != "preference-1" || result.Components == nil || !reflect.DeepEqual(*result.Components, []string{"GPU"}) || result.Active == nil || *result.Active {
					t.Errorf("GetJSON200() = %+v; want preference-1, [GPU], inactive", result)
				}
			})
	})
	t.Run("DeleteV1NotificationPreferenceByIDResponse", func(t *testing.T) {
		testRound2PolicyPutResponse(t, &DeleteV1NotificationPreferenceByIDResponse{}, ParseDeleteV1NotificationPreferenceByIDResponse, []int{400, 401, 403, 404, 499, 500, 503},
			func(rsp *DeleteV1NotificationPreferenceByIDResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: rsp.GetJSON400(), 401: rsp.GetJSON401(), 403: rsp.GetJSON403(), 404: rsp.GetJSON404(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500(), 503: rsp.GetJSON503()}
			})
	})
	t.Run("PutV1NotificationPreferenceByIDResponse", func(t *testing.T) {
		testRound2PolicyPutResponse(t, &PutV1NotificationPreferenceByIDResponse{}, ParsePutV1NotificationPreferenceByIDResponse, []int{400, 401, 403, 404, 499, 500, 503},
			func(rsp *PutV1NotificationPreferenceByIDResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: rsp.GetJSON400(), 401: rsp.GetJSON401(), 403: rsp.GetJSON403(), 404: rsp.GetJSON404(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500(), 503: rsp.GetJSON503()}
			})
	})
	t.Run("PostV1NotificationPreferencesResponse", func(t *testing.T) {
		const jsonType = "application/json; charset=utf-8"
		makeResponse := func(status int, contentType string, body io.ReadCloser) *http.Response {
			return &http.Response{Status: fmt.Sprintf("%d supplied status", status), StatusCode: status,
				Header: http.Header{"Content-Type": []string{contentType}}, Body: body}
		}
		errorsFor := func(rsp *PostV1NotificationPreferencesResponse) map[int]*ModelsErrorResponse {
			return map[int]*ModelsErrorResponse{400: rsp.GetJSON400(), 401: rsp.GetJSON401(), 403: rsp.GetJSON403(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500(), 503: rsp.GetJSON503()}
		}
		zero := &PostV1NotificationPreferencesResponse{}
		assertRound2ResponseMetadata(t, zero, nil, "", 0, "")
		assertRound2ResponsePointer(t, "GetJSON201", zero.GetJSON201(), nil)
		assertRound2AlertTimelineError(t, 0, errorsFor(zero))
		t.Run("success", func(t *testing.T) {
			body := []byte(`{"id":"preference-1"}`)
			rsp, err := ParsePostV1NotificationPreferencesResponse(makeResponse(201, jsonType, io.NopCloser(bytes.NewReader(body))))
			if err != nil || rsp == nil {
				t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
			}
			assertRound2ResponseMetadata(t, rsp, body, "201 supplied status", 201, jsonType)
			assertRound2AlertTimelineError(t, 0, errorsFor(rsp))
			if result := rsp.GetJSON201(); result == nil || result.Id != "preference-1" {
				t.Errorf("GetJSON201() = %+v; want id preference-1", result)
			}
		})
		body := []byte(`{"error":"denied","details":"specific detail"}`)
		for _, status := range []int{400, 401, 403, 499, 500, 503} {
			t.Run(fmt.Sprint(status), func(t *testing.T) {
				rsp, err := ParsePostV1NotificationPreferencesResponse(makeResponse(status, jsonType, io.NopCloser(bytes.NewReader(body))))
				if err != nil || rsp == nil {
					t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
				}
				assertRound2ResponseMetadata(t, rsp, body, fmt.Sprintf("%d supplied status", status), status, jsonType)
				assertRound2ResponsePointer(t, "GetJSON201", rsp.GetJSON201(), nil)
				assertRound2AlertTimelineError(t, status, errorsFor(rsp))
			})
		}
		t.Run("text_plain", func(t *testing.T) {
			rsp, err := ParsePostV1NotificationPreferencesResponse(makeResponse(400, "text/plain", io.NopCloser(bytes.NewReader(body))))
			if err != nil || rsp == nil {
				t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
			}
			assertRound2ResponseMetadata(t, rsp, body, "400 supplied status", 400, "text/plain")
			assertRound2ResponsePointer(t, "GetJSON201", rsp.GetJSON201(), nil)
			assertRound2AlertTimelineError(t, 0, errorsFor(rsp))
		})
		t.Run("malformed", func(t *testing.T) {
			rsp, err := ParsePostV1NotificationPreferencesResponse(makeResponse(201, jsonType, io.NopCloser(bytes.NewBufferString(`{"id":`))))
			if err == nil || rsp != nil {
				t.Errorf("parse = %v, %v; want nil response and decoding error", rsp, err)
			}
		})
		t.Run("read_error", func(t *testing.T) {
			sentinel := errors.New("notification read error")
			reader := &round2AlertTimelineErrorReader{err: sentinel}
			rsp, err := ParsePostV1NotificationPreferencesResponse(makeResponse(201, jsonType, reader))
			if !errors.Is(err, sentinel) || rsp != nil || !reader.closed {
				t.Errorf("parse = %v, %v; closed = %t; want nil response, sentinel error and closed body", rsp, err, reader.closed)
			}
		})
	})
}

func TestRound2StatsSecurityOptionsAndNodeDeletionResponseContracts(t *testing.T) {
	statuses := []int{400, 401, 403, 404, 499, 500, 503}
	t.Run("stats", func(t *testing.T) {
		testRound2MetricsResponse(t, &GetV1StatsResponse{}, ParseGetV1StatsResponse, statuses, 400,
			func(r *GetV1StatsResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: r.GetJSON400(), 401: r.GetJSON401(), 403: r.GetJSON403(), 404: r.GetJSON404(), 499: r.GetJSON499(), 500: r.GetJSON500(), 503: r.GetJSON503()}
			}, `{"timestamps":[1719489600,1719576000,1719662400],"series":[{"name":"Online","values":[7,null,0]}]}`, func(t *testing.T, result *ModelsStatsResponse) {
				t.Helper()
				if result == nil || result.Timestamps == nil || !reflect.DeepEqual(*result.Timestamps, []int{1719489600, 1719576000, 1719662400}) || result.Series == nil || len(*result.Series) != 1 {
					t.Fatalf("stats = %+v; want exact timestamps and one series", result)
				}
				series := (*result.Series)[0]
				seven, zero := 7, 0
				if series.Name == nil || *series.Name != "Online" || series.Values == nil || !reflect.DeepEqual(*series.Values, []*int{&seven, nil, &zero}) {
					t.Errorf("series = %+v; want Online with values [7, null, 0]", series)
				}
			})
	})
	t.Run("security_options", func(t *testing.T) {
		testRound2MetricsResponse(t, &GetV1ReportsSecurityVulnerabilityOptionsResponse{}, ParseGetV1ReportsSecurityVulnerabilityOptionsResponse, []int{400, 401, 403, 422, 499, 500, 503}, 400,
			func(r *GetV1ReportsSecurityVulnerabilityOptionsResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: r.GetJSON400(), 401: r.GetJSON401(), 403: r.GetJSON403(), 422: r.GetJSON422(), 499: r.GetJSON499(), 500: r.GetJSON500(), 503: r.GetJSON503()}
			}, `{"filters":{"computeZoneIds":[{"id":"zone-1","name":"East"}],"nodeGroupIds":[{"id":"group-1","name":"Training"}],"componentTypes":[{"id":"gpuDriver","name":"GPU Driver"},{"id":"vbios","name":"VBIOS"}],"gpuTypes":["H100","A100"],"severities":["CRITICAL","HIGH"]},"sorting":{"fields":["vulnerability","affectedNodes"],"orders":["asc","desc"],"defaults":{"field":"affectedNodes","order":"desc"}}}`, func(t *testing.T, result *ModelsSecurityVulnerabilityOptionsResponse) {
				t.Helper()
				if result == nil {
					t.Fatal("GetJSON200() = nil")
				}
				want := ModelsSecurityVulnerabilityFilterOptions{
					ComputeZoneIds: []ModelsIDNameOption{{Id: "zone-1", Name: "East"}},
					NodeGroupIds:   []ModelsIDNameOption{{Id: "group-1", Name: "Training"}},
					ComponentTypes: []ModelsIDNameOption{{Id: "gpuDriver", Name: "GPU Driver"}, {Id: "vbios", Name: "VBIOS"}},
					GpuTypes:       []string{"H100", "A100"}, Severities: []ModelsSecurityVulnerabilitySeverity{SecurityVulnerabilitySeverityCritical, SecurityVulnerabilitySeverityHigh},
				}
				if !reflect.DeepEqual(result.Filters, want) {
					t.Errorf("filters = %+v; want %+v", result.Filters, want)
				}
				s := result.Sorting
				if s.Fields == nil || !reflect.DeepEqual(*s.Fields, []string{"vulnerability", "affectedNodes"}) || s.Orders == nil || !reflect.DeepEqual(*s.Orders, []string{"asc", "desc"}) || s.Defaults == nil || s.Defaults.Field == nil || *s.Defaults.Field != "affectedNodes" || s.Defaults.Order == nil || *s.Defaults.Order != "desc" {
					t.Errorf("sorting = %+v; want exact fields, orders and defaults", s)
				}
			})
	})
	checkDeletion := func(t *testing.T, result *map[string]interface{}) {
		t.Helper()
		want := map[string]interface{}{"message": "accepted", "count": float64(2)}
		if result == nil || !reflect.DeepEqual(*result, want) {
			t.Errorf("GetJSON200() = %v; want %v", result, want)
		}
	}
	const deletionBody = `{"message":"accepted","count":2}`
	t.Run("delete_nodes", func(t *testing.T) {
		testRound2MetricsResponse(t, &DeleteV1NodesResponse{}, ParseDeleteV1NodesResponse, statuses, 400,
			func(r *DeleteV1NodesResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: r.GetJSON400(), 401: r.GetJSON401(), 403: r.GetJSON403(), 404: r.GetJSON404(), 499: r.GetJSON499(), 500: r.GetJSON500(), 503: r.GetJSON503()}
			}, deletionBody, checkDeletion)
	})
	t.Run("delete_node", func(t *testing.T) {
		testRound2MetricsResponse(t, &DeleteV1NodesNodeUuidResponse{}, ParseDeleteV1NodesNodeUuidResponse, statuses, 400,
			func(r *DeleteV1NodesNodeUuidResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: r.GetJSON400(), 401: r.GetJSON401(), 403: r.GetJSON403(), 404: r.GetJSON404(), 499: r.GetJSON499(), 500: r.GetJSON500(), 503: r.GetJSON503()}
			}, deletionBody, checkDeletion)
	})
}

func TestRound2XIDBurstsAndNodeRemovalHistoryResponseContracts(t *testing.T) {
	t.Run("GetV1XIDBurstsResponse", func(t *testing.T) {
		testRound2MetricsResponse(t, &GetV1XIDBurstsResponse{}, ParseGetV1XIDBurstsResponse, []int{400, 401, 403, 499, 500}, 400,
			func(r *GetV1XIDBurstsResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: r.GetJSON400(), 401: r.GetJSON401(), 403: r.GetJSON403(), 499: r.GetJSON499(), 500: r.GetJSON500()}
			}, `{"items":[{"burstId":"burst-1","nodeUuid":"node-1"}],"page":2,"pageSize":25,"total":42}`, func(t *testing.T, result *ModelsXIDBurstsListResponse) {
				t.Helper()
				if result == nil || result.Items == nil || len(*result.Items) != 1 {
					t.Fatalf("GetJSON200() = %+v; want one item", result)
				}
				item := (*result.Items)[0]
				if item.BurstId != "burst-1" || item.NodeUuid != "node-1" {
					t.Errorf("item = %+v; want burst-1 and node-1", item)
				}
				if result.Page == nil || *result.Page != 2 || result.PageSize == nil || *result.PageSize != 25 || result.Total == nil || *result.Total != 42 {
					t.Errorf("pagination = %+v; want page 2, pageSize 25, total 42", result)
				}
			})
	})
	t.Run("GetV1XIDBurstOptionsResponse", func(t *testing.T) {
		testRound2MetricsResponse(t, &GetV1XIDBurstOptionsResponse{}, ParseGetV1XIDBurstOptionsResponse, []int{400, 401, 403, 499, 500}, 400,
			func(r *GetV1XIDBurstOptionsResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: r.GetJSON400(), 401: r.GetJSON401(), 403: r.GetJSON403(), 499: r.GetJSON499(), 500: r.GetJSON500()}
			}, `{"dataSource":"all","gpuTypes":["H100","A100"],"xidNumbers":[31,79],"jobDisruption":[true,false]}`, func(t *testing.T, result *ModelsXIDBurstFilterOptionsResponse) {
				t.Helper()
				if result == nil || result.DataSource == nil || *result.DataSource != "all" || result.GpuTypes == nil || !reflect.DeepEqual(*result.GpuTypes, []string{"H100", "A100"}) || result.XidNumbers == nil || !reflect.DeepEqual(*result.XidNumbers, []int{31, 79}) || result.JobDisruption == nil || !reflect.DeepEqual(*result.JobDisruption, []bool{true, false}) {
					t.Errorf("GetJSON200() = %+v; want exact source, GPU types, XID numbers and disruption options", result)
				}
			})
	})
	t.Run("GetV1XIDBurstDetailResponse", func(t *testing.T) {
		testRound2MetricsResponse(t, &GetV1XIDBurstDetailResponse{}, ParseGetV1XIDBurstDetailResponse, []int{400, 401, 403, 404, 499, 500}, 400,
			func(r *GetV1XIDBurstDetailResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: r.GetJSON400(), 401: r.GetJSON401(), 403: r.GetJSON403(), 404: r.GetJSON404(), 499: r.GetJSON499(), 500: r.GetJSON500()}
			}, `{"burstId":"burst-1","nodeUuid":"node-1","dataSource":"oob","deviceIds":{"0000:01:00.0":[31,79]},"jobDisruption":true}`, func(t *testing.T, result *ModelsXIDBurstDetail) {
				t.Helper()
				if result == nil || result.BurstId != "burst-1" || result.NodeUuid != "node-1" || result.DataSource == nil || *result.DataSource != "oob" || result.DeviceIds == nil || !reflect.DeepEqual(*result.DeviceIds, map[string][]int{"0000:01:00.0": {31, 79}}) || result.JobDisruption == nil || !*result.JobDisruption {
					t.Errorf("GetJSON200() = %+v; want exact IDs, source, device map and disruption", result)
				}
			})
	})
	t.Run("GetV1NodesHistoryResponse", func(t *testing.T) {
		testRound2MetricsResponse(t, &GetV1NodesHistoryResponse{}, ParseGetV1NodesHistoryResponse, []int{400, 401, 403, 500, 503}, 400,
			func(r *GetV1NodesHistoryResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: r.GetJSON400(), 401: r.GetJSON401(), 403: r.GetJSON403(), 500: r.GetJSON500(), 503: r.GetJSON503()}
			}, `{"history":[{"nodeUuid":"node-1","deletedBy":"alice","actorType":"user","hostname":"node-a"}],"page":0,"pageSize":2,"total":3,"hasMore":true}`, func(t *testing.T, result *ModelsNodeHistoryResponse) {
				t.Helper()
				if result == nil || result.History == nil || len(*result.History) != 1 {
					t.Fatalf("GetJSON200() = %+v; want one history entry", result)
				}
				entry := (*result.History)[0]
				if entry.NodeUuid == nil || *entry.NodeUuid != "node-1" || entry.DeletedBy == nil || *entry.DeletedBy != "alice" || entry.ActorType == nil || *entry.ActorType != "user" || entry.Hostname == nil || *entry.Hostname != "node-a" {
					t.Errorf("history entry = %+v; want node-1, alice, user and node-a", entry)
				}
				if result.Page == nil || *result.Page != 0 || result.PageSize == nil || *result.PageSize != 2 || result.Total == nil || *result.Total != 3 || result.HasMore == nil || !*result.HasMore {
					t.Errorf("pagination = %+v; want page 0, pageSize 2, total 3, hasMore true", result)
				}
			})
	})
}

func TestRound2XIDBurstFilterRequestContracts(t *testing.T) {
	const server = "https://example.test/"
	start, end := "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z"
	node := "11111111-1111-4111-8111-111111111111"
	groups := []string{"22222222-2222-4222-8222-222222222222", "33333333-3333-4333-8333-333333333333"}
	zones := []string{"44444444-4444-4444-8444-444444444444"}
	excludedGroups := []string{"55555555-5555-4555-8555-555555555555", "66666666-6666-4666-8666-666666666666"}
	excludedZones := []string{"77777777-7777-4777-8777-777777777777", "88888888-8888-4888-8888-888888888888"}
	disruption := true
	hostname, category, subcategory := "rack & gpu", "hardware", "memory"
	tenantAction, tenantInvestigation := "CONTACT", "CHECK"
	dcAction, dcInvestigation := "REPLACE", "INSPECT"
	categories := []string{"hardware", "thermal"}
	subcategories := []string{"memory", "power"}
	tenantActions := []string{"CONTACT_SUPPORT", "REBOOT"}
	tenantInvestigations := []string{"CHECK_LOGS", "INSPECT"}
	dcActions := []string{"REPLACE", "REPAIR"}
	dcInvestigations := []string{"INSPECT", "DIAGNOSE"}

	check := func(t *testing.T, req *http.Request, path string, want url.Values) {
		t.Helper()
		if req.Method != http.MethodGet || req.URL.Path != path {
			t.Errorf("request = %s %s, want GET %s", req.Method, req.URL.Path, path)
		}
		got, err := url.ParseQuery(req.URL.RawQuery)
		if err != nil {
			t.Fatalf("parse query: %v", err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("query = %#v, want %#v", got, want)
		}
	}

	t.Run("list_inclusions", func(t *testing.T) {
		req, err := NewGetV1XIDBurstsRequest(server, &GetV1XIDBurstsParams{
			StartTime: &start, EndTime: &end, NodeUUID: &node, NodeGroupIds: &groups, ComputeZoneIds: &zones,
			JobDisruptionDueToPlatformIssue: &disruption, HostnameSearch: &hostname,
			CategorySearch: &category, SubcategorySearch: &subcategory, TenantActionSearch: &tenantAction,
			TenantInvestigationSearch: &tenantInvestigation, DcAdminActionSearch: &dcAction, DcAdminInvestigationSearch: &dcInvestigation,
			Categories: &categories, Subcategories: &subcategories, TenantActions: &tenantActions,
			TenantInvestigations: &tenantInvestigations, DcAdminActions: &dcActions, DcAdminInvestigations: &dcInvestigations,
		})
		if err != nil {
			t.Fatalf("construct list request: %v", err)
		}
		check(t, req, "/v1/xid/bursts", url.Values{
			"startTime": {"2026-01-01T00:00:00Z"}, "endTime": {"2026-01-02T00:00:00Z"},
			"nodeUUID":                        {"11111111-1111-4111-8111-111111111111"},
			"nodeGroupIds":                    {"22222222-2222-4222-8222-222222222222", "33333333-3333-4333-8333-333333333333"},
			"computeZoneIds":                  {"44444444-4444-4444-8444-444444444444"},
			"jobDisruptionDueToPlatformIssue": {"true"}, "hostnameSearch": {"rack & gpu"},
			"categorySearch": {"hardware"}, "subcategorySearch": {"memory"},
			"tenantActionSearch": {"CONTACT"}, "tenantInvestigationSearch": {"CHECK"},
			"dcAdminActionSearch": {"REPLACE"}, "dcAdminInvestigationSearch": {"INSPECT"},
			"categories": {"hardware", "thermal"}, "subcategories": {"memory", "power"},
			"tenantActions": {"CONTACT_SUPPORT", "REBOOT"}, "tenantInvestigations": {"CHECK_LOGS", "INSPECT"},
			"dcAdminActions": {"REPLACE", "REPAIR"}, "dcAdminInvestigations": {"INSPECT", "DIAGNOSE"},
		})
	})

	excludedQuery := url.Values{
		"startTime": {"2026-01-01T00:00:00Z"}, "endTime": {"2026-01-02T00:00:00Z"},
		"excludeNodeGroupIds":   {"55555555-5555-4555-8555-555555555555", "66666666-6666-4666-8666-666666666666"},
		"excludeComputeZoneIds": {"77777777-7777-4777-8777-777777777777", "88888888-8888-4888-8888-888888888888"},
	}
	t.Run("list_exclusions", func(t *testing.T) {
		req, err := NewGetV1XIDBurstsRequest(server, &GetV1XIDBurstsParams{
			StartTime: &start, EndTime: &end, ExcludeNodeGroupIds: &excludedGroups, ExcludeComputeZoneIds: &excludedZones,
		})
		if err != nil {
			t.Fatalf("construct list request: %v", err)
		}
		check(t, req, "/v1/xid/bursts", excludedQuery)
	})
	t.Run("options_inclusions", func(t *testing.T) {
		req, err := NewGetV1XIDBurstOptionsRequest(server, &GetV1XIDBurstOptionsParams{
			StartTime: &start, EndTime: &end, NodeGroupIds: &groups, ComputeZoneIds: &zones,
		})
		if err != nil {
			t.Fatalf("construct options request: %v", err)
		}
		check(t, req, "/v1/xid/bursts/options", url.Values{
			"startTime": {"2026-01-01T00:00:00Z"}, "endTime": {"2026-01-02T00:00:00Z"},
			"nodeGroupIds":   {"22222222-2222-4222-8222-222222222222", "33333333-3333-4333-8333-333333333333"},
			"computeZoneIds": {"44444444-4444-4444-8444-444444444444"},
		})
	})
	t.Run("options_exclusions", func(t *testing.T) {
		req, err := NewGetV1XIDBurstOptionsRequest(server, &GetV1XIDBurstOptionsParams{
			StartTime: &start, EndTime: &end, ExcludeNodeGroupIds: &excludedGroups, ExcludeComputeZoneIds: &excludedZones,
		})
		if err != nil {
			t.Fatalf("construct options request: %v", err)
		}
		check(t, req, "/v1/xid/bursts/options", excludedQuery)
	})
}

func TestRound2RawPolicyAndNodeRequests(t *testing.T) {
	const contentType = "application/json; charset=utf-8"
	const node = "11111111-1111-4111-8111-111111111111"
	dataSource := DeleteV1NodesParamsDataSource("oob")
	agentType := DeleteV1NodesParamsAgentType("oob")
	params := &DeleteV1NodesParams{DataSource: &dataSource, AgentType: &agentType}
	tests := []struct {
		name, method, path, body, responseBody string
		query                                  url.Values
		status                                 int
		call                                   func(*ClientWithResponses, RequestEditorFn) (interface{}, error)
		construct                              func() (*http.Request, error)
	}{
		{
			name: "node_removal_policy", method: http.MethodPut, path: "/v1/node_removal_policy",
			body: `{"enabled":true,"offlineThresholdDays":14}`, status: http.StatusNoContent,
			call: func(c *ClientWithResponses, editor RequestEditorFn) (interface{}, error) {
				return c.PutV1NodeRemovalPolicyWithBodyWithResponse(context.Background(), contentType, bytes.NewBufferString(`{"enabled":true,"offlineThresholdDays":14}`), editor)
			},
			construct: func() (*http.Request, error) {
				return NewPutV1NodeRemovalPolicyRequestWithBody("http://[::1", contentType, bytes.NewReader(nil))
			},
		},
		{
			name: "retention_policy", method: http.MethodPut, path: "/v1/retention_policy",
			body: `{"retentionDays":365}`, status: http.StatusNoContent,
			call: func(c *ClientWithResponses, editor RequestEditorFn) (interface{}, error) {
				return c.PutV1RetentionPolicyWithBodyWithResponse(context.Background(), contentType, bytes.NewBufferString(`{"retentionDays":365}`), editor)
			},
			construct: func() (*http.Request, error) {
				return NewPutV1RetentionPolicyRequestWithBody("http://[::1", contentType, bytes.NewReader(nil))
			},
		},
		{
			name: "delete_nodes", method: http.MethodDelete, path: "/v1/nodes",
			body: `{"nodeUUIDs":["11111111-1111-4111-8111-111111111111"]}`, responseBody: `{"message":"accepted","count":1}`,
			query: url.Values{"dataSource": {"oob"}, "agentType": {"oob"}}, status: http.StatusOK,
			call: func(c *ClientWithResponses, editor RequestEditorFn) (interface{}, error) {
				return c.DeleteV1NodesWithBodyWithResponse(context.Background(), params, contentType, bytes.NewBufferString(`{"nodeUUIDs":["11111111-1111-4111-8111-111111111111"]}`), editor)
			},
			construct: func() (*http.Request, error) {
				return NewDeleteV1NodesRequestWithBody("http://[::1", params, contentType, bytes.NewReader(nil))
			},
		},
		{
			name: "node_tags", method: http.MethodPut, path: "/v1/nodes/" + node + "/tags",
			body: `{"tags":["gpu-health","node123"]}`, responseBody: `{"nodeUUID":"11111111-1111-4111-8111-111111111111","tags":["gpu-health","node123"]}`, status: http.StatusOK,
			call: func(c *ClientWithResponses, editor RequestEditorFn) (interface{}, error) {
				return c.PutV1NodesNodeUuidTagsWithBodyWithResponse(context.Background(), node, contentType, bytes.NewBufferString(`{"tags":["gpu-health","node123"]}`), editor)
			},
			construct: func() (*http.Request, error) {
				return NewPutV1NodesNodeUuidTagsRequestWithBody("http://[::1", node, contentType, bytes.NewReader(nil))
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			doer := authStatusTestDoer(func(req *http.Request) (*http.Response, error) {
				calls++
				body, err := io.ReadAll(req.Body)
				if err != nil {
					t.Fatalf("read request body: %v", err)
				}
				if req.Method != tc.method || req.URL.Path != tc.path || !bytes.Equal(body, []byte(tc.body)) || req.Header.Get("Content-Type") != contentType {
					t.Errorf("request = %s %s, body %q, content type %q; want %s %s, body %q, content type %q", req.Method, req.URL.Path, body, req.Header.Get("Content-Type"), tc.method, tc.path, tc.body, contentType)
				}
				query, err := url.ParseQuery(req.URL.RawQuery)
				if err != nil {
					t.Fatalf("parse query: %v", err)
				}
				wantQuery := tc.query
				if wantQuery == nil {
					wantQuery = url.Values{}
				}
				if !reflect.DeepEqual(query, wantQuery) {
					t.Errorf("query = %v, want %v", query, wantQuery)
				}
				return &http.Response{StatusCode: tc.status, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(bytes.NewBufferString(tc.responseBody))}, nil
			})
			client, err := NewClientWithResponses("http://example.test", WithHTTPClient(doer))
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			response, err := tc.call(client, func(_ context.Context, _ *http.Request) error { return nil })
			if err != nil || response == nil || reflect.ValueOf(response).IsNil() {
				t.Fatalf("response = %v, error = %v", response, err)
			}
			if calls != 1 {
				t.Errorf("transport calls = %d, want 1", calls)
			}
			metadata := response.(round2ResponseMetadata)
			if metadata.StatusCode() != tc.status || !bytes.Equal(metadata.GetBody(), []byte(tc.responseBody)) {
				t.Errorf("status = %d, body = %q; want %d, %q", metadata.StatusCode(), metadata.GetBody(), tc.status, tc.responseBody)
			}
			switch r := response.(type) {
			case *DeleteV1NodesResponse:
				want := map[string]interface{}{"message": "accepted", "count": float64(1)}
				if r.JSON200 == nil || !reflect.DeepEqual(*r.JSON200, want) {
					t.Errorf("decoded deletion = %v, want %v", r.JSON200, want)
				}
			case *PutV1NodesNodeUuidTagsResponse:
				if r.JSON200 == nil || r.JSON200.NodeUUID == nil || *r.JSON200.NodeUUID != node || r.JSON200.Tags == nil || !reflect.DeepEqual(*r.JSON200.Tags, []string{"gpu-health", "node123"}) {
					t.Errorf("decoded tags = %+v", r.JSON200)
				}
			}
			sentinel := errors.New("editor rejected request")
			calls = 0
			response, err = tc.call(client, func(_ context.Context, _ *http.Request) error { return sentinel })
			if !errors.Is(err, sentinel) || response == nil || !reflect.ValueOf(response).IsNil() || calls != 0 {
				t.Errorf("editor failure: response = %v, error = %v, calls = %d; want nil response, sentinel, zero calls", response, err, calls)
			}
			req, err := tc.construct()
			if err == nil || req != nil {
				t.Errorf("malformed URL: request = %v, error = %v; want nil request and error", req, err)
			}
		})
	}
}

func TestRound2RawMuteNotificationAndTagRequests(t *testing.T) {
	const contentType = "application/json; charset=utf-8"
	const id = "11111111-1111-4111-8111-111111111111"
	const muteBody = `{"component":"GPU","severities":["Warning"]}`
	const notificationBody = `{"components":["GPU"],"notificationTargets":[{"channel":"email","config":{"recipients":["ops@example.test"]}}],"notifiedOn":["Critical"]}`
	const tagBody = `{"nodeUUIDs":["11111111-1111-4111-8111-111111111111"],"add":["gpu-health"],"remove":["legacy"]}`
	tests := []struct {
		name, method, path, body, responseBody string
		status                                 int
		call                                   func(*ClientWithResponses, io.Reader, RequestEditorFn) (interface{}, error)
		construct                              func() (*http.Request, error)
	}{
		{
			name: "delete_mute_rules", method: http.MethodDelete, path: "/v1/alert_mute_rules", body: muteBody, status: http.StatusNoContent,
			call: func(c *ClientWithResponses, body io.Reader, editor RequestEditorFn) (interface{}, error) {
				return c.DeleteV1AlertMuteRulesWithBodyWithResponse(context.Background(), contentType, body, editor)
			},
			construct: func() (*http.Request, error) {
				return NewDeleteV1AlertMuteRulesRequestWithBody("http://[::1", contentType, bytes.NewReader(nil))
			},
		},
		{
			name: "put_mute_rules", method: http.MethodPut, path: "/v1/alert_mute_rules", body: muteBody, status: http.StatusNoContent,
			call: func(c *ClientWithResponses, body io.Reader, editor RequestEditorFn) (interface{}, error) {
				return c.PutV1AlertMuteRulesWithBodyWithResponse(context.Background(), contentType, body, editor)
			},
			construct: func() (*http.Request, error) {
				return NewPutV1AlertMuteRulesRequestWithBody("http://[::1", contentType, bytes.NewReader(nil))
			},
		},
		{
			name: "create_notification", method: http.MethodPost, path: "/v1/notification_preferences", body: notificationBody,
			status: http.StatusCreated, responseBody: `{"id":"11111111-1111-4111-8111-111111111111"}`,
			call: func(c *ClientWithResponses, body io.Reader, editor RequestEditorFn) (interface{}, error) {
				return c.PostV1NotificationPreferencesWithBodyWithResponse(context.Background(), contentType, body, editor)
			},
			construct: func() (*http.Request, error) {
				return NewPostV1NotificationPreferencesRequestWithBody("http://[::1", contentType, bytes.NewReader(nil))
			},
		},
		{
			name: "update_notification", method: http.MethodPut, path: "/v1/notification_preferences/" + id, body: notificationBody, status: http.StatusNoContent,
			call: func(c *ClientWithResponses, body io.Reader, editor RequestEditorFn) (interface{}, error) {
				return c.PutV1NotificationPreferenceByIDWithBodyWithResponse(context.Background(), id, contentType, body, editor)
			},
			construct: func() (*http.Request, error) {
				return NewPutV1NotificationPreferenceByIDRequestWithBody("http://[::1", id, contentType, bytes.NewReader(nil))
			},
		},
		{
			name: "patch_tags", method: http.MethodPatch, path: "/v1/tags", body: tagBody, status: http.StatusNoContent,
			call: func(c *ClientWithResponses, body io.Reader, editor RequestEditorFn) (interface{}, error) {
				return c.PatchV1TagsWithBodyWithResponse(context.Background(), contentType, body, editor)
			},
			construct: func() (*http.Request, error) {
				return NewPatchV1TagsRequestWithBody("http://[::1", contentType, bytes.NewReader(nil))
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			doer := authStatusTestDoer(func(req *http.Request) (*http.Response, error) {
				calls++
				body, err := io.ReadAll(req.Body)
				if err != nil {
					t.Fatalf("read request body: %v", err)
				}
				if req.Method != tc.method || req.URL.Path != tc.path || req.URL.RawQuery != "" || req.Header.Get("Content-Type") != contentType || !bytes.Equal(body, []byte(tc.body)) {
					t.Errorf("request = %s %s?%s, content type %q, body %q; want %s %s with empty query, content type %q, body %q", req.Method, req.URL.Path, req.URL.RawQuery, req.Header.Get("Content-Type"), body, tc.method, tc.path, contentType, tc.body)
				}
				return &http.Response{StatusCode: tc.status, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(bytes.NewBufferString(tc.responseBody))}, nil
			})
			client, err := NewClientWithResponses("http://example.test", WithHTTPClient(doer))
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			response, err := tc.call(client, bytes.NewBufferString(tc.body), func(_ context.Context, _ *http.Request) error { return nil })
			if err != nil || response == nil || reflect.ValueOf(response).IsNil() {
				t.Fatalf("response = %v, error = %v", response, err)
			}
			if calls != 1 {
				t.Errorf("transport calls = %d, want 1", calls)
			}
			metadata := response.(round2ResponseMetadata)
			if metadata.StatusCode() != tc.status || !bytes.Equal(metadata.GetBody(), []byte(tc.responseBody)) {
				t.Errorf("status = %d, body = %q; want %d, %q", metadata.StatusCode(), metadata.GetBody(), tc.status, tc.responseBody)
			}
			if tc.name == "create_notification" {
				created := response.(*PostV1NotificationPreferencesResponse)
				if created.JSON201 == nil || created.JSON201.Id != id {
					t.Errorf("decoded notification = %+v, want ID %q", created.JSON201, id)
				}
			}
			sentinel := errors.New("editor rejected request")
			calls = 0
			response, err = tc.call(client, bytes.NewBufferString(tc.body), func(_ context.Context, _ *http.Request) error { return sentinel })
			if !errors.Is(err, sentinel) || response == nil || !reflect.ValueOf(response).IsNil() || calls != 0 {
				t.Errorf("editor failure: response = %v, error = %v, calls = %d; want nil response, sentinel, zero calls", response, err, calls)
			}
			req, err := tc.construct()
			if err == nil || req != nil {
				t.Errorf("malformed URL: request = %v, error = %v; want nil request and error", req, err)
			}
		})
	}
}

func TestRound2GeneratedGetEditorFailures(t *testing.T) {
	sentinel := errors.New("editor rejected request")
	tests := []struct {
		name string
		path string
		call func(*ClientWithResponses, RequestEditorFn) (bool, error)
	}{
		{
			name: "computezones", path: "/v1/computezones",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1ComputezonesWithResponse(context.Background(), &GetV1ComputezonesParams{}, editor)
				return response == nil, err
			},
		},
		{
			name: "events", path: "/v1/events",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1EventsWithResponse(context.Background(), &GetV1EventsParams{}, editor)
				return response == nil, err
			},
		},
		{
			name: "nodegroups", path: "/v1/nodegroups",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1NodegroupsWithResponse(context.Background(), &GetV1NodegroupsParams{}, editor)
				return response == nil, err
			},
		},
		{
			name: "nodes", path: "/v1/nodes",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1NodesWithResponse(context.Background(), &GetV1NodesParams{}, editor)
				return response == nil, err
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			doer := authStatusTestDoer(func(_ *http.Request) (*http.Response, error) {
				calls++
				return nil, errors.New("unexpected transport call")
			})
			client, err := NewClientWithResponses("http://example.test", WithHTTPClient(doer))
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			editorCalls := 0
			responseNil, err := tc.call(client, func(_ context.Context, req *http.Request) error {
				editorCalls++
				if req.Method != http.MethodGet || req.URL.Path != tc.path {
					t.Errorf("request = %s %s, want GET %s", req.Method, req.URL.Path, tc.path)
				}
				return sentinel
			})
			if !errors.Is(err, sentinel) {
				t.Errorf("error = %v, want sentinel", err)
			}
			if !responseNil {
				t.Error("response is non-nil, want nil")
			}
			if editorCalls != 1 {
				t.Errorf("editor calls = %d, want 1", editorCalls)
			}
			if calls != 0 {
				t.Errorf("transport calls = %d, want 0", calls)
			}

			malformedClient, err := NewClientWithResponses("http://[::1", WithHTTPClient(doer))
			if err != nil {
				t.Fatalf("new malformed-server client: %v", err)
			}
			editorCalls, calls = 0, 0
			responseNil, err = tc.call(malformedClient, func(_ context.Context, _ *http.Request) error {
				editorCalls++
				return nil
			})
			if err == nil {
				t.Error("malformed server: expected URL construction error")
			}
			if !responseNil {
				t.Error("malformed server: response is non-nil, want nil")
			}
			if editorCalls != 0 || calls != 0 {
				t.Errorf("malformed server: editor calls = %d, transport calls = %d; want 0 and 0", editorCalls, calls)
			}
		})
	}
}

func TestRound2GeneratedReadRequestFailures(t *testing.T) {
	sentinel := errors.New("editor rejected request")
	tests := []struct {
		name string
		path string
		call func(*ClientWithResponses, RequestEditorFn) (bool, error)
	}{
		{
			name: "event buckets", path: "/v1/events/buckets",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				mode, window := GetV1EventsBucketsParamsTimeModeRelative, "24h"
				response, err := c.GetV1EventsBucketsWithResponse(context.Background(), &GetV1EventsBucketsParams{TimeMode: &mode, Window: &window}, editor)
				return response == nil, err
			},
		},
		{
			name: "metrics", path: "/v1/metrics",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1MetricsWithResponse(context.Background(), &GetV1MetricsParams{}, editor)
				return response == nil, err
			},
		},
		{
			name: "overview", path: "/v1/overview",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1OverviewWithResponse(context.Background(), &GetV1OverviewParams{}, editor)
				return response == nil, err
			},
		},
		{
			name: "inventory", path: "/v1/reports/inventory",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1ReportsInventoryWithResponse(context.Background(), &GetV1ReportsInventoryParams{}, editor)
				return response == nil, err
			},
		},
		{
			name: "burst detail", path: "/v1/xid/bursts/burst-1",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1XIDBurstDetailWithResponse(context.Background(), "burst-1", editor)
				return response == nil, err
			},
		},
		{
			name: "commands", path: "/v2/commands",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV2CommandsWithResponse(context.Background(), editor)
				return response == nil, err
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			transportCalls := 0
			doer := authStatusTestDoer(func(_ *http.Request) (*http.Response, error) {
				transportCalls++
				return nil, errors.New("unexpected transport call")
			})
			client, err := NewClientWithResponses("http://example.test", WithHTTPClient(doer))
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			editorCalls := 0
			responseNil, err := tc.call(client, func(_ context.Context, req *http.Request) error {
				editorCalls++
				if req.Method != http.MethodGet || req.URL.Path != tc.path {
					t.Errorf("request = %s %s, want GET %s", req.Method, req.URL.Path, tc.path)
				}
				return sentinel
			})
			if !errors.Is(err, sentinel) {
				t.Errorf("error = %v, want sentinel", err)
			}
			if !responseNil {
				t.Error("response is non-nil, want nil")
			}
			if editorCalls != 1 || transportCalls != 0 {
				t.Errorf("editor calls = %d, transport calls = %d; want 1 and 0", editorCalls, transportCalls)
			}

			malformedClient, err := NewClientWithResponses("http://[::1", WithHTTPClient(doer))
			if err != nil {
				t.Fatalf("new malformed-server client: %v", err)
			}
			editorCalls, transportCalls = 0, 0
			responseNil, err = tc.call(malformedClient, func(_ context.Context, _ *http.Request) error {
				editorCalls++
				return nil
			})
			if err == nil {
				t.Error("malformed server: expected URL construction error")
			}
			if !responseNil {
				t.Error("malformed server: response is non-nil, want nil")
			}
			if editorCalls != 0 || transportCalls != 0 {
				t.Errorf("malformed server: editor calls = %d, transport calls = %d; want 0 and 0", editorCalls, transportCalls)
			}
		})
	}
}

func TestRound2AlertRequestFailures(t *testing.T) {
	sentinel := errors.New("editor rejected request")
	tests := []struct {
		name string
		path string
		call func(*ClientWithResponses, RequestEditorFn) (bool, error)
	}{
		{
			name: "timeline filter options", path: "/v1/alert_timeline/filter_options",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1AlertTimelineFilterOptionsWithResponse(context.Background(), &GetV1AlertTimelineFilterOptionsParams{}, editor)
				return response == nil, err
			},
		},
		{
			name: "timeline nodes", path: "/v1/alert_timeline/nodes",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1AlertTimelineNodesWithResponse(context.Background(), &GetV1AlertTimelineNodesParams{}, editor)
				return response == nil, err
			},
		},
		{
			name: "timeline node alerts", path: "/v1/alert_timeline/nodes/node-1/alerts",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1AlertTimelineNodesNodeUuidAlertsWithResponse(context.Background(), "node-1", &GetV1AlertTimelineNodesNodeUuidAlertsParams{}, editor)
				return response == nil, err
			},
		},
		{
			name: "timeline node alert", path: "/v1/alert_timeline/nodes/node-1/alerts/alert-1",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1AlertTimelineNodesNodeUuidAlertsAlertUuidWithResponse(context.Background(), "node-1", "alert-1", &GetV1AlertTimelineNodesNodeUuidAlertsAlertUuidParams{}, editor)
				return response == nil, err
			},
		},
		{
			name: "alerts", path: "/v1/alerts",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1AlertsWithResponse(context.Background(), &GetV1AlertsParams{}, editor)
				return response == nil, err
			},
		},
		{
			name: "node alerts", path: "/v1/alerts/node-1",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1AlertsNodeUuidWithResponse(context.Background(), "node-1", &GetV1AlertsNodeUuidParams{}, editor)
				return response == nil, err
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			transportCalls := 0
			doer := authStatusTestDoer(func(_ *http.Request) (*http.Response, error) {
				transportCalls++
				return nil, errors.New("unexpected transport call")
			})
			client, err := NewClientWithResponses("http://example.test", WithHTTPClient(doer))
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			editorCalls := 0
			responseNil, err := tc.call(client, func(_ context.Context, req *http.Request) error {
				editorCalls++
				if req.Method != http.MethodGet || req.URL.Path != tc.path {
					t.Errorf("request = %s %s, want GET %s", req.Method, req.URL.Path, tc.path)
				}
				return sentinel
			})
			if !errors.Is(err, sentinel) {
				t.Errorf("error = %v, want sentinel", err)
			}
			if !responseNil {
				t.Error("response is non-nil, want nil")
			}
			if editorCalls != 1 || transportCalls != 0 {
				t.Errorf("editor calls = %d, transport calls = %d; want 1 and 0", editorCalls, transportCalls)
			}

			malformedClient, err := NewClientWithResponses("http://[::1", WithHTTPClient(doer))
			if err != nil {
				t.Fatalf("new malformed-server client: %v", err)
			}
			editorCalls, transportCalls = 0, 0
			responseNil, err = tc.call(malformedClient, func(_ context.Context, _ *http.Request) error {
				editorCalls++
				return nil
			})
			if err == nil {
				t.Error("malformed server: expected URL construction error")
			}
			if !responseNil {
				t.Error("malformed server: response is non-nil, want nil")
			}
			if editorCalls != 0 || transportCalls != 0 {
				t.Errorf("malformed server: editor calls = %d, transport calls = %d; want 0 and 0", editorCalls, transportCalls)
			}
		})
	}
}

func TestRound2NotificationReportReadRequestFailures(t *testing.T) {
	sentinel := errors.New("editor rejected request")
	tests := []struct {
		name   string
		method string
		path   string
		call   func(*ClientWithResponses, RequestEditorFn) (bool, error)
	}{
		{
			name: "notification delivery log", method: http.MethodGet, path: "/v1/notification_delivery_log",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1NotificationDeliveryLogWithResponse(context.Background(), &GetV1NotificationDeliveryLogParams{}, editor)
				return response == nil, err
			},
		},
		{
			name: "notification preferences", method: http.MethodGet, path: "/v1/notification_preferences",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1NotificationPreferencesWithResponse(context.Background(), editor)
				return response == nil, err
			},
		},
		{
			name: "notification preference", method: http.MethodGet, path: "/v1/notification_preferences/preference-1",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1NotificationPreferenceByIDWithResponse(context.Background(), "preference-1", editor)
				return response == nil, err
			},
		},
		{
			name: "delete notification preference", method: http.MethodDelete, path: "/v1/notification_preferences/preference-1",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.DeleteV1NotificationPreferenceByIDWithResponse(context.Background(), "preference-1", editor)
				return response == nil, err
			},
		},
		{
			name: "predicted health rankings", method: http.MethodGet, path: "/v1/predicted_health_rankings",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1PredictedHealthRankingsWithResponse(context.Background(), &GetV1PredictedHealthRankingsParams{}, editor)
				return response == nil, err
			},
		},
		{
			name: "error report", method: http.MethodGet, path: "/v1/reports/error",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1ReportsErrorWithResponse(context.Background(), &GetV1ReportsErrorParams{View: Overview}, editor)
				return response == nil, err
			},
		},
		{
			name: "security vulnerabilities", method: http.MethodGet, path: "/v1/reports/security/vulnerabilities",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1ReportsSecurityVulnerabilitiesWithResponse(context.Background(), &GetV1ReportsSecurityVulnerabilitiesParams{}, editor)
				return response == nil, err
			},
		},
		{
			name: "security vulnerability options", method: http.MethodGet, path: "/v1/reports/security/vulnerabilities/options",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1ReportsSecurityVulnerabilityOptionsWithResponse(context.Background(), editor)
				return response == nil, err
			},
		},
		{
			name: "security vulnerability nodes", method: http.MethodGet, path: "/v1/reports/security/vulnerabilities/CVE-2026-1234/nodes",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1ReportsSecurityVulnerabilityNodesWithResponse(context.Background(), "CVE-2026-1234", &GetV1ReportsSecurityVulnerabilityNodesParams{}, editor)
				return response == nil, err
			},
		},
		{
			name: "stats", method: http.MethodGet, path: "/v1/stats",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				dataSource := GetV1StatsParamsDataSourceInband
				response, err := c.GetV1StatsWithResponse(context.Background(), &GetV1StatsParams{Name: GetV1StatsParamsNameAgentStatus, DataSource: &dataSource, GroupBy: GetV1StatsParamsGroupByCustomer, Window: "72h"}, editor)
				return response == nil, err
			},
		},
		{
			name: "tags", method: http.MethodGet, path: "/v1/tags",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1TagsWithResponse(context.Background(), &GetV1TagsParams{}, editor)
				return response == nil, err
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			transportCalls := 0
			doer := authStatusTestDoer(func(_ *http.Request) (*http.Response, error) {
				transportCalls++
				return nil, errors.New("unexpected transport call")
			})
			client, err := NewClientWithResponses("http://example.test", WithHTTPClient(doer))
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			editorCalls := 0
			responseNil, err := tc.call(client, func(_ context.Context, req *http.Request) error {
				editorCalls++
				if req.Method != tc.method || req.URL.Path != tc.path {
					t.Errorf("request = %s %s, want %s %s", req.Method, req.URL.Path, tc.method, tc.path)
				}
				return sentinel
			})
			if !errors.Is(err, sentinel) {
				t.Errorf("error = %v, want sentinel", err)
			}
			if !responseNil {
				t.Error("response is non-nil, want nil")
			}
			if editorCalls != 1 || transportCalls != 0 {
				t.Errorf("editor calls = %d, transport calls = %d; want 1 and 0", editorCalls, transportCalls)
			}

			malformedClient, err := NewClientWithResponses("http://[::1", WithHTTPClient(doer))
			if err != nil {
				t.Fatalf("new malformed-server client: %v", err)
			}
			editorCalls, transportCalls = 0, 0
			responseNil, err = tc.call(malformedClient, func(_ context.Context, _ *http.Request) error {
				editorCalls++
				return nil
			})
			if err == nil {
				t.Error("malformed server: expected URL construction error")
			}
			if !responseNil {
				t.Error("malformed server: response is non-nil, want nil")
			}
			if editorCalls != 0 || transportCalls != 0 {
				t.Errorf("malformed server: editor calls = %d, transport calls = %d; want 0 and 0", editorCalls, transportCalls)
			}
		})
	}
}

func TestRound2TypedMutationRequestFailures(t *testing.T) {
	const nodeUUID = "11111111-1111-4111-8111-111111111111"
	const muteFixture = `{"component":"GPU","severities":["Warning"]}`
	const zoneFixture = `{"id":"cz-1"}`
	const removalFixture = `{"enabled":true,"offlineThresholdDays":14}`
	const nodesFixture = `{"nodeUUIDs":["11111111-1111-4111-8111-111111111111"]}`
	const nodeTagsFixture = `{"tags":["gpu-health"]}`
	const notificationFixture = `{"components":["GPU"],"notificationTargets":[{"channel":"email","config":{"recipients":["ops@example.test"]}}],"notifiedOn":["Critical"]}`
	const retentionFixture = `{"retentionDays":365}`
	const batchTagsFixture = `{"nodeUUIDs":["11111111-1111-4111-8111-111111111111"],"add":["gpu-health"]}`
	sentinel := errors.New("editor rejected request")
	tests := []struct {
		name, method, path, fixture string
		call                        func(*testing.T, *ClientWithResponses, RequestEditorFn) (bool, error)
	}{
		{
			name: "delete mute", method: http.MethodDelete, path: "/v1/alert_mute_rules", fixture: muteFixture,
			call: func(t *testing.T, c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				var body DeleteV1AlertMuteRulesJSONRequestBody
				if err := json.Unmarshal([]byte(muteFixture), &body); err != nil {
					t.Fatalf("decode fixture: %v", err)
				}
				response, err := c.DeleteV1AlertMuteRulesWithResponse(context.Background(), body, editor)
				return response == nil, err
			},
		},
		{
			name: "update mute", method: http.MethodPut, path: "/v1/alert_mute_rules", fixture: muteFixture,
			call: func(t *testing.T, c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				var body PutV1AlertMuteRulesJSONRequestBody
				if err := json.Unmarshal([]byte(muteFixture), &body); err != nil {
					t.Fatalf("decode fixture: %v", err)
				}
				response, err := c.PutV1AlertMuteRulesWithResponse(context.Background(), body, editor)
				return response == nil, err
			},
		},
		{
			name: "compute zone", method: http.MethodPut, path: "/v1/computezones", fixture: zoneFixture,
			call: func(t *testing.T, c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				var body PutV1ComputezonesJSONRequestBody
				if err := json.Unmarshal([]byte(zoneFixture), &body); err != nil {
					t.Fatalf("decode fixture: %v", err)
				}
				response, err := c.PutV1ComputezonesWithResponse(context.Background(), body, editor)
				return response == nil, err
			},
		},
		{
			name: "node removal", method: http.MethodPut, path: "/v1/node_removal_policy", fixture: removalFixture,
			call: func(t *testing.T, c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				var body PutV1NodeRemovalPolicyJSONRequestBody
				if err := json.Unmarshal([]byte(removalFixture), &body); err != nil {
					t.Fatalf("decode fixture: %v", err)
				}
				response, err := c.PutV1NodeRemovalPolicyWithResponse(context.Background(), body, editor)
				return response == nil, err
			},
		},
		{
			name: "delete nodes", method: http.MethodDelete, path: "/v1/nodes", fixture: nodesFixture,
			call: func(t *testing.T, c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				var body DeleteV1NodesJSONRequestBody
				if err := json.Unmarshal([]byte(nodesFixture), &body); err != nil {
					t.Fatalf("decode fixture: %v", err)
				}
				response, err := c.DeleteV1NodesWithResponse(context.Background(), &DeleteV1NodesParams{}, body, editor)
				return response == nil, err
			},
		},
		{
			name: "node tags", method: http.MethodPut, path: "/v1/nodes/11111111-1111-4111-8111-111111111111/tags", fixture: nodeTagsFixture,
			call: func(t *testing.T, c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				var body PutV1NodesNodeUuidTagsJSONRequestBody
				if err := json.Unmarshal([]byte(nodeTagsFixture), &body); err != nil {
					t.Fatalf("decode fixture: %v", err)
				}
				response, err := c.PutV1NodesNodeUuidTagsWithResponse(context.Background(), nodeUUID, body, editor)
				return response == nil, err
			},
		},
		{
			name: "create notification", method: http.MethodPost, path: "/v1/notification_preferences", fixture: notificationFixture,
			call: func(t *testing.T, c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				var body PostV1NotificationPreferencesJSONRequestBody
				if err := json.Unmarshal([]byte(notificationFixture), &body); err != nil {
					t.Fatalf("decode fixture: %v", err)
				}
				response, err := c.PostV1NotificationPreferencesWithResponse(context.Background(), body, editor)
				return response == nil, err
			},
		},
		{
			name: "update notification", method: http.MethodPut, path: "/v1/notification_preferences/preference-1", fixture: notificationFixture,
			call: func(t *testing.T, c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				var body PutV1NotificationPreferenceByIDJSONRequestBody
				if err := json.Unmarshal([]byte(notificationFixture), &body); err != nil {
					t.Fatalf("decode fixture: %v", err)
				}
				response, err := c.PutV1NotificationPreferenceByIDWithResponse(context.Background(), "preference-1", body, editor)
				return response == nil, err
			},
		},
		{
			name: "retention", method: http.MethodPut, path: "/v1/retention_policy", fixture: retentionFixture,
			call: func(t *testing.T, c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				var body PutV1RetentionPolicyJSONRequestBody
				if err := json.Unmarshal([]byte(retentionFixture), &body); err != nil {
					t.Fatalf("decode fixture: %v", err)
				}
				response, err := c.PutV1RetentionPolicyWithResponse(context.Background(), body, editor)
				return response == nil, err
			},
		},
		{
			name: "batch tags", method: http.MethodPatch, path: "/v1/tags", fixture: batchTagsFixture,
			call: func(t *testing.T, c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				var body PatchV1TagsJSONRequestBody
				if err := json.Unmarshal([]byte(batchTagsFixture), &body); err != nil {
					t.Fatalf("decode fixture: %v", err)
				}
				response, err := c.PatchV1TagsWithResponse(context.Background(), body, editor)
				return response == nil, err
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var expected interface{}
			if err := json.Unmarshal([]byte(tc.fixture), &expected); err != nil {
				t.Fatalf("decode expected fixture: %v", err)
			}
			transportCalls := 0
			doer := authStatusTestDoer(func(_ *http.Request) (*http.Response, error) {
				transportCalls++
				return nil, errors.New("unexpected transport call")
			})
			client, err := NewClientWithResponses("http://example.test", WithHTTPClient(doer))
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			editorCalls := 0
			responseNil, err := tc.call(t, client, func(_ context.Context, req *http.Request) error {
				editorCalls++
				if req.Method != tc.method || req.URL.Path != tc.path {
					t.Errorf("request = %s %s, want %s %s", req.Method, req.URL.Path, tc.method, tc.path)
				}
				if got := req.Header.Get("Content-Type"); got != "application/json" {
					t.Errorf("content type = %q, want application/json", got)
				}
				var actual interface{}
				if err := json.NewDecoder(req.Body).Decode(&actual); err != nil {
					t.Errorf("decode request body: %v", err)
				} else if !reflect.DeepEqual(actual, expected) {
					t.Errorf("request body = %#v, want %#v", actual, expected)
				}
				return sentinel
			})
			if !errors.Is(err, sentinel) || !responseNil {
				t.Errorf("editor failure: nil response = %t, error = %v; want nil response and sentinel", responseNil, err)
			}
			if editorCalls != 1 || transportCalls != 0 {
				t.Errorf("editor calls = %d, transport calls = %d; want 1 and 0", editorCalls, transportCalls)
			}

			malformedClient, err := NewClientWithResponses("http://[::1", WithHTTPClient(doer))
			if err != nil {
				t.Fatalf("new malformed-server client: %v", err)
			}
			editorCalls, transportCalls = 0, 0
			responseNil, err = tc.call(t, malformedClient, func(_ context.Context, _ *http.Request) error {
				editorCalls++
				return nil
			})
			if err == nil || !responseNil {
				t.Errorf("malformed server: nil response = %t, error = %v; want nil response and construction error", responseNil, err)
			}
			if editorCalls != 0 || transportCalls != 0 {
				t.Errorf("malformed server: editor calls = %d, transport calls = %d; want 0 and 0", editorCalls, transportCalls)
			}
		})
	}
}

type round2NodeReportResponse interface {
	comparable
	round2ResponseMetadata
	GetJSON400() *ModelsErrorResponse
	GetJSON401() *ModelsErrorResponse
	GetJSON403() *ModelsErrorResponse
	GetJSON499() *ModelsErrorResponse
	GetJSON500() *ModelsErrorResponse
	GetJSON503() *ModelsErrorResponse
}

type round2NodeReportClosingReader struct {
	io.Reader
	closed bool
}

func (r *round2NodeReportClosingReader) Close() error {
	r.closed = true
	return nil
}

func round2NodeReportErrors[R round2NodeReportResponse](rsp R) map[int]*ModelsErrorResponse {
	result := map[int]*ModelsErrorResponse{400: rsp.GetJSON400(), 401: rsp.GetJSON401(), 403: rsp.GetJSON403(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500(), 503: rsp.GetJSON503()}
	if with404, ok := any(rsp).(interface{ GetJSON404() *ModelsErrorResponse }); ok {
		result[404] = with404.GetJSON404()
	}
	return result
}

func round2NodeReportHTTPResponse(status int, contentType string, body io.ReadCloser) *http.Response {
	return &http.Response{Status: fmt.Sprintf("%d supplied status", status), StatusCode: status,
		Header: http.Header{"Content-Type": []string{contentType}}, Body: body}
}

func testRound2NodeReportResponse[R round2NodeReportResponse](t *testing.T, zero R, parse func(*http.Response) (R, error), checkNoJSON func(*testing.T, R), success func(*testing.T, func(*http.Response) (R, error))) {
	t.Helper()
	const jsonType = "application/json; charset=utf-8"
	assertRound2ResponseMetadata(t, zero, nil, "", 0, "")
	assertRound2AlertTimelineError(t, 0, round2NodeReportErrors(zero))
	checkNoJSON(t, zero)
	body := []byte(`{"error":"denied","details":"specific detail"}`)
	statuses := []int{400, 401, 403, 499, 500, 503}
	if _, ok := round2NodeReportErrors(zero)[404]; ok {
		statuses = []int{400, 401, 403, 404, 499, 500, 503}
	}
	for _, status := range statuses {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			rsp, err := parse(round2NodeReportHTTPResponse(status, jsonType, io.NopCloser(bytes.NewReader(body))))
			var nilResponse R
			if err != nil || rsp == nilResponse {
				t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
			}
			assertRound2ResponseMetadata(t, rsp, body, fmt.Sprintf("%d supplied status", status), status, jsonType)
			assertRound2AlertTimelineError(t, status, round2NodeReportErrors(rsp))
			checkNoJSON(t, rsp)
		})
	}
	t.Run("malformed", func(t *testing.T) {
		reader := &round2NodeReportClosingReader{Reader: bytes.NewBufferString(`{"error":`)}
		rsp, err := parse(round2NodeReportHTTPResponse(400, jsonType, reader))
		var nilResponse R
		if err == nil || rsp != nilResponse || !reader.closed {
			t.Errorf("parse = %v, %v; closed = %t; want nil response, decoding error and closed body", rsp, err, reader.closed)
		}
	})
	t.Run("read_error", func(t *testing.T) {
		sentinel := errors.New("node report read error")
		reader := &round2AlertTimelineErrorReader{err: sentinel}
		rsp, err := parse(round2NodeReportHTTPResponse(400, jsonType, reader))
		var nilResponse R
		if !errors.Is(err, sentinel) || rsp != nilResponse || !reader.closed {
			t.Errorf("parse = %v, %v; closed = %t; want nil response, sentinel error and closed body", rsp, err, reader.closed)
		}
	})
	t.Run("text_plain", func(t *testing.T) {
		rsp, err := parse(round2NodeReportHTTPResponse(400, "text/plain", io.NopCloser(bytes.NewReader(body))))
		var nilResponse R
		if err != nil || rsp == nilResponse {
			t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
		}
		assertRound2ResponseMetadata(t, rsp, body, "400 supplied status", 400, "text/plain")
		assertRound2AlertTimelineError(t, 0, round2NodeReportErrors(rsp))
		checkNoJSON(t, rsp)
	})
	success(t, parse)
}

func round2NodeReportSuccess[R round2NodeReportResponse](t *testing.T, parse func(*http.Response) (R, error), contentType, body string, headers http.Header) R {
	t.Helper()
	httpResponse := round2NodeReportHTTPResponse(200, contentType, io.NopCloser(bytes.NewBufferString(body)))
	for key, values := range headers {
		httpResponse.Header[key] = values
	}
	rsp, err := parse(httpResponse)
	var nilResponse R
	if err != nil || rsp == nilResponse {
		t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
	}
	assertRound2ResponseMetadata(t, rsp, []byte(body), "200 supplied status", 200, contentType)
	assertRound2AlertTimelineError(t, 0, round2NodeReportErrors(rsp))
	return rsp
}

func round2NodeReportInvalidHeader[R round2NodeReportResponse](t *testing.T, parse func(*http.Response) (R, error)) {
	t.Helper()
	rsp := round2NodeReportHTTPResponse(200, "text/csv", io.NopCloser(bytes.NewBufferString("node\n")))
	rsp.Header.Set("X-Total-Count", "not-an-integer")
	result, err := parse(rsp)
	var nilResponse R
	if err == nil || result != nilResponse {
		t.Errorf("parse = %v, %v; want nil response and header parsing error", result, err)
	}
}

func TestRound2NodeDetailAndReportResponseContracts(t *testing.T) {
	const jsonType = "application/json; charset=utf-8"
	const csvBody = "nodeUUID,hostname\nnode-1,host-1\n"
	headers := http.Header{"Content-Disposition": []string{"attachment; filename=report.csv"}, "X-Total-Count": []string{"3"}}
	t.Run("node_detail", func(t *testing.T) {
		testRound2NodeReportResponse(t, &GetV1NodesNodeUuidResponse{}, ParseGetV1NodesNodeUuidResponse,
			func(t *testing.T, r *GetV1NodesNodeUuidResponse) {
				assertRound2ResponsePointer(t, "GetJSON200", r.GetJSON200(), nil)
			},
			func(t *testing.T, parse func(*http.Response) (*GetV1NodesNodeUuidResponse, error)) {
				r := round2NodeReportSuccess(t, parse, jsonType, `{"nodeUUID":"node-1","dataSource":"inband","hostname":"host-1"}`, nil)
				if r.GetJSON200() == nil {
					t.Fatal("GetJSON200() = nil")
				}
				v, err := r.GetJSON200().AsModelsInbandNodeDetailsResponse()
				if err != nil || v.NodeUUID != "node-1" || v.DataSource != "inband" || v.Hostname == nil || *v.Hostname != "host-1" {
					t.Errorf("node detail = %+v, %v", v, err)
				}
			})
	})
	t.Run("error_report", func(t *testing.T) {
		testRound2NodeReportResponse(t, &GetV1ReportsErrorResponse{}, ParseGetV1ReportsErrorResponse,
			func(t *testing.T, r *GetV1ReportsErrorResponse) {
				assertRound2ResponsePointer(t, "GetJSON200", r.GetJSON200(), nil)
			},
			func(t *testing.T, parse func(*http.Response) (*GetV1ReportsErrorResponse, error)) {
				r := round2NodeReportSuccess(t, parse, jsonType, `{"totalErrors":12,"totalErrorNodes":4,"totalErrorTypes":3}`, nil)
				if r.GetJSON200() == nil {
					t.Fatal("GetJSON200() = nil")
				}
				v, err := r.GetJSON200().AsModelsErrorOverviewResponse()
				if err != nil || v.TotalErrors == nil || *v.TotalErrors != 12 || v.TotalErrorNodes == nil || *v.TotalErrorNodes != 4 || v.TotalErrorTypes == nil || *v.TotalErrorTypes != 3 {
					t.Errorf("error overview = %+v, %v", v, err)
				}
				csv := round2NodeReportSuccess(t, parse, "text/csv", csvBody, nil)
				assertRound2ResponsePointer(t, "GetJSON200", csv.GetJSON200(), nil)
			})
	})
	t.Run("inventory", func(t *testing.T) {
		testRound2NodeReportResponse(t, &GetV1ReportsInventoryResponse{}, ParseGetV1ReportsInventoryResponse,
			func(t *testing.T, r *GetV1ReportsInventoryResponse) {
				assertRound2ResponsePointer(t, "GetJSON200", r.GetJSON200(), nil)
			},
			func(t *testing.T, parse func(*http.Response) (*GetV1ReportsInventoryResponse, error)) {
				r := round2NodeReportSuccess(t, parse, jsonType, `{"nodes":[{"nodeUUID":"inband-1","hostname":"host-1"}],"page":2}`, nil)
				if r.GetJSON200() == nil {
					t.Fatal("GetJSON200() = nil")
				}
				v, err := r.GetJSON200().AsModelsInbandInventoryReportResponse()
				if err != nil || v.Page == nil || *v.Page != 2 || v.Nodes == nil || len(*v.Nodes) != 1 {
					t.Fatalf("inventory = %+v, %v", v, err)
				}
				n := (*v.Nodes)[0]
				if n.NodeUUID == nil || *n.NodeUUID != "inband-1" || n.Hostname == nil || *n.Hostname != "host-1" {
					t.Errorf("inventory node = %+v", n)
				}
				csv := round2NodeReportSuccess(t, parse, "text/csv", csvBody, nil)
				assertRound2ResponsePointer(t, "GetJSON200", csv.GetJSON200(), nil)
			})
	})
	t.Run("security_vulnerabilities", func(t *testing.T) {
		testRound2NodeReportResponse(t, &GetV1ReportsSecurityVulnerabilitiesResponse{}, ParseGetV1ReportsSecurityVulnerabilitiesResponse,
			func(t *testing.T, r *GetV1ReportsSecurityVulnerabilitiesResponse) {
				assertRound2ResponsePointer(t, "GetJSON200", r.GetJSON200(), nil)
			},
			func(t *testing.T, parse func(*http.Response) (*GetV1ReportsSecurityVulnerabilitiesResponse, error)) {
				r := round2NodeReportSuccess(t, parse, jsonType, `{"total":3}`, headers)
				if r.GetJSON200() == nil {
					t.Fatal("GetJSON200() = nil")
				}
				v, err := r.GetJSON200().AsGetV1ReportsSecurityVulnerabilities200JSONResponseBody1()
				if err != nil || v.Total != 3 {
					t.Errorf("security count = %+v, %v", v, err)
				}
				if r.Headers200 == nil || r.Headers200.ContentDisposition == nil || *r.Headers200.ContentDisposition != "attachment; filename=report.csv" || r.Headers200.XTotalCount == nil || *r.Headers200.XTotalCount != 3 {
					t.Errorf("headers = %+v", r.Headers200)
				}
				csv := round2NodeReportSuccess(t, parse, "text/csv", csvBody, headers)
				assertRound2ResponsePointer(t, "GetJSON200", csv.GetJSON200(), nil)
				if csv.Headers200 == nil || csv.Headers200.ContentDisposition == nil || *csv.Headers200.ContentDisposition != "attachment; filename=report.csv" || csv.Headers200.XTotalCount == nil || *csv.Headers200.XTotalCount != 3 {
					t.Errorf("CSV headers = %+v", csv.Headers200)
				}
				round2NodeReportInvalidHeader(t, parse)
			})
	})
	t.Run("vulnerability_nodes", func(t *testing.T) {
		testRound2NodeReportResponse(t, &GetV1ReportsSecurityVulnerabilityNodesResponse{}, ParseGetV1ReportsSecurityVulnerabilityNodesResponse,
			func(_ *testing.T, _ *GetV1ReportsSecurityVulnerabilityNodesResponse) {},
			func(t *testing.T, parse func(*http.Response) (*GetV1ReportsSecurityVulnerabilityNodesResponse, error)) {
				r := round2NodeReportSuccess(t, parse, "text/csv", csvBody, headers)
				if r.Headers200 == nil || r.Headers200.ContentDisposition == nil || *r.Headers200.ContentDisposition != "attachment; filename=report.csv" || r.Headers200.XTotalCount == nil || *r.Headers200.XTotalCount != 3 {
					t.Errorf("headers = %+v", r.Headers200)
				}
				round2NodeReportInvalidHeader(t, parse)
			})
	})
}

func testRound2MuteRuleParser[R interface {
	comparable
	round2ResponseMetadata
}](t *testing.T, statuses []int, noContent bool, parse func(*http.Response) (R, error), errorGetters func(R) map[int]*ModelsErrorResponse, httpResponse func(R) *http.Response) {
	t.Helper()
	const jsonType = "application/json; charset=utf-8"
	body := []byte(`{"error":"denied","details":"specific detail"}`)
	for _, status := range statuses {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			input := round2NodeReportHTTPResponse(status, jsonType, io.NopCloser(bytes.NewReader(body)))
			rsp, err := parse(input)
			var zero R
			if err != nil || rsp == zero {
				t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
			}
			assertRound2ResponseMetadata(t, rsp, body, input.Status, status, jsonType)
			if httpResponse(rsp) != input {
				t.Error("HTTPResponse was not preserved")
			}
			assertRound2AlertTimelineError(t, status, errorGetters(rsp))
		})
		t.Run(fmt.Sprintf("%d_malformed", status), func(t *testing.T) {
			reader := &round2NodeReportClosingReader{Reader: bytes.NewBufferString(`{"error":`)}
			rsp, err := parse(round2NodeReportHTTPResponse(status, jsonType, reader))
			var zero R
			if rsp != zero || err == nil || !reader.closed {
				t.Errorf("parse = %v, %v; closed = %t; want nil response, decoding error and closed body", rsp, err, reader.closed)
			}
		})
	}
	t.Run("read_error", func(t *testing.T) {
		sentinel := errors.New("mute rule read error")
		reader := &round2AlertTimelineErrorReader{err: sentinel}
		rsp, err := parse(round2NodeReportHTTPResponse(statuses[0], jsonType, reader))
		var zero R
		if rsp != zero || !errors.Is(err, sentinel) || !reader.closed {
			t.Errorf("parse = %v, %v; closed = %t; want nil response, sentinel error and closed body", rsp, err, reader.closed)
		}
	})
	t.Run("text_plain", func(t *testing.T) {
		input := round2NodeReportHTTPResponse(statuses[0], "text/plain", io.NopCloser(bytes.NewReader(body)))
		rsp, err := parse(input)
		var zero R
		if err != nil || rsp == zero {
			t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
		}
		assertRound2ResponseMetadata(t, rsp, body, input.Status, input.StatusCode, "text/plain")
		if httpResponse(rsp) != input {
			t.Error("HTTPResponse was not preserved")
		}
		assertRound2AlertTimelineError(t, 0, errorGetters(rsp))
	})
	if noContent {
		t.Run("no_content", func(t *testing.T) {
			input := round2NodeReportHTTPResponse(http.StatusNoContent, jsonType, io.NopCloser(bytes.NewReader(nil)))
			rsp, err := parse(input)
			var zero R
			if err != nil || rsp == zero {
				t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
			}
			assertRound2ResponseMetadata(t, rsp, []byte{}, input.Status, http.StatusNoContent, jsonType)
			if httpResponse(rsp) != input {
				t.Error("HTTPResponse was not preserved")
			}
			assertRound2AlertTimelineError(t, 0, errorGetters(rsp))
		})
	}
}

func TestRound2MuteRuleParserErrorContracts(t *testing.T) {
	t.Run("delete", func(t *testing.T) {
		testRound2MuteRuleParser(t, []int{400, 401, 403, 404, 499, 500}, true, ParseDeleteV1AlertMuteRulesResponse,
			func(r *DeleteV1AlertMuteRulesResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: r.GetJSON400(), 401: r.GetJSON401(), 403: r.GetJSON403(), 404: r.GetJSON404(), 499: r.GetJSON499(), 500: r.GetJSON500()}
			}, func(r *DeleteV1AlertMuteRulesResponse) *http.Response { return r.HTTPResponse })
	})
	t.Run("get", func(t *testing.T) {
		testRound2MuteRuleParser(t, []int{401, 403, 499, 500}, false, ParseGetV1AlertMuteRulesResponse,
			func(r *GetV1AlertMuteRulesResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{401: r.GetJSON401(), 403: r.GetJSON403(), 499: r.GetJSON499(), 500: r.GetJSON500()}
			}, func(r *GetV1AlertMuteRulesResponse) *http.Response { return r.HTTPResponse })
	})
	t.Run("put", func(t *testing.T) {
		testRound2MuteRuleParser(t, []int{400, 401, 403, 499, 500}, true, ParsePutV1AlertMuteRulesResponse,
			func(r *PutV1AlertMuteRulesResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: r.GetJSON400(), 401: r.GetJSON401(), 403: r.GetJSON403(), 499: r.GetJSON499(), 500: r.GetJSON500()}
			}, func(r *PutV1AlertMuteRulesResponse) *http.Response { return r.HTTPResponse })
	})
}

func testRound2AlertParserMalformedStatuses[T any](t *testing.T, statuses []int, parse func(*http.Response) (*T, error)) {
	t.Helper()
	for _, status := range statuses {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			reader := &round2NodeReportClosingReader{Reader: bytes.NewBufferString(`{"error":`)}
			rsp, err := parse(round2NodeReportHTTPResponse(status, "application/json", reader))
			if rsp != nil {
				t.Errorf("response = %v, want nil", rsp)
			}
			var syntaxErr *json.SyntaxError
			if !errors.As(err, &syntaxErr) {
				t.Errorf("error = %v, want *json.SyntaxError", err)
			}
			if !reader.closed {
				t.Error("response body was not closed")
			}
		})
	}
}

func TestRound2SigningOverviewBucketsAndV2MetricsMalformedStatuses(t *testing.T) {
	t.Run("signing_key", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{499, 503}, ParseGetWellKnownSigningKeyPubResponse)
	})
	t.Run("overview", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 401, 403, 499, 500, 503}, ParseGetV1OverviewResponse)
	})
	t.Run("event_buckets", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 401, 403, 499, 500, 503}, ParseGetV1EventsBucketsResponse)
	})
	t.Run("v2_metrics", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200}, ParseGetV2MetricsResponse)
	})
}

func TestRound2AlertParserMalformedStatuses(t *testing.T) {
	t.Run("filter_options", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 401, 500}, ParseGetV1AlertTimelineFilterOptionsResponse)
	})
	t.Run("timeline_nodes", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 401, 499, 500}, ParseGetV1AlertTimelineNodesResponse)
	})
	t.Run("timeline_node_alerts", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 401, 404, 499, 500}, ParseGetV1AlertTimelineNodesNodeUuidAlertsResponse)
	})
	t.Run("timeline_node_alert", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 401, 404, 499, 500}, ParseGetV1AlertTimelineNodesNodeUuidAlertsAlertUuidResponse)
	})
	t.Run("alerts", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 499, 500}, ParseGetV1AlertsResponse)
	})
	t.Run("node_alerts", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 404, 499, 500}, ParseGetV1AlertsNodeUuidResponse)
	})
}

func TestRound2MetricsParserMalformedStatuses(t *testing.T) {
	t.Run("metrics", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 403, 499, 500}, ParseGetV1MetricsResponse)
	})
	t.Run("query_range", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 401, 403, 404, 499, 500, 503}, ParseGetV1MetricsQueryRangeResponse)
	})
}

func TestRound2NodeDetailTagsRankingsMalformedStatuses(t *testing.T) {
	t.Run("node_detail", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 401, 403, 404, 499, 500, 503}, ParseGetV1NodesNodeUuidResponse)
	})
	t.Run("node_tags", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 404, 499, 500}, ParsePutV1NodesNodeUuidTagsResponse)
	})
	t.Run("rankings", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 401, 403, 499, 500, 501, 503}, ParseGetV1PredictedHealthRankingsResponse)
	})
}

func TestRound2ResourceListParserMalformedStatuses(t *testing.T) {
	t.Run("get_compute_zones", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{401, 403, 499, 500, 503}, ParseGetV1ComputezonesResponse)
	})
	t.Run("put_compute_zones", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{401, 403, 499, 500}, ParsePutV1ComputezonesResponse)
	})
	t.Run("events", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{401, 403, 499, 500, 503}, ParseGetV1EventsResponse)
	})
	t.Run("node_groups", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{401, 403, 499, 500, 503}, ParseGetV1NodegroupsResponse)
	})
	t.Run("nodes", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{401, 403, 499, 500, 503}, ParseGetV1NodesResponse)
	})
}

func TestRound2ResourceParserMalformedStatuses(t *testing.T) {
	t.Run("event_options", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 403, 500, 503}, ParseGetV1EventsOptionsResponse)
	})
	t.Run("info", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 499, 500}, ParseGetV1InfoResponse)
	})
	t.Run("node_group_memberships", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 401, 403, 499, 500, 503}, ParseGetV1NodeGroupMembershipsResponse)
	})
	t.Run("get_node_removal_policy", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 403, 500, 503}, ParseGetV1NodeRemovalPolicyResponse)
	})
	t.Run("put_node_removal_policy", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{401, 403, 500, 503}, ParsePutV1NodeRemovalPolicyResponse)
	})
	t.Run("node_group_options", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 403, 499, 500}, ParseGetV1NodegroupsOptionsResponse)
	})
	t.Run("delete_nodes", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 401, 403, 404, 499, 500, 503}, ParseDeleteV1NodesResponse)
	})
	t.Run("delete_node", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 401, 403, 404, 499, 500, 503}, ParseDeleteV1NodesNodeUuidResponse)
	})
	t.Run("nodes_history", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 401, 403, 500, 503}, ParseGetV1NodesHistoryResponse)
	})
	t.Run("nodes_options", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 401, 403, 499, 500}, ParseGetV1NodesOptionsResponse)
	})
	t.Run("node_health_history", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 401, 404, 499, 500}, ParseGetV1NodesNodeUuidHealthHistoryResponse)
	})
}

func TestRound2NotificationParserMalformedStatuses(t *testing.T) {
	t.Run("delivery_log", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 401, 403, 499, 500, 503}, ParseGetV1NotificationDeliveryLogResponse)
	})
	t.Run("preferences", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 403, 499, 500, 503}, ParseGetV1NotificationPreferencesResponse)
	})
	t.Run("preference_by_id", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 403, 404, 499, 500, 503}, ParseGetV1NotificationPreferenceByIDResponse)
	})
	t.Run("post_preferences", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{400, 401, 403, 499, 500, 503}, ParsePostV1NotificationPreferencesResponse)
	})
	t.Run("delete_preference_by_id", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{401, 403, 404, 499, 500, 503}, ParseDeleteV1NotificationPreferenceByIDResponse)
	})
	t.Run("put_preference_by_id", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{401, 403, 404, 499, 500, 503}, ParsePutV1NotificationPreferenceByIDResponse)
	})
}

func TestRound2ReportParserMalformedStatuses(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 401, 403, 499, 500, 503}, ParseGetV1ReportsErrorResponse)
	})
	t.Run("inventory", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 401, 403, 499, 500, 503}, ParseGetV1ReportsInventoryResponse)
	})
	t.Run("security_vulnerabilities", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 401, 403, 499, 500, 503}, ParseGetV1ReportsSecurityVulnerabilitiesResponse)
	})
	t.Run("security_vulnerability_options", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 401, 403, 422, 499, 500, 503}, ParseGetV1ReportsSecurityVulnerabilityOptionsResponse)
	})
	t.Run("security_vulnerability_nodes", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{401, 403, 404, 499, 500, 503}, ParseGetV1ReportsSecurityVulnerabilityNodesResponse)
	})
}

func TestRound2PolicyTagsXIDParserMalformedStatuses(t *testing.T) {
	t.Run("get_retention_policy", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 403, 404, 499, 500, 503}, ParseGetV1RetentionPolicyResponse)
	})
	t.Run("put_retention_policy", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{401, 403, 499, 500, 503}, ParsePutV1RetentionPolicyResponse)
	})
	t.Run("stats", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 401, 403, 404, 499, 500, 503}, ParseGetV1StatsResponse)
	})
	t.Run("get_tags", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 401, 403, 499, 500}, ParseGetV1TagsResponse)
	})
	t.Run("patch_tags", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{401, 403, 499, 500}, ParsePatchV1TagsResponse)
	})
	t.Run("xid_bursts", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 401, 403, 499, 500}, ParseGetV1XIDBurstsResponse)
	})
	t.Run("xid_burst_options", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 401, 403, 499, 500}, ParseGetV1XIDBurstOptionsResponse)
	})
	t.Run("xid_burst_detail", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 401, 403, 404, 499, 500}, ParseGetV1XIDBurstDetailResponse)
	})
	t.Run("commands", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 499, 500}, ParseGetV2CommandsResponse)
	})
}

func TestRound2RemainingModelEnumValidContracts(t *testing.T) {
	// Literals come from the corresponding schemas in api/openapi/openapi.yaml.
	t.Run("ErrorSeverity", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[ModelsErrorSeverity]{
			{ErrorSeverityFatal, "Fatal"}, {ErrorSeverityCritical, "Critical"}, {ErrorSeverityWarning, "Warning"},
		}, "fatal")
	})
	t.Run("FirmwareCheck", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[ModelsFirmwareCheck]{
			{FirmwareCheckPassed, "Passed"}, {FirmwareCheckFailed, "Failed"}, {FirmwareCheckUnknown, "Unknown"},
		}, "passed")
	})
	t.Run("IntegrityCheck", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[ModelsIntegrityCheck]{
			{IntegrityCheckFullAttestation, "Full Attestation"}, {IntegrityCheckPartialAttestation, "Partial Attestation"},
			{IntegrityCheckVerificationFailed, "Verification Failed"}, {IntegrityCheckPassed, "Verified"},
			{IntegrityCheckFailed, "Unverified"}, {IntegrityCheckDegraded, "Degraded"},
			{IntegrityCheckPending, "Pending"}, {IntegrityCheckUnsupported, "Unsupported"}, {IntegrityCheckUnknown, "Unknown"},
		}, "verified")
	})
	t.Run("VerificationCheck", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[ModelsVerificationCheck]{
			{VerificationCheckFullAttestation, "Full Attestation"}, {VerificationCheckPartialAttestation, "Partial Attestation"},
			{VerificationCheckVerificationFailed, "Verification Failed"}, {VerificationCheckPassed, "Verified"},
			{VerificationCheckFailed, "Unverified"}, {VerificationCheckDegraded, "Degraded"},
			{VerificationCheckPending, "Pending"}, {VerificationCheckUnsupported, "Unsupported"}, {VerificationCheckUnknown, "Unknown"},
		}, "verified")
	})
	t.Run("MetricUnit", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[ModelsMetricUnit]{
			{MetricUnitMegahertz, "MHz"}, {MetricUnitCount, "count"}, {MetricUnitDegreesCelsius, "°C"},
			{MetricUnitDegreesFahrenheit, "°F"}, {MetricUnitCelsius, "celsius"}, {MetricUnitWatts, "watts"},
			{MetricUnitBoolean, "boolean"}, {MetricUnitMegabyte, "MB"}, {MetricUnitRatio, "ratio"},
			{MetricUnitMegabytesPerSecond, "MB/s"}, {MetricUnitBitErrorRate, "ber"}, {MetricUnitNanoseconds, "nanoseconds"},
			{MetricUnitMilliseconds, "ms"}, {MetricUnitSeconds, "seconds"}, {MetricUnitSecond, "s"},
			{MetricUnitBytes, "bytes"}, {MetricUnitBytesPerSecond, "bytes/s"}, {MetricUnitMillijoules, "millijoules"},
			{MetricUnitLoad, "load"}, {MetricUnitCores, "cores"}, {MetricUnitPercent, "percent"},
			{MetricUnitRPM, "rpm"}, {MetricUnitVolts, "volts"},
		}, "mhz")
	})
	t.Run("NodeKind", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[ModelsNodeKind]{
			{NodeKindUnknown, "unknown"}, {NodeKindCompute, "compute"}, {NodeKindNvSwitch, "nvswitch"}, {NodeKindPowerShelf, "power_shelf"},
		}, "Compute")
	})
	t.Run("PredictionStatus", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[ModelsPredictedHealthRankingItemPredictionStatus]{
			{Ranked, "ranked"}, {Unranked, "unranked"},
		}, "Ranked")
	})
	t.Run("RankUnit", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[ModelsPredictedHealthRankingsResponseRankUnit]{
			{ModelsPredictedHealthRankingsResponseRankUnitNode, "node"},
			{ModelsPredictedHealthRankingsResponseRankUnitNvlinkDomain, "nvlinkDomain"},
			{ModelsPredictedHealthRankingsResponseRankUnitRack, "rack"},
		}, "Node")
	})
	t.Run("SecurityVulnerabilitySeverity", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[ModelsSecurityVulnerabilitySeverity]{
			{SecurityVulnerabilitySeverityLow, "LOW"}, {SecurityVulnerabilitySeverityMedium, "MEDIUM"},
			{SecurityVulnerabilitySeverityHigh, "HIGH"}, {SecurityVulnerabilitySeverityCritical, "CRITICAL"},
		}, "low")
	})
	t.Run("VulnerabilityComponent", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[ModelsVulnerabilityComponent]{
			{VulnerabilityComponentGPUDriver, "gpuDriver"}, {VulnerabilityComponentVBIOS, "vbios"},
		}, "GpuDriver")
	})
	t.Run("VerificationPolicyTier", func(t *testing.T) {
		for _, value := range []struct {
			constant ModelsVerificationPolicyTier
			literal  int
		}{
			{ModelsVerificationPolicyTierN1, 1}, {ModelsVerificationPolicyTierN2, 2}, {ModelsVerificationPolicyTierN3, 3},
		} {
			if int(value.constant) != value.literal {
				t.Errorf("tier constant = %d, want %d", value.constant, value.literal)
			}
			if !value.constant.Valid() {
				t.Errorf("tier constant %d is not valid", value.constant)
			}
			if !ModelsVerificationPolicyTier(value.literal).Valid() {
				t.Errorf("tier schema literal %d is not valid", value.literal)
			}
		}
		for _, invalid := range []ModelsVerificationPolicyTier{0, 4} {
			if invalid.Valid() {
				t.Errorf("invalid tier %d was accepted", invalid)
			}
		}
	})
	t.Run("VerificationPolicyVerdictTier", func(t *testing.T) {
		for _, value := range []struct {
			constant ModelsVerificationPolicyVerdictTier
			literal  int
		}{
			{ModelsVerificationPolicyVerdictTierN1, 1}, {ModelsVerificationPolicyVerdictTierN2, 2}, {ModelsVerificationPolicyVerdictTierN3, 3},
		} {
			if int(value.constant) != value.literal {
				t.Errorf("verdict tier constant = %d, want %d", value.constant, value.literal)
			}
			if !value.constant.Valid() {
				t.Errorf("verdict tier constant %d is not valid", value.constant)
			}
			if !ModelsVerificationPolicyVerdictTier(value.literal).Valid() {
				t.Errorf("verdict tier schema literal %d is not valid", value.literal)
			}
		}
		for _, invalid := range []ModelsVerificationPolicyVerdictTier{0, 4} {
			if invalid.Valid() {
				t.Errorf("invalid verdict tier %d was accepted", invalid)
			}
		}
	})
}

func TestRound2AlertComputezoneQueryEnumContracts(t *testing.T) {
	t.Run("GetV1AlertTimelineNodesParamsSortBy", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1AlertTimelineNodesParamsSortBy]{
			{GetV1AlertTimelineNodesParamsSortByHostname, "hostname"},
			{GetV1AlertTimelineNodesParamsSortByNodeName, "nodeName"},
			{GetV1AlertTimelineNodesParamsSortByBmcMac, "bmcMac"},
			{GetV1AlertTimelineNodesParamsSortByNodeUUID, "nodeUUID"},
			{GetV1AlertTimelineNodesParamsSortByAlert, "alert"},
			{GetV1AlertTimelineNodesParamsSortByGpuType, "gpuType"},
			{GetV1AlertTimelineNodesParamsSortByNodeGroup, "nodeGroup"},
			{GetV1AlertTimelineNodesParamsSortByComputeZone, "computeZone"},
			{GetV1AlertTimelineNodesParamsSortByLastUpdate, "lastUpdate"},
		}, "Hostname")
	})
	t.Run("GetV1AlertTimelineNodesParamsOrder", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1AlertTimelineNodesParamsOrder]{
			{GetV1AlertTimelineNodesParamsOrderAsc, "asc"},
			{GetV1AlertTimelineNodesParamsOrderDesc, "desc"},
		}, "Asc")
	})
	t.Run("GetV1AlertTimelineNodesParamsAlertStates", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1AlertTimelineNodesParamsAlertStates]{
			{GetV1AlertTimelineNodesParamsAlertStatesCritical, "Critical"},
			{GetV1AlertTimelineNodesParamsAlertStatesWarning, "Warning"},
			{GetV1AlertTimelineNodesParamsAlertStatesResolved, "Resolved"},
		}, "critical")
	})
	t.Run("GetV1AlertTimelineNodesParamsDataSources", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1AlertTimelineNodesParamsDataSources]{
			{GetV1AlertTimelineNodesParamsDataSourcesInband, "inband"},
			{GetV1AlertTimelineNodesParamsDataSourcesOob, "oob"},
		}, "Inband")
	})
	t.Run("GetV1AlertTimelineNodesNodeUuidAlertsParamsSortBy", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1AlertTimelineNodesNodeUuidAlertsParamsSortBy]{
			{GetV1AlertTimelineNodesNodeUuidAlertsParamsSortByStartTime, "startTime"},
			{GetV1AlertTimelineNodesNodeUuidAlertsParamsSortByLastUpdate, "lastUpdate"},
			{GetV1AlertTimelineNodesNodeUuidAlertsParamsSortByDataSource, "dataSource"},
		}, "StartTime")
	})
	t.Run("GetV1AlertTimelineNodesNodeUuidAlertsParamsOrder", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1AlertTimelineNodesNodeUuidAlertsParamsOrder]{
			{GetV1AlertTimelineNodesNodeUuidAlertsParamsOrderAsc, "asc"},
			{GetV1AlertTimelineNodesNodeUuidAlertsParamsOrderDesc, "desc"},
		}, "Asc")
	})
	t.Run("GetV1AlertTimelineNodesNodeUuidAlertsParamsAlertStates", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1AlertTimelineNodesNodeUuidAlertsParamsAlertStates]{
			{GetV1AlertTimelineNodesNodeUuidAlertsParamsAlertStatesCritical, "Critical"},
			{GetV1AlertTimelineNodesNodeUuidAlertsParamsAlertStatesWarning, "Warning"},
			{GetV1AlertTimelineNodesNodeUuidAlertsParamsAlertStatesResolved, "Resolved"},
		}, "critical")
	})
	t.Run("GetV1AlertTimelineNodesNodeUuidAlertsParamsDataSources", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1AlertTimelineNodesNodeUuidAlertsParamsDataSources]{
			{GetV1AlertTimelineNodesNodeUuidAlertsParamsDataSourcesInband, "inband"},
			{GetV1AlertTimelineNodesNodeUuidAlertsParamsDataSourcesOob, "oob"},
		}, "Inband")
	})
	t.Run("GetV1AlertTimelineNodesNodeUuidAlertsAlertUuidParamsOrder", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1AlertTimelineNodesNodeUuidAlertsAlertUuidParamsOrder]{
			{GetV1AlertTimelineNodesNodeUuidAlertsAlertUuidParamsOrderAsc, "asc"},
			{GetV1AlertTimelineNodesNodeUuidAlertsAlertUuidParamsOrderDesc, "desc"},
		}, "Asc")
	})
	t.Run("GetV1ComputezonesParamsSortBy", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1ComputezonesParamsSortBy]{
			{GetV1ComputezonesParamsSortByCreatedAt, "createdAt"},
			{GetV1ComputezonesParamsSortByName, "name"},
		}, "CreatedAt")
	})
	t.Run("GetV1ComputezonesParamsOrder", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1ComputezonesParamsOrder]{
			{GetV1ComputezonesParamsOrderAsc, "asc"},
			{GetV1ComputezonesParamsOrderDesc, "desc"},
		}, "Asc")
	})
}

func TestRound2EventsQueryEnumContracts(t *testing.T) {
	t.Run("GetV1EventsParamsTimeMode", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1EventsParamsTimeMode]{
			{GetV1EventsParamsTimeModeAbsolute, "absolute"},
			{GetV1EventsParamsTimeModeRelative, "relative"},
		}, "Absolute")
	})
	t.Run("GetV1EventsParamsDataSources", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1EventsParamsDataSources]{
			{GetV1EventsParamsDataSourcesInband, "inband"},
			{GetV1EventsParamsDataSourcesOob, "oob"},
		}, "Inband")
	})
	t.Run("GetV1EventsParamsSortBy", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1EventsParamsSortBy]{
			{GetV1EventsParamsSortByTimestamp, "timestamp"},
			{GetV1EventsParamsSortByEventName, "eventName"},
			{GetV1EventsParamsSortBySeverity, "severity"},
			{GetV1EventsParamsSortByComponent, "component"},
			{GetV1EventsParamsSortByGpuType, "gpuType"},
			{GetV1EventsParamsSortByHostname, "hostname"},
			{GetV1EventsParamsSortByNodeName, "nodeName"},
			{GetV1EventsParamsSortByBmcMac, "bmcMac"},
			{GetV1EventsParamsSortByNodeUUID, "nodeUUID"},
			{GetV1EventsParamsSortByNodeGroup, "nodeGroup"},
			{GetV1EventsParamsSortByComputeZone, "computeZone"},
		}, "Timestamp")
	})
	t.Run("GetV1EventsParamsSortOrder", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1EventsParamsSortOrder]{
			{GetV1EventsParamsSortOrderAsc, "asc"},
			{GetV1EventsParamsSortOrderDesc, "desc"},
		}, "Asc")
	})
	t.Run("GetV1EventsBucketsParamsTimeMode", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1EventsBucketsParamsTimeMode]{
			{GetV1EventsBucketsParamsTimeModeAbsolute, "absolute"},
			{GetV1EventsBucketsParamsTimeModeRelative, "relative"},
		}, "Absolute")
	})
	t.Run("GetV1EventsBucketsParamsDataSources", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1EventsBucketsParamsDataSources]{
			{GetV1EventsBucketsParamsDataSourcesInband, "inband"},
			{GetV1EventsBucketsParamsDataSourcesOob, "oob"},
		}, "Inband")
	})
}

func TestRound2MetricsAndNodeQueryEnumContracts(t *testing.T) {
	t.Run("GetV1MetricsParamsDataSource", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1MetricsParamsDataSource]{
			{GetV1MetricsParamsDataSourceAll, "all"},
			{GetV1MetricsParamsDataSourceInband, "inband"},
			{GetV1MetricsParamsDataSourceOob, "oob"},
		}, "ALL")
	})
	t.Run("GetV1MetricsParamsEntityLevel", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1MetricsParamsEntityLevel]{
			{GetV1MetricsParamsEntityLevelComputezone, "computezone"},
			{GetV1MetricsParamsEntityLevelNode, "node"},
			{GetV1MetricsParamsEntityLevelNodegroup, "nodegroup"},
		}, "COMPUTEZONE")
	})
	t.Run("GetV1MetricsQueryRangeParamsTimeMode", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1MetricsQueryRangeParamsTimeMode]{
			{GetV1MetricsQueryRangeParamsTimeModeAbsolute, "absolute"},
			{GetV1MetricsQueryRangeParamsTimeModeRelative, "relative"},
		}, "ABSOLUTE")
	})
	t.Run("GetV1MetricsQueryRangeParamsEntityLevel", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1MetricsQueryRangeParamsEntityLevel]{
			{GetV1MetricsQueryRangeParamsEntityLevelNode, "node"},
		}, "NODE")
	})
	t.Run("GetV1NodegroupsParamsHealthStatuses", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1NodegroupsParamsHealthStatuses]{
			{GetV1NodegroupsParamsHealthStatusesHealthy, "Healthy"},
			{GetV1NodegroupsParamsHealthStatusesDegraded, "Degraded"},
			{GetV1NodegroupsParamsHealthStatusesUnhealthy, "Unhealthy"},
			{GetV1NodegroupsParamsHealthStatusesUnknown, "Unknown"},
		}, "hEALTHY")
	})
	t.Run("GetV1NodesParamsHealthStatuses", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1NodesParamsHealthStatuses]{
			{GetV1NodesParamsHealthStatusesHealthy, "Healthy"},
			{GetV1NodesParamsHealthStatusesDegraded, "Degraded"},
			{GetV1NodesParamsHealthStatusesUnhealthy, "Unhealthy"},
			{GetV1NodesParamsHealthStatusesUnknown, "Unknown"},
		}, "hEALTHY")
	})
	t.Run("GetV1NodegroupsParamsOrder", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1NodegroupsParamsOrder]{
			{GetV1NodegroupsParamsOrderAsc, "asc"},
			{GetV1NodegroupsParamsOrderDesc, "desc"},
		}, "ASC")
	})
	t.Run("DeleteV1NodesParamsDataSource", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[DeleteV1NodesParamsDataSource]{
			{DeleteV1NodesParamsDataSourceInband, "inband"},
			{DeleteV1NodesParamsDataSourceOob, "oob"},
		}, "INBAND")
	})
	t.Run("DeleteV1NodesParamsAgentType", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[DeleteV1NodesParamsAgentType]{
			{DeleteV1NodesParamsAgentTypeInband, "inband"},
			{DeleteV1NodesParamsAgentTypeOob, "oob"},
		}, "INBAND")
	})
	t.Run("GetV1NodesParamsDataSource", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1NodesParamsDataSource]{
			{GetV1NodesParamsDataSourceInband, "inband"},
			{GetV1NodesParamsDataSourceOob, "oob"},
		}, "INBAND")
	})
	t.Run("GetV1NodesParamsAgentType", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1NodesParamsAgentType]{
			{GetV1NodesParamsAgentTypeInband, "inband"},
			{GetV1NodesParamsAgentTypeOob, "oob"},
		}, "INBAND")
	})
	t.Run("GetV1NodesOptionsParamsDataSource", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1NodesOptionsParamsDataSource]{
			{GetV1NodesOptionsParamsDataSourceInband, "inband"},
			{GetV1NodesOptionsParamsDataSourceOob, "oob"},
		}, "INBAND")
	})
	t.Run("GetV1NodesOptionsParamsAgentType", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1NodesOptionsParamsAgentType]{
			{GetV1NodesOptionsParamsAgentTypeInband, "inband"},
			{GetV1NodesOptionsParamsAgentTypeOob, "oob"},
		}, "INBAND")
	})
	t.Run("DeleteV1NodesNodeUuidParamsDataSource", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[DeleteV1NodesNodeUuidParamsDataSource]{
			{DeleteV1NodesNodeUuidParamsDataSourceInband, "inband"},
			{DeleteV1NodesNodeUuidParamsDataSourceOob, "oob"},
		}, "INBAND")
	})
	t.Run("DeleteV1NodesNodeUuidParamsAgentType", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[DeleteV1NodesNodeUuidParamsAgentType]{
			{DeleteV1NodesNodeUuidParamsAgentTypeInband, "inband"},
			{DeleteV1NodesNodeUuidParamsAgentTypeOob, "oob"},
		}, "INBAND")
	})
	t.Run("GetV1NodesNodeUuidParamsDataSource", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1NodesNodeUuidParamsDataSource]{
			{GetV1NodesNodeUuidParamsDataSourceInband, "inband"},
			{GetV1NodesNodeUuidParamsDataSourceOob, "oob"},
		}, "INBAND")
	})
	t.Run("GetV1NodesNodeUuidParamsAgentType", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1NodesNodeUuidParamsAgentType]{
			{GetV1NodesNodeUuidParamsAgentTypeInband, "inband"},
			{GetV1NodesNodeUuidParamsAgentTypeOob, "oob"},
		}, "INBAND")
	})
	t.Run("GetV1NodesParamsSortBy", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1NodesParamsSortBy]{
			{GetV1NodesParamsSortByHostname, "hostname"},
			{GetV1NodesParamsSortByNodeName, "nodeName"},
			{GetV1NodesParamsSortByNodeUUID, "nodeUUID"},
			{GetV1NodesParamsSortByHealthStatus, "healthStatus"},
			{GetV1NodesParamsSortByNodegroup, "nodegroup"},
			{GetV1NodesParamsSortByComputezone, "computezone"},
			{GetV1NodesParamsSortByGpuType, "gpuType"},
			{GetV1NodesParamsSortByGpuCount, "gpuCount"},
			{GetV1NodesParamsSortByVerificationCheck, "verificationCheck"},
			{GetV1NodesParamsSortByIntegrityCheck, "integrityCheck"},
			{GetV1NodesParamsSortByAgentStatus, "agentStatus"},
			{GetV1NodesParamsSortByAgentVersion, "agentVersion"},
			{GetV1NodesParamsSortByKernelVersion, "kernelVersion"},
			{GetV1NodesParamsSortByGpuDriverVersion, "gpuDriverVersion"},
			{GetV1NodesParamsSortByGpuFirmwareVersions, "gpuFirmwareVersions"},
			{GetV1NodesParamsSortByBmcMac, "bmcMac"},
		}, "HOSTNAME")
	})
	t.Run("GetV1NodesHistoryParamsActorType", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1NodesHistoryParamsActorType]{
			{GetV1NodesHistoryParamsActorTypeSystem, "system"},
			{GetV1NodesHistoryParamsActorTypeUser, "user"},
			{GetV1NodesHistoryParamsActorTypeAll, "all"},
		}, "SYSTEM")
	})
	t.Run("GetV1NodesOptionsParamsView", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1NodesOptionsParamsView]{
			{GetV1NodesOptionsParamsViewDetail, "detail"},
			{GetV1NodesOptionsParamsViewBasic, "basic"},
		}, "DETAIL")
	})
}

func TestRound2RankingsReportStatsQueryEnumContracts(t *testing.T) {
	t.Run("GetV1PredictedHealthRankingsParamsRankUnit", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1PredictedHealthRankingsParamsRankUnit]{
			{GetV1PredictedHealthRankingsParamsRankUnitNode, "node"},
			{GetV1PredictedHealthRankingsParamsRankUnitNvlinkDomain, "nvlinkDomain"},
			{GetV1PredictedHealthRankingsParamsRankUnitRack, "rack"},
		}, "NODE")
	})
	t.Run("GetV1PredictedHealthRankingsParamsAggregation", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1PredictedHealthRankingsParamsAggregation]{
			{Max, "max"},
			{NthMax, "nthMax"},
			{Mean, "mean"},
			{CountAbove, "countAbove"},
		}, "MAX")
	})
	t.Run("GetV1PredictedHealthRankingsParamsMissingScorePolicy", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1PredictedHealthRankingsParamsMissingScorePolicy]{
			{Ignore, "ignore"},
			{RequireAll, "requireAll"},
		}, "IGNORE")
	})
	t.Run("GetV1PredictedHealthRankingsParamsTagMatch", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1PredictedHealthRankingsParamsTagMatch]{
			{GetV1PredictedHealthRankingsParamsTagMatchAny, "any"},
			{GetV1PredictedHealthRankingsParamsTagMatchAll, "all"},
		}, "ANY")
	})
	t.Run("GetV1PredictedHealthRankingsParamsHealthStatuses", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1PredictedHealthRankingsParamsHealthStatuses]{
			{GetV1PredictedHealthRankingsParamsHealthStatusesHealthy, "Healthy"},
			{GetV1PredictedHealthRankingsParamsHealthStatusesDegraded, "Degraded"},
			{GetV1PredictedHealthRankingsParamsHealthStatusesUnhealthy, "Unhealthy"},
			{GetV1PredictedHealthRankingsParamsHealthStatusesUnknown, "Unknown"},
		}, "healthy")
	})
	t.Run("GetV1PredictedHealthRankingsParamsOrder", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1PredictedHealthRankingsParamsOrder]{
			{BestFirst, "bestFirst"},
			{WorstFirst, "worstFirst"},
		}, "BESTFIRST")
	})
	t.Run("GetV1ReportsErrorParamsFormat", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1ReportsErrorParamsFormat]{
			{GetV1ReportsErrorParamsFormatJson, "json"},
			{GetV1ReportsErrorParamsFormatCsv, "csv"},
		}, "CSV")
	})
	t.Run("GetV1ReportsErrorParamsDataSource", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1ReportsErrorParamsDataSource]{
			{GetV1ReportsErrorParamsDataSourceInband, "inband"},
			{GetV1ReportsErrorParamsDataSourceOob, "oob"},
		}, "INBAND")
	})
	t.Run("GetV1ReportsInventoryParamsFormat", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1ReportsInventoryParamsFormat]{
			{GetV1ReportsInventoryParamsFormatJson, "json"},
			{GetV1ReportsInventoryParamsFormatCsv, "csv"},
		}, "CSV")
	})
	t.Run("GetV1ReportsInventoryParamsDataSource", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1ReportsInventoryParamsDataSource]{
			{GetV1ReportsInventoryParamsDataSourceInband, "inband"},
			{GetV1ReportsInventoryParamsDataSourceOob, "oob"},
		}, "INBAND")
	})
	t.Run("GetV1ReportsInventoryParamsAgentType", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1ReportsInventoryParamsAgentType]{
			{GetV1ReportsInventoryParamsAgentTypeInband, "inband"},
			{GetV1ReportsInventoryParamsAgentTypeOob, "oob"},
		}, "INBAND")
	})
	t.Run("GetV1ReportsInventoryParamsOrder", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1ReportsInventoryParamsOrder]{
			{GetV1ReportsInventoryParamsOrderAsc, "asc"},
			{GetV1ReportsInventoryParamsOrderDesc, "desc"},
		}, "ASC")
	})
	t.Run("GetV1ReportsSecurityVulnerabilitiesParamsOrder", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1ReportsSecurityVulnerabilitiesParamsOrder]{
			{GetV1ReportsSecurityVulnerabilitiesParamsOrderAsc, "asc"},
			{GetV1ReportsSecurityVulnerabilitiesParamsOrderDesc, "desc"},
		}, "ASC")
	})
	t.Run("GetV1ReportsSecurityVulnerabilitiesParamsFormat", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1ReportsSecurityVulnerabilitiesParamsFormat]{
			{GetV1ReportsSecurityVulnerabilitiesParamsFormatJson, "json"},
			{GetV1ReportsSecurityVulnerabilitiesParamsFormatCsv, "csv"},
		}, "CSV")
	})
	t.Run("GetV1ReportsSecurityVulnerabilityNodesParamsFormat", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1ReportsSecurityVulnerabilityNodesParamsFormat]{
			{GetV1ReportsSecurityVulnerabilityNodesParamsFormatCsv, "csv"},
		}, "CSV")
	})
	t.Run("GetV1StatsParamsName", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1StatsParamsName]{
			{GetV1StatsParamsNameAgentStatus, "agentStatus"},
		}, "AGENTSTATUS")
	})
	t.Run("GetV1StatsParamsDataSource", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1StatsParamsDataSource]{
			{GetV1StatsParamsDataSourceInband, "inband"},
			{GetV1StatsParamsDataSourceOob, "oob"},
		}, "INBAND")
	})
}

func TestRound2RemainingReadRequestFailures(t *testing.T) {
	sentinel := errors.New("editor rejected request")
	tests := []struct {
		name string
		path string
		call func(*ClientWithResponses, RequestEditorFn) (bool, error)
	}{
		{
			name: "signing key", path: "/.well-known/signing-key.pub",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetWellKnownSigningKeyPubWithResponse(context.Background(), editor)
				return response == nil, err
			},
		},
		{
			name: "alert mute rules", path: "/v1/alert_mute_rules",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1AlertMuteRulesWithResponse(context.Background(), editor)
				return response == nil, err
			},
		},
		{
			name: "event options", path: "/v1/events/options",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1EventsOptionsWithResponse(context.Background(), editor)
				return response == nil, err
			},
		},
		{
			name: "info", path: "/v1/info",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1InfoWithResponse(context.Background(), editor)
				return response == nil, err
			},
		},
		{
			name: "metrics query range", path: "/v1/metrics/query_range",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				mode, window := GetV1MetricsQueryRangeParamsTimeModeRelative, "1h30m"
				response, err := c.GetV1MetricsQueryRangeWithResponse(context.Background(), &GetV1MetricsQueryRangeParams{TimeMode: &mode, Window: &window, Step: "5m", Component: "gpu", Metric: "gpu_temperature", EntityLevel: GetV1MetricsQueryRangeParamsEntityLevelNode, EntityID: "node-1"}, editor)
				return response == nil, err
			},
		},
		{
			name: "node group memberships", path: "/v1/node_group_memberships",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1NodeGroupMembershipsWithResponse(context.Background(), &GetV1NodeGroupMembershipsParams{}, editor)
				return response == nil, err
			},
		},
		{
			name: "node removal policy", path: "/v1/node_removal_policy",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1NodeRemovalPolicyWithResponse(context.Background(), editor)
				return response == nil, err
			},
		},
		{
			name: "nodegroup options", path: "/v1/nodegroups/options",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1NodegroupsOptionsWithResponse(context.Background(), editor)
				return response == nil, err
			},
		},
		{
			name: "node history", path: "/v1/nodes/history",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1NodesHistoryWithResponse(context.Background(), &GetV1NodesHistoryParams{}, editor)
				return response == nil, err
			},
		},
		{
			name: "node options", path: "/v1/nodes/options",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1NodesOptionsWithResponse(context.Background(), &GetV1NodesOptionsParams{}, editor)
				return response == nil, err
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			transportCalls := 0
			doer := authStatusTestDoer(func(_ *http.Request) (*http.Response, error) {
				transportCalls++
				return nil, errors.New("unexpected transport call")
			})
			client, err := NewClientWithResponses("http://example.test", WithHTTPClient(doer))
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			editorCalls := 0
			responseNil, err := tc.call(client, func(_ context.Context, req *http.Request) error {
				editorCalls++
				if req.Method != http.MethodGet || req.URL.Path != tc.path {
					t.Errorf("request = %s %s, want GET %s", req.Method, req.URL.Path, tc.path)
				}
				return sentinel
			})
			if !errors.Is(err, sentinel) {
				t.Errorf("error = %v, want sentinel", err)
			}
			if !responseNil {
				t.Error("response is non-nil, want nil")
			}
			if editorCalls != 1 || transportCalls != 0 {
				t.Errorf("editor calls = %d, transport calls = %d; want 1 and 0", editorCalls, transportCalls)
			}

			malformedClient, err := NewClientWithResponses("http://[::1", WithHTTPClient(doer))
			if err != nil {
				t.Fatalf("new malformed-server client: %v", err)
			}
			editorCalls, transportCalls = 0, 0
			responseNil, err = tc.call(malformedClient, func(_ context.Context, _ *http.Request) error {
				editorCalls++
				return nil
			})
			if err == nil {
				t.Error("malformed server: expected URL construction error")
			}
			if !responseNil {
				t.Error("malformed server: response is non-nil, want nil")
			}
			if editorCalls != 0 || transportCalls != 0 {
				t.Errorf("malformed server: editor calls = %d, transport calls = %d; want 0 and 0", editorCalls, transportCalls)
			}
		})
	}
}

func TestRound2RemainingClientConstructionFailures(t *testing.T) {
	const id = "11111111-1111-4111-8111-111111111111"
	sentinel := errors.New("editor rejected request")
	tests := []struct {
		name, method, path string
		call               func(*ClientWithResponses, RequestEditorFn) (bool, error)
	}{
		{
			name: "delete mute rules",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.DeleteV1AlertMuteRulesWithBodyWithResponse(context.Background(), "application/json", strings.NewReader("{}"), editor)
				return response == nil, err
			},
		},
		{
			name: "put mute rules",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.PutV1AlertMuteRulesWithBodyWithResponse(context.Background(), "application/json", strings.NewReader("{}"), editor)
				return response == nil, err
			},
		},
		{
			name: "put compute zones",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.PutV1ComputezonesWithBodyWithResponse(context.Background(), "application/json", strings.NewReader("{}"), editor)
				return response == nil, err
			},
		},
		{
			name: "put node removal policy",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.PutV1NodeRemovalPolicyWithBodyWithResponse(context.Background(), "application/json", strings.NewReader("{}"), editor)
				return response == nil, err
			},
		},
		{
			name: "delete nodes",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.DeleteV1NodesWithBodyWithResponse(context.Background(), &DeleteV1NodesParams{}, "application/json", strings.NewReader("{}"), editor)
				return response == nil, err
			},
		},
		{
			name: "put node tags",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.PutV1NodesNodeUuidTagsWithBodyWithResponse(context.Background(), id, "application/json", strings.NewReader("{}"), editor)
				return response == nil, err
			},
		},
		{
			name: "post notification preferences",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.PostV1NotificationPreferencesWithBodyWithResponse(context.Background(), "application/json", strings.NewReader("{}"), editor)
				return response == nil, err
			},
		},
		{
			name: "put notification preference",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.PutV1NotificationPreferenceByIDWithBodyWithResponse(context.Background(), id, "application/json", strings.NewReader("{}"), editor)
				return response == nil, err
			},
		},
		{
			name: "put retention policy",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.PutV1RetentionPolicyWithBodyWithResponse(context.Background(), "application/json", strings.NewReader("{}"), editor)
				return response == nil, err
			},
		},
		{
			name: "patch tags",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.PatchV1TagsWithBodyWithResponse(context.Background(), "application/json", strings.NewReader("{}"), editor)
				return response == nil, err
			},
		},
		{
			name: "auth status",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1AuthStatusWithResponse(context.Background(), editor)
				return response == nil, err
			},
		},
		{
			name: "delete node", method: http.MethodDelete, path: "/v1/nodes/" + id,
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.DeleteV1NodesNodeUuidWithResponse(context.Background(), id, &DeleteV1NodesNodeUuidParams{}, editor)
				return response == nil, err
			},
		},
		{
			name: "node detail", method: http.MethodGet, path: "/v1/nodes/" + id,
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1NodesNodeUuidWithResponse(context.Background(), id, &GetV1NodesNodeUuidParams{}, editor)
				return response == nil, err
			},
		},
		{
			name: "node health history", method: http.MethodGet, path: "/v1/nodes/" + id + "/health_history",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1NodesNodeUuidHealthHistoryWithResponse(context.Background(), id, &GetV1NodesNodeUuidHealthHistoryParams{}, editor)
				return response == nil, err
			},
		},
		{
			name: "retention policy", method: http.MethodGet, path: "/v1/retention_policy",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1RetentionPolicyWithResponse(context.Background(), editor)
				return response == nil, err
			},
		},
		{
			name: "xid bursts", method: http.MethodGet, path: "/v1/xid/bursts",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1XIDBurstsWithResponse(context.Background(), &GetV1XIDBurstsParams{}, editor)
				return response == nil, err
			},
		},
		{
			name: "xid burst options", method: http.MethodGet, path: "/v1/xid/bursts/options",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1XIDBurstOptionsWithResponse(context.Background(), &GetV1XIDBurstOptionsParams{}, editor)
				return response == nil, err
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			transportCalls := 0
			doer := authStatusTestDoer(func(_ *http.Request) (*http.Response, error) {
				transportCalls++
				return nil, errors.New("unexpected transport call")
			})
			client, err := NewClientWithResponses("http://[::1", WithHTTPClient(doer))
			if err != nil {
				t.Fatalf("new malformed-server client: %v", err)
			}
			editorCalls := 0
			responseNil, err := tc.call(client, func(_ context.Context, _ *http.Request) error {
				editorCalls++
				return nil
			})
			if err == nil {
				t.Error("malformed server: expected URL construction error")
			}
			if !responseNil {
				t.Error("malformed server: response is non-nil, want nil")
			}
			if editorCalls != 0 || transportCalls != 0 {
				t.Errorf("malformed server: editor calls = %d, transport calls = %d; want 0 and 0", editorCalls, transportCalls)
			}

			if tc.method == "" {
				return
			}
			client, err = NewClientWithResponses("http://example.test", WithHTTPClient(doer))
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			editorCalls, transportCalls = 0, 0
			responseNil, err = tc.call(client, func(_ context.Context, req *http.Request) error {
				editorCalls++
				if req.Method != tc.method || req.URL.Path != tc.path {
					t.Errorf("request = %s %s, want %s %s", req.Method, req.URL.Path, tc.method, tc.path)
				}
				return sentinel
			})
			if !errors.Is(err, sentinel) {
				t.Errorf("error = %v, want sentinel", err)
			}
			if !responseNil {
				t.Error("response is non-nil, want nil")
			}
			if editorCalls != 1 || transportCalls != 0 {
				t.Errorf("editor calls = %d, transport calls = %d; want 1 and 0", editorCalls, transportCalls)
			}
		})
	}
}

func TestRound2AlertTimelineCountOnlyRequest(t *testing.T) {
	active, yes, no := true, true, false
	for _, tc := range []struct {
		name      string
		countOnly *bool
		query     url.Values
	}{
		{name: "true", countOnly: &yes, query: url.Values{"active": {"true"}, "countOnly": {"true"}}},
		{name: "false", countOnly: &no, query: url.Values{"active": {"true"}, "countOnly": {"false"}}},
		{name: "nil", query: url.Values{"active": {"true"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, err := NewGetV1AlertTimelineNodesRequest("http://example.test", &GetV1AlertTimelineNodesParams{Active: &active, CountOnly: tc.countOnly})
			if err != nil {
				t.Fatalf("construct request: %v", err)
			}
			if req.Method != http.MethodGet || req.URL.Path != "/v1/alert_timeline/nodes" {
				t.Errorf("request = %s %s, want GET /v1/alert_timeline/nodes", req.Method, req.URL.Path)
			}
			if got := req.URL.Query(); !reflect.DeepEqual(got, tc.query) {
				t.Errorf("query = %v, want %v", got, tc.query)
			}
		})
	}
}

func TestRound2V2MetricsRequestFailures(t *testing.T) {
	params := &GetV2MetricsParams{GroupBy: GetV2MetricsParamsGroupByNode, MetricNames: []string{"gpu_util"}, Step: "5m"}
	transportCalls := 0
	doer := authStatusTestDoer(func(_ *http.Request) (*http.Response, error) {
		transportCalls++
		return nil, errors.New("unexpected transport call")
	})
	client, err := NewClientWithResponses("http://[::1", WithHTTPClient(doer))
	if err != nil {
		t.Fatalf("new malformed-server client: %v", err)
	}
	editorCalls := 0
	response, err := client.GetV2MetricsWithResponse(context.Background(), params, func(_ context.Context, _ *http.Request) error {
		editorCalls++
		return nil
	})
	if err == nil || response != nil {
		t.Errorf("malformed server: response = %v, error = %v; want nil response and construction error", response, err)
	}
	if editorCalls != 0 || transportCalls != 0 {
		t.Errorf("malformed server: editor calls = %d, transport calls = %d; want 0 and 0", editorCalls, transportCalls)
	}

	client, err = NewClientWithResponses("http://example.test", WithHTTPClient(doer))
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	sentinel := errors.New("editor rejected request")
	response, err = client.GetV2MetricsWithResponse(context.Background(), params, func(_ context.Context, req *http.Request) error {
		editorCalls++
		if req.Method != http.MethodGet || req.URL.Path != "/v2/metrics" {
			t.Errorf("request = %s %s, want GET /v2/metrics", req.Method, req.URL.Path)
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) || response != nil {
		t.Errorf("editor failure: response = %v, error = %v; want nil response and sentinel", response, err)
	}
	if editorCalls != 1 || transportCalls != 0 {
		t.Errorf("editor failure: editor calls = %d, transport calls = %d; want 1 and 0", editorCalls, transportCalls)
	}
}

func TestRound2GeneratedIOAndEditorFailurePropagation(t *testing.T) {
	transportCalls := 0
	doer := authStatusTestDoer(func(_ *http.Request) (*http.Response, error) {
		transportCalls++
		return nil, errors.New("unexpected transport call")
	})
	client, err := NewClientWithResponses("http://example.test", WithHTTPClient(doer))
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	editorSentinel := errors.New("editor rejected compute zones request")
	editorCalls := 0
	response, err := client.PutV1ComputezonesWithBodyWithResponse(context.Background(), "application/json", strings.NewReader(`{}`), func(_ context.Context, req *http.Request) error {
		editorCalls++
		if req.Method != http.MethodPut || req.URL.Path != "/v1/computezones" {
			t.Errorf("request = %s %s, want PUT /v1/computezones", req.Method, req.URL.Path)
		}
		if got := req.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", got)
		}
		return editorSentinel
	})
	if response != nil || !errors.Is(err, editorSentinel) {
		t.Errorf("editor failure: response = %v, error = %v; want nil response and sentinel", response, err)
	}
	if editorCalls != 1 || transportCalls != 0 {
		t.Errorf("editor failure: editor calls = %d, transport calls = %d; want 1 and 0", editorCalls, transportCalls)
	}

	readSentinel := errors.New("signing key body read failed")
	reader := &round2AlertTimelineErrorReader{err: readSentinel}
	parsed, err := ParseGetWellKnownSigningKeyPubResponse(&http.Response{Body: reader})
	if parsed != nil || !errors.Is(err, readSentinel) {
		t.Errorf("body read failure: response = %v, error = %v; want nil response and sentinel", parsed, err)
	}
	if !reader.closed {
		t.Error("body read failure: response body was not closed")
	}
}

func TestRound2PredictedHealthRankingQueryContracts(t *testing.T) {
	domainUnit := GetV1PredictedHealthRankingsParamsRankUnitNvlinkDomain
	rackUnit := GetV1PredictedHealthRankingsParamsRankUnitRack
	nodeUnit := GetV1PredictedHealthRankingsParamsRankUnitNode
	nthMax := NthMax
	countAbove := CountAbove
	requireAll := RequireAll
	aggregationN := 2
	threshold := float32(0.5)
	maxRank := 5
	nodeGroups := []string{"group-1", "group-2"}
	zones := []string{"zone-1"}
	domains := []string{"domain-1"}
	excludedNodes := []string{"node-3"}
	returnDomains := []string{"domain-1"}
	chassis := []string{"rack-a", "rack-b"}
	returnChassis := []string{"rack-a"}
	deciles := []int{1, 3}
	returnNodes := []string{"node-1", "node-2"}
	returnTags := []string{"team:a", "env:prod"}
	healthStatuses := []GetV1PredictedHealthRankingsParamsHealthStatuses{
		GetV1PredictedHealthRankingsParamsHealthStatusesHealthy,
		GetV1PredictedHealthRankingsParamsHealthStatusesDegraded,
	}

	cases := []struct {
		name   string
		params GetV1PredictedHealthRankingsParams
		query  url.Values
	}{
		{
			name: "domain nth max",
			params: GetV1PredictedHealthRankingsParams{
				RankUnit: &domainUnit, Aggregation: &nthMax, AggregationN: &aggregationN,
				MissingScorePolicy: &requireAll, NodeGroupIds: &nodeGroups, ComputeZoneIds: &zones,
				NvlinkDomainIds: &domains, ExcludeNodeUUIDs: &excludedNodes, ReturnNvlinkDomainIds: &returnDomains,
			},
			query: url.Values{
				"rankUnit": {"nvlinkDomain"}, "aggregation": {"nthMax"}, "aggregationN": {"2"},
				"missingScorePolicy": {"requireAll"}, "nodeGroupIds": {"group-1", "group-2"},
				"computeZoneIds": {"zone-1"}, "nvlinkDomainIds": {"domain-1"},
				"excludeNodeUUIDs": {"node-3"}, "returnNvlinkDomainIds": {"domain-1"},
			},
		},
		{
			name: "rack count above",
			params: GetV1PredictedHealthRankingsParams{
				RankUnit: &rackUnit, Aggregation: &countAbove, Threshold: &threshold,
				ChassisSNs: &chassis, ReturnChassisSNs: &returnChassis, MaxRank: &maxRank, Deciles: &deciles,
			},
			query: url.Values{
				"rankUnit": {"rack"}, "aggregation": {"countAbove"}, "threshold": {"0.5"},
				"chassisSNs": {"rack-a", "rack-b"}, "returnChassisSNs": {"rack-a"},
				"maxRank": {"5"}, "deciles": {"1", "3"},
			},
		},
		{
			name: "node return filters",
			params: GetV1PredictedHealthRankingsParams{
				RankUnit: &nodeUnit, ReturnNodeUUIDs: &returnNodes, ReturnTags: &returnTags, HealthStatuses: &healthStatuses,
			},
			query: url.Values{
				"rankUnit": {"node"}, "returnNodeUUIDs": {"node-1", "node-2"},
				"returnTags": {"team:a", "env:prod"}, "healthStatuses": {"Healthy", "Degraded"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := NewGetV1PredictedHealthRankingsRequest("https://example.test/", &tc.params)
			if err != nil {
				t.Fatalf("construct request: %v", err)
			}
			if req == nil {
				t.Fatal("construct request returned nil request")
			}
			if req.Method != http.MethodGet {
				t.Errorf("method = %q, want GET", req.Method)
			}
			if req.URL.Path != "/v1/predicted_health_rankings" {
				t.Errorf("path = %q, want /v1/predicted_health_rankings", req.URL.Path)
			}
			if got := req.URL.Query(); !reflect.DeepEqual(got, tc.query) {
				t.Errorf("query = %v, want %v", got, tc.query)
			}
		})
	}
}

func TestRound2RemainingQueryFilterContracts(t *testing.T) {
	const server = "https://example.test/"
	yes, no := true, false
	zone := "zone-legacy"
	nodeAgent := GetV1NodesParamsAgentTypeOob
	inventoryAgent := GetV1ReportsInventoryParamsAgentTypeOob
	checks := []ModelsIntegrityCheck{IntegrityCheckPassed, IntegrityCheckFailed}
	groups := []string{"group-1", "group-2"}
	gpuTypes := []string{"H100", "A100"}

	cases := []struct {
		name  string
		path  string
		query url.Values
		call  func() (*http.Request, error)
	}{
		{
			name: "metrics psirt true", path: "/v1/metrics", query: url.Values{"psirtComponent": {"true"}},
			call: func() (*http.Request, error) {
				return NewGetV1MetricsRequest(server, &GetV1MetricsParams{PsirtComponent: &yes})
			},
		},
		{
			name: "metrics psirt false", path: "/v1/metrics", query: url.Values{"psirtComponent": {"false"}},
			call: func() (*http.Request, error) {
				return NewGetV1MetricsRequest(server, &GetV1MetricsParams{PsirtComponent: &no})
			},
		},
		{
			name: "nodegroups legacy zone", path: "/v1/nodegroups", query: url.Values{"computeZoneId": {"zone-legacy"}},
			call: func() (*http.Request, error) {
				return NewGetV1NodegroupsRequest(server, &GetV1NodegroupsParams{ComputeZoneId: &zone})
			},
		},
		{
			name: "nodes agent type", path: "/v1/nodes", query: url.Values{"agentType": {"oob"}},
			call: func() (*http.Request, error) {
				return NewGetV1NodesRequest(server, &GetV1NodesParams{AgentType: &nodeAgent})
			},
		},
		{
			name: "nodes integrity checks", path: "/v1/nodes", query: url.Values{"integrityChecks": {"Verified", "Unverified"}},
			call: func() (*http.Request, error) {
				return NewGetV1NodesRequest(server, &GetV1NodesParams{IntegrityChecks: &checks})
			},
		},
		{
			name: "inventory agent type", path: "/v1/reports/inventory", query: url.Values{"agentType": {"oob"}},
			call: func() (*http.Request, error) {
				return NewGetV1ReportsInventoryRequest(server, &GetV1ReportsInventoryParams{AgentType: &inventoryAgent})
			},
		},
		{
			name: "security vulnerability filters", path: "/v1/reports/security/vulnerabilities",
			query: url.Values{"nodeGroupIds": {"group-1", "group-2"}, "gpuTypes": {"H100", "A100"}},
			call: func() (*http.Request, error) {
				return NewGetV1ReportsSecurityVulnerabilitiesRequest(server, &GetV1ReportsSecurityVulnerabilitiesParams{NodeGroupIds: &groups, GpuTypes: &gpuTypes})
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := tc.call()
			if err != nil {
				t.Fatalf("construct request: %v", err)
			}
			if req == nil {
				t.Fatal("construct request returned nil request")
			}
			if req.Method != http.MethodGet {
				t.Errorf("method = %q, want GET", req.Method)
			}
			if req.URL.Path != tc.path {
				t.Errorf("path = %q, want %q", req.URL.Path, tc.path)
			}
			if got := req.URL.Query(); !reflect.DeepEqual(got, tc.query) {
				t.Errorf("query = %v, want %v", got, tc.query)
			}
		})
	}
}

func TestRound2DeleteNodeAgentTypeRequest(t *testing.T) {
	agentType := DeleteV1NodesNodeUuidParamsAgentTypeOob
	req, err := NewDeleteV1NodesNodeUuidRequest("https://example.test/", "node-1", &DeleteV1NodesNodeUuidParams{AgentType: &agentType})
	if err != nil {
		t.Fatalf("construct request: %v", err)
	}
	if req.Method != http.MethodDelete || req.URL.Path != "/v1/nodes/node-1" {
		t.Errorf("request = %s %s, want DELETE /v1/nodes/node-1", req.Method, req.URL.Path)
	}
	want := url.Values{"agentType": {"oob"}}
	if got := req.URL.Query(); !reflect.DeepEqual(got, want) {
		t.Errorf("query = %v, want %v", got, want)
	}
}

func TestRound2V2MetricsNameFilters(t *testing.T) {
	group := "training group"
	zone := "zone-1"
	cases := []struct {
		name   string
		params GetV2MetricsParams
		query  url.Values
	}{
		{
			name:   "legacy group name",
			params: GetV2MetricsParams{GroupBy: GetV2MetricsParamsGroupByNodegroup, GroupName: &group, MetricNames: []string{"gpu_util"}, Step: "5m"},
			query:  url.Values{"groupBy": {"nodegroup"}, "groupName": {"training group"}, "metricNames": {"gpu_util"}, "step": {"5m"}},
		},
		{
			name:   "node group and compute zone ID",
			params: GetV2MetricsParams{GroupBy: GetV2MetricsParamsGroupByNodegroup, NodeGroupName: &group, ComputeZoneId: &zone, MetricNames: []string{"gpu_util"}, Step: "5m"},
			query:  url.Values{"groupBy": {"nodegroup"}, "nodeGroupName": {"training group"}, "computeZoneId": {"zone-1"}, "metricNames": {"gpu_util"}, "step": {"5m"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := NewGetV2MetricsRequest("https://example.test/", &tc.params)
			if err != nil {
				t.Fatalf("construct request: %v", err)
			}
			if req.Method != http.MethodGet || req.URL.Path != "/v2/metrics" {
				t.Errorf("request = %s %s, want GET /v2/metrics", req.Method, req.URL.Path)
			}
			if got := req.URL.Query(); !reflect.DeepEqual(got, tc.query) {
				t.Errorf("query = %v, want %v", got, tc.query)
			}
		})
	}
}

func TestRound2WithBaseURLRequest(t *testing.T) {
	client, err := NewClient("https://original.example/", WithBaseURL("https://replacement.example/api"))
	if err != nil {
		t.Fatalf("construct client: %v", err)
	}
	if client.Server != "https://replacement.example/api/" {
		t.Errorf("server = %q, want https://replacement.example/api/", client.Server)
	}
	req, err := NewGetV1NodesRequest(client.Server, nil)
	if err != nil {
		t.Fatalf("construct request: %v", err)
	}
	if req.Method != http.MethodGet || req.URL.String() != "https://replacement.example/api/v1/nodes" {
		t.Errorf("request = %s %s, want GET https://replacement.example/api/v1/nodes", req.Method, req.URL)
	}
}

func TestRound2ComputeZoneNonfiniteRequestRejection(t *testing.T) {
	for _, coordinate := range []string{"latitude", "longitude"} {
		t.Run(coordinate, func(t *testing.T) {
			latitude, longitude := float32(39.5), float32(-76.25)
			if coordinate == "latitude" {
				latitude = float32(math.NaN())
			} else {
				longitude = float32(math.NaN())
			}
			body := ModelsUpdateComputeZoneRequest{
				Id:       "cz-1",
				Location: &ModelsLocation{Latitude: &latitude, Longitude: &longitude},
			}

			req, err := NewPutV1ComputezonesRequest("https://example.test", body)
			if req != nil {
				t.Errorf("request = %v, want nil", req)
			}
			var unsupported *json.UnsupportedValueError
			if !errors.As(err, &unsupported) {
				t.Errorf("request error = %v, want *json.UnsupportedValueError", err)
			}

			doerCalled, editorCalled := false, false
			doer := authStatusTestDoer(func(_ *http.Request) (*http.Response, error) {
				doerCalled = true
				return nil, errors.New("unexpected transport call")
			})
			client, err := NewClientWithResponses("https://example.test", WithHTTPClient(doer))
			if err != nil {
				t.Fatalf("NewClientWithResponses: %v", err)
			}
			editor := func(_ context.Context, _ *http.Request) error {
				editorCalled = true
				return nil
			}
			response, err := client.PutV1ComputezonesWithResponse(context.Background(), body, editor)
			if response != nil {
				t.Errorf("response = %v, want nil", response)
			}
			unsupported = nil
			if !errors.As(err, &unsupported) {
				t.Errorf("client error = %v, want *json.UnsupportedValueError", err)
			}
			if editorCalled {
				t.Error("request editor called for non-finite coordinate")
			}
			if doerCalled {
				t.Error("HTTP doer called for non-finite coordinate")
			}
		})
	}
}
