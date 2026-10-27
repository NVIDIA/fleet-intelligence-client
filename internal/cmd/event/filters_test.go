// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package event

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
)

func TestEventFilterCommands(t *testing.T) {
	for _, endpoint := range []string{"list", "buckets"} {
		for _, format := range []string{"table", "json"} {
			for _, all := range []bool{false, true} {
				if endpoint == "buckets" && all {
					continue
				}
				t.Run(fmt.Sprintf("%s/%s/all=%v", endpoint, format, all), func(t *testing.T) {
					t.Setenv("HOME", t.TempDir())
					calls := 0
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						q := r.URL.Query()
						expected := map[string][]string{
							"dataSources": {"inband", "oob"}, "nameSearch": {"bmc"}, "eventNames": {"XID", "Others"}, "severities": {"Critical", "Info"}, "components": {"gpu", "oob-redfish-events"}, "gpuTypes": {"H100", "B200"}, "nodeGroupIds": {"ng1", "ng2"}, "excludeComputeZoneIds": {"cz1", "cz2"},
						}
						for key, want := range expected {
							if !reflect.DeepEqual(q[key], want) {
								t.Errorf("%s=%v; want %v", key, q[key], want)
							}
						}
						if q.Get("window") != "24h" {
							t.Errorf("window=%v", q)
						}
						w.Header().Set("Content-Type", "application/json")
						if endpoint == "buckets" {
							if r.URL.Path != "/v1/events/buckets" || q.Has("sortBy") || q.Has("sortOrder") {
								t.Errorf("unexpected URL %s", r.URL)
							}
							_, _ = w.Write([]byte(`{"bucketInterval":"1h","buckets":[{"count":2}]}`))
						} else {
							if r.URL.Path != "/v1/events" || q.Get("sortBy") != "bmcMac" || q.Get("sortOrder") != "asc" {
								t.Errorf("unexpected URL %s", r.URL)
							}
							if all && q.Get("page") != fmt.Sprint(calls) {
								t.Errorf("page=%s, want %d", q.Get("page"), calls)
							}
							fmt.Fprintf(w, `{"events":[{"eventId":"e%d","nodeUUID":"n1","dataSource":"oob","hostname":"os-1","nodeName":"machine-1","bmcMac":"bmc-1","gpuType":"H100","nodeGroup":"group","nodeGroupId":"ng1","computeZone":"zone","computeZoneId":"cz1"}],"page":%d,"pageSize":1,"total":2,"hasMore":%t}`, calls, calls, calls == 0)
						}
						calls++
					}))
					defer server.Close()
					cmdtest.SaveConfig(t, server.URL, "test-key")
					args := []string{"event", endpoint, "--window", "24h", "--output", format, "--data-source", "inband, oob", "--hostname", " bmc ", "--event-names", "XID,Others", "--severity", "Critical,Info", "--components", "gpu,oob-redfish-events", "--gpu-type", "H100,B200", "--nodegroup-ids", "ng1,ng2", "--exclude-compute-zone-ids", "cz1,cz2"}
					if endpoint == "list" {
						args = append(args, "--sort-by", "bmcMac", "--order", "asc", "--page-size", "1")
					}
					if all {
						args = append(args, "--all")
					}
					cmd := newRootCmd()
					var out bytes.Buffer
					cmd.SetOut(&out)
					cmd.SetArgs(args)
					if err := cmd.Execute(); err != nil {
						t.Fatal(err)
					}
					wantCalls := 1
					if all {
						wantCalls = 2
					}
					if calls != wantCalls {
						t.Fatalf("requests=%d; want %d", calls, wantCalls)
					}
					if endpoint == "list" {
						if format == "table" {
							for _, want := range []string{"DATA SOURCE", "oob"} {
								if !strings.Contains(out.String(), want) {
									t.Errorf("missing %s in %s", want, &out)
								}
							}
						} else {
							var result struct {
								Events []map[string]any `json:"events"`
								Items  []map[string]any `json:"items"`
							}
							if err := json.Unmarshal(out.Bytes(), &result); err != nil {
								t.Fatal(err)
							}
							if all {
								result.Events = result.Items
							}
							if len(result.Events) != wantCalls {
								t.Fatalf("events=%v", result.Events)
							}
							for _, event := range result.Events {
								for key, want := range map[string]string{"dataSource": "oob", "hostname": "os-1", "nodeName": "machine-1", "bmcMac": "bmc-1", "gpuType": "H100", "nodeGroup": "group", "nodeGroupId": "ng1", "computeZone": "zone", "computeZoneId": "cz1"} {
									if event[key] != want {
										t.Errorf("%s=%v; want %s", key, event[key], want)
									}
								}
							}
						}
					} else if !strings.Contains(out.String(), "1h") {
						t.Errorf("missing bucket interval: %s", &out)
					}
				})
			}
		}
	}
}

func TestEventRejectsInvalidFiltersBeforeAuthentication(t *testing.T) {
	for _, endpoint := range []string{"list", "buckets"} {
		for _, args := range [][]string{
			{"--data-source", "invalid"}, {"--data-source", "inband,"}, {"--components", "gpu,,other"},
			{"--component", "gpu", "--components", "gpu"},
			{"--nodegroup-ids", "ng1", "--exclude-nodegroup-ids", "ng2"},
			{"--compute-zone-ids", "cz1", "--exclude-compute-zone-ids", "cz2"},
			{"--sort-by", "invalid"}, {"--order", "up"},
		} {
			t.Run(endpoint+strings.Join(args, " "), func(t *testing.T) {
				t.Setenv("HOME", t.TempDir())
				cmd := newRootCmd()
				cmd.SetOut(new(bytes.Buffer))
				cmd.SetArgs(append([]string{"event", endpoint, "--window", "24h"}, args...))
				err := cmd.Execute()
				if err == nil || strings.Contains(err.Error(), "auth") {
					t.Fatalf("expected local filter error, got %v", err)
				}
			})
		}
	}
}

func TestEventOptionsCommand(t *testing.T) {
	for _, format := range []string{"table", "json"} {
		t.Run(format, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			body := `{"components":[{"component":"oob-redfish-events","componentDisplayName":"Redfish Events"}],"dataSources":["oob"],"eventNames":["Others"],"gpuTypes":["H100"],"severities":["Info"],"sortFields":["timestamp","bmcMac"]}`
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/events/options" || r.URL.RawQuery != "" {
					t.Errorf("unexpected URL %s", r.URL)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")
			cmd := newRootCmd()
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetArgs([]string{"event", "options", "--output", format})
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			if format == "json" {
				if strings.TrimSpace(out.String()) != body {
					t.Fatalf("JSON changed: %s", &out)
				}
			} else {
				for _, want := range []string{"--event-names", "--severity", "--components", "--data-source", "--gpu-type", "--sort-by", "event list only", "Others", "oob-redfish-events", "Redfish Events", "bmcMac"} {
					if !strings.Contains(out.String(), want) {
						t.Errorf("missing %q in %s", want, &out)
					}
				}
			}
		})
	}
}
