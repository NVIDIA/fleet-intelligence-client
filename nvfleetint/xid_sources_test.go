// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestXIDBurstSources(t *testing.T) {
	for _, source := range []DataSource{"", DataSourceInband, DataSourceOOB, DataSourceAll} {
		t.Run(string(source), func(t *testing.T) {
			row := `{"burstId":"b1","nodeUuid":"n1","dataSource":"oob","hostname":"os-1","nodeName":"machine-1","bmcMac":"bmc-1","gpuType":"H100","deviceIds":{"GPU-or-Redfish-label":[48]}}`
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query()
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/v1/xid/bursts/b1" {
					if len(q) != 0 {
						t.Errorf("describe must not send source: %v", q)
					}
					_, _ = w.Write([]byte(row))
					return
				}
				if q.Get("dataSource") != string(source) || q.Has("dataSource") != (source != "") {
					t.Errorf("unexpected source query: %v", q)
				}
				if r.URL.Path == "/v1/xid/bursts/options" {
					if q.Has("timeMode") || q.Has("window") {
						t.Errorf("unscoped options sent time: %v", q)
					}
					echo := source
					if echo == "" {
						echo = DataSourceInband
					}
					fmt.Fprintf(w, `{"dataSource":%q,"gpuTypes":["H100","B200"]}`, echo)
					return
				}
				if r.URL.Path != "/v1/xid/bursts" {
					t.Errorf("path=%s", r.URL.Path)
				}
				if !reflect.DeepEqual(q["gpuTypes"], []string{"H100", "B200"}) || q.Get("nameSearch") != "machine" || q.Has("hostnameSearch") {
					t.Errorf("filters=%v", q)
				}
				fmt.Fprintf(w, `{"items":[%s],"page":0,"pageSize":20,"total":1}`, row)
			}))
			defer server.Close()
			client, err := NewClient(server.URL, "test-key")
			if err != nil {
				t.Fatal(err)
			}
			page, err := client.ListXIDBursts(context.Background(), ListXIDBurstsOptions{Window: "24h", DataSource: source, GPUTypes: []string{"H100", "B200"}, NameSearch: " machine ", HostnameSearch: "ignored"})
			if err != nil {
				t.Fatal(err)
			}
			if len(page.Bursts) != 1 {
				t.Fatalf("bursts=%v", page.Bursts)
			}
			got := page.Bursts[0]
			if got.DataSource != "oob" || got.Hostname != "os-1" || got.NodeName != "machine-1" || got.BMCMAC != "bmc-1" || got.GPUType != "H100" {
				t.Fatalf("fields lost: %+v", got)
			}
			detail, err := client.DescribeXIDBurst(context.Background(), "b1")
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(detail.XIDBurst, got) {
				t.Fatalf("detail differs: %+v", detail)
			}
			if !reflect.DeepEqual(detail.DeviceIDs, map[string][]int{"GPU-or-Redfish-label": {48}}) {
				t.Fatalf("device labels changed: %v", detail.DeviceIDs)
			}
			options, err := client.GetXIDBurstFilterOptions(context.Background(), XIDBurstFilterOptionsScope{DataSource: source})
			if err != nil {
				t.Fatal(err)
			}
			want := source
			if want == "" {
				want = DataSourceInband
			}
			if options.DataSource != string(want) || !reflect.DeepEqual(options.GPUTypes, []string{"H100", "B200"}) {
				t.Fatalf("options=%+v", options)
			}
		})
	}
}

func TestXIDBurstRejectsInvalidSources(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("unexpected request")
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []DataSource{"invalid", "inband,oob", "ALL"} {
		if _, err := client.ListXIDBursts(context.Background(), ListXIDBurstsOptions{Window: "24h", DataSource: source}); err == nil {
			t.Errorf("list accepted %q", source)
		}
		if _, err := client.GetXIDBurstFilterOptions(context.Background(), XIDBurstFilterOptionsScope{DataSource: source}); err == nil {
			t.Errorf("options accepted %q", source)
		}
	}
	// The XID-only all selector must not widen source contracts elsewhere.
	if err := DataSourceAll.Validate(); err == nil {
		t.Fatal("single-source validation accepted all")
	}
	if err := validateDataSources([]DataSource{DataSourceAll}); err == nil {
		t.Fatal("source array validation accepted all")
	}
}
