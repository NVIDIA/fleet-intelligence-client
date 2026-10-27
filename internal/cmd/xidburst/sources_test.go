// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package xidburst

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

func TestXIDBurstSourceListAndOptions(t *testing.T) {
	for _, source := range []string{"", "inband", "oob", "all"} {
		for _, format := range []string{"table", "json"} {
			for _, allPages := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/allPages=%v", source, format, allPages), func(t *testing.T) {
					t.Setenv("HOME", t.TempDir())
					calls := 0
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						q := r.URL.Query()
						w.Header().Set("Content-Type", "application/json")
						if q.Get("dataSource") != source || q.Has("dataSource") != (source != "") {
							t.Errorf("source query=%v", q)
						}
						if q.Get("window") != "24h" || q.Get("timeMode") != "relative" || !reflect.DeepEqual(q["nodeGroupIds"], []string{"ng1", "ng2"}) {
							t.Errorf("scope=%v", q)
						}
						echo := source
						if echo == "" {
							echo = "inband"
						}
						if r.URL.Path == "/v1/xid/bursts/options" {
							fmt.Fprintf(w, `{"dataSource":%q,"gpuTypes":["H100","B200"],"xidNumbers":[48]}`, echo)
							return
						}
						if r.URL.Path != "/v1/xid/bursts" || q.Get("sortBy") != "dataSource" || q.Get("sortOrder") != "asc" || q.Get("nameSearch") != "bmc" || q.Has("hostnameSearch") || !reflect.DeepEqual(q["gpuTypes"], []string{"H100", "B200"}) {
							t.Errorf("list request=%s", r.URL)
						}
						if allPages && q.Get("page") != fmt.Sprint(calls) {
							t.Errorf("page=%s; want %d", q.Get("page"), calls)
						}
						if echo == "all" {
							echo = []string{"inband", "oob"}[calls]
						}
						fmt.Fprintf(w, `{"items":[{"burstId":"b%d","nodeUuid":"same-node","dataSource":%q,"nodeName":"machine-1","bmcMac":"bmc-1","gpuType":"H100"}],"page":%d,"pageSize":1,"total":2}`, calls, echo, calls)
						calls++
					}))
					defer server.Close()
					cmdtest.SaveConfig(t, server.URL, "test-key")
					scope := []string{"--window", "24h", "--nodegroup-ids", "ng1,ng2", "--output", format}
					if source != "" {
						scope = append(scope, "--data-source", source)
					}
					cmd := newRootCmd()
					var out bytes.Buffer
					cmd.SetOut(&out)
					args := append([]string{"xidburst", "list"}, scope...)
					args = append(args, "--gpu-type", "H100,B200", "--hostname", "bmc", "--sort-by", "dataSource", "--order", "asc", "--page-size", "1")
					if allPages {
						args = append(args, "--all")
					}
					cmd.SetArgs(args)
					if err := cmd.Execute(); err != nil {
						t.Fatal(err)
					}
					wantRows := 1
					if allPages {
						wantRows = 2
					}
					if calls != wantRows {
						t.Fatalf("calls=%d; want %d", calls, wantRows)
					}
					if format == "json" {
						var result struct {
							Items []map[string]any `json:"items"`
						}
						if err := json.Unmarshal(out.Bytes(), &result); err != nil {
							t.Fatal(err)
						}
						if len(result.Items) != wantRows {
							t.Fatalf("lost observations: %s", &out)
						}
						for i, item := range result.Items {
							wantSource := source
							if wantSource == "" {
								wantSource = "inband"
							}
							if wantSource == "all" {
								wantSource = []string{"inband", "oob"}[i]
							}
							for key, want := range map[string]string{"dataSource": wantSource, "nodeName": "machine-1", "bmcMac": "bmc-1", "gpuType": "H100", "nodeUuid": "same-node"} {
								if item[key] != want {
									t.Errorf("%s=%v; want %s", key, item[key], want)
								}
							}
						}
					} else {
						for _, want := range []string{"DATA SOURCE", "NODE NAME", "BMC MAC", "GPU TYPE", "machine-1", "bmc-1", "H100"} {
							if !strings.Contains(out.String(), want) {
								t.Errorf("missing %s in %s", want, &out)
							}
						}
					}
					cmd = newRootCmd()
					out.Reset()
					cmd.SetOut(&out)
					cmd.SetArgs(append([]string{"xidburst", "options"}, scope...))
					if err := cmd.Execute(); err != nil {
						t.Fatal(err)
					}
					if format == "json" {
						var result map[string]any
						if err := json.Unmarshal(out.Bytes(), &result); err != nil {
							t.Fatal(err)
						}
						wantSource := source
						if wantSource == "" {
							wantSource = "inband"
						}
						if result["dataSource"] != wantSource {
							t.Errorf("options=%v", result)
						}
					} else {
						for _, want := range []string{"Data source:", "--gpu-type", "H100", "B200"} {
							if !strings.Contains(out.String(), want) {
								t.Errorf("missing %s in %s", want, &out)
							}
						}
					}
				})
			}
		}
	}
}

func TestXIDBurstOOBDescribe(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/xid/bursts/oob-burst" || r.URL.RawQuery != "" {
			t.Errorf("describe request=%s", r.URL)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"burstId":"oob-burst","nodeUuid":"n1","dataSource":"oob","nodeName":"machine-1","bmcMac":"bmc-1","gpuType":"H100","deviceIds":{"Redfish-token":[48]}}`))
	}))
	defer server.Close()
	cmdtest.SaveConfig(t, server.URL, "test-key")
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"xidburst", "describe", "oob-burst"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"DATA SOURCE", "oob", "NODE NAME", "machine-1", "BMC MAC", "bmc-1", "GPU TYPE", "H100", "Redfish-token"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %s in %s", want, &out)
		}
	}
}

func TestXIDBurstInvalidSourceBeforeAuthentication(t *testing.T) {
	for _, command := range []string{"list", "options"} {
		for _, source := range []string{"invalid", "inband,oob", "ALL"} {
			t.Run(command+"/"+source, func(t *testing.T) {
				t.Setenv("HOME", t.TempDir())
				cmd := newRootCmd()
				cmd.SetOut(new(bytes.Buffer))
				cmd.SetArgs([]string{"xidburst", command, "--window", "24h", "--data-source", source})
				if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "--data-source") {
					t.Fatalf("expected source validation error, got %v", err)
				}
			})
		}
	}
}
