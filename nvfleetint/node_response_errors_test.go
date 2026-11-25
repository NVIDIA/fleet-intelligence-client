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
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/generated/fleetapi"
)

func TestNodeResponseDecodingErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		view   NodeView
		source DataSource
		detail bool
	}{
		{"basic list", NodeViewBasic, DataSourceInband, false},
		{"inband detail list", NodeViewDetail, DataSourceInband, false},
		{"oob detail list", NodeViewDetail, DataSourceOOB, false},
		{"inband describe", "", DataSourceInband, true},
		{"oob describe", "", DataSourceOOB, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				path := "/v1/nodes"
				if tc.detail {
					path += "/node-1"
				}
				if r.Method != http.MethodGet || r.URL.Path != path || r.URL.Query().Get("view") != string(tc.view) || r.URL.Query().Get("dataSource") != string(tc.source) {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
				}
				w.Header().Set("Content-Type", "text/plain")
				if tc.detail {
					_, _ = w.Write([]byte(`{"gpuCount":"not-an-integer"}`))
				} else {
					_, _ = w.Write([]byte(`{"page":"not-an-integer"}`))
				}
			}))
			defer server.Close()
			client, err := NewClient(server.URL, "test-key")
			if err != nil {
				t.Fatal(err)
			}
			if tc.detail {
				got, err := client.DescribeNodeWithOptions(context.Background(), "node-1", DescribeNodeOptions{DataSource: tc.source})
				var decodeErr *json.UnmarshalTypeError
				if !errors.As(err, &decodeErr) || !reflect.DeepEqual(got, NodeDetails{}) {
					t.Fatalf("got result %#v and error %v; want zero result and decoding error", got, err)
				}
			} else {
				got, err := client.ListNodes(context.Background(), ListNodesOptions{View: tc.view, DataSource: tc.source})
				var decodeErr *json.UnmarshalTypeError
				if !errors.As(err, &decodeErr) || !reflect.DeepEqual(got, NodesPage{}) {
					t.Fatalf("got result %#v and error %v; want zero result and decoding error", got, err)
				}
			}
		})
	}
}

func TestOOBInventoryOptionalTopologyAndLinks(t *testing.T) {
	const payload = `{"source":{"address":"192.0.2.10","mac":"02:00:00:00:00:01","sourceType":"redfish"},"systems":[{"id":"system-1","links":{"missing":null,"empty":[],"related":[{"odataId":"/redfish/v1/Chassis/1"}]},"processors":[{"id":"gpu-1","nvidiaGpu":{}},{"id":"gpu-2","nvidiaGpu":{"nvlinkTopology":{"chassisSerialNumber":"chassis-1","ibGuid":"ib-1","lsbIbGuid":"lsb-1","systemGuid":"system-1","traySerialNumber":"tray-1","hostId":0}}}]}]}`
	var input fleetapi.ModelsOobInventory
	if err := json.Unmarshal([]byte(payload), &input); err != nil {
		t.Fatal(err)
	}
	got := oobInventoryFromGenerated(&input)
	if got == nil || len(got.Systems) != 1 || len(got.Systems[0].Processors) != 2 {
		t.Fatalf("unexpected inventory: %#v", got)
	}
	system := got.Systems[0]
	if system.Processors[0].NvidiaGPU == nil || system.Processors[0].NvidiaGPU.NVLinkTopology != nil {
		t.Fatalf("GPU without topology changed: %#v", system.Processors[0].NvidiaGPU)
	}
	gpu := system.Processors[1].NvidiaGPU
	if gpu == nil || gpu.NVLinkTopology == nil {
		t.Fatalf("missing GPU topology: %#v", gpu)
	}
	topology := gpu.NVLinkTopology
	if topology.ChassisSerialNumber != "chassis-1" || topology.IBGUID != "ib-1" || topology.LSBIBGUID != "lsb-1" || topology.SystemGUID != "system-1" || topology.TraySerialNumber != "tray-1" || topology.HostID == nil || *topology.HostID != 0 {
		t.Fatalf("unexpected topology: %#v", topology)
	}
	missing, missingOK := system.Links["missing"]
	empty, emptyOK := system.Links["empty"]
	related, relatedOK := system.Links["related"]
	if !missingOK || missing != nil || !emptyOK || empty == nil || len(empty) != 0 || !relatedOK || len(related) != 1 || related[0].ODataID != "/redfish/v1/Chassis/1" {
		t.Fatalf("unexpected links: %#v", system.Links)
	}
	wantJSON := oobJSONValue(t, input)
	if actual := oobJSONValue(t, got); !reflect.DeepEqual(actual, wantJSON) {
		t.Fatalf("SDK inventory changed JSON representation\ngot: %#v\nwant: %#v", actual, wantJSON)
	}
	generatedSystem := &(*input.Systems)[0]
	(*generatedSystem.Links)["related"][0].OdataId = "/changed"
	*(*generatedSystem.Processors)[1].NvidiaGpu.NvlinkTopology.HostId = 42
	if related[0].ODataID != "/redfish/v1/Chassis/1" || *topology.HostID != 0 || !reflect.DeepEqual(oobJSONValue(t, got), wantJSON) {
		t.Fatal("SDK inventory aliases generated reference or topology")
	}
}
