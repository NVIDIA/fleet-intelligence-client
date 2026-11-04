// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestErrorReportSources(t *testing.T) {
	for _, source := range []DataSource{"", DataSourceInband, DataSourceOOB} {
		for _, tc := range []struct {
			name   string
			view   ErrorReportView
			group  ErrorReportGroupBy
			format ReportFormat
			body   string
		}{
			{"nodes", ErrorReportViewList, ErrorReportGroupByNode, ReportFormatJSON, ""},
			{"errors", ErrorReportViewList, ErrorReportGroupByError, ReportFormatJSON, `{"errors":[{"name":"Xid","count":2}],"total":1}`},
			{"graph", ErrorReportViewGraph, "", ReportFormatJSON, `{"result":[]}`},
			{"overview", ErrorReportViewOverview, "", ReportFormatJSON, `{"totalErrors":2}`},
			{"nodes-csv", ErrorReportViewList, ErrorReportGroupByNode, ReportFormatCSV, "Node UUID,Node Name,BMC MAC\nn1,machine-1,02:00:0a:00:00:0a\n"},
			{"errors-csv", ErrorReportViewList, ErrorReportGroupByError, ReportFormatCSV, "Error,Count\nXid,2\n"},
		} {
			t.Run(string(source)+"/"+tc.name, func(t *testing.T) {
				body := tc.body
				if tc.name == "nodes" {
					identity := `"hostname":"gpu-001"`
					if source == DataSourceOOB {
						identity = `"nodeName":"machine-1","bmcMac":"02:00:0a:00:00:0a"`
					}
					body = fmt.Sprintf(`{"nodes":[{"nodeUUID":"n1",%s,"errors":["Xid"]},{"nodeUUID":"n2","errors":[]}],"total":2,"page":0}`, identity)
				}
				requests := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					requests++
					q := r.URL.Query()
					if r.URL.Path != "/v1/reports/error" || q.Get("dataSource") != string(source) || q.Has("dataSource") != (source != "") || q.Has("agentType") {
						t.Errorf("unexpected source request: %s", r.URL)
					}
					if q.Get("window") != "24h" || q.Get("view") != string(tc.view) {
						t.Errorf("lost report options: %s", r.URL)
					}
					contentType := "application/json"
					if tc.format == ReportFormatCSV {
						contentType = "text/csv"
					}
					if r.Header.Get("Accept") != contentType {
						t.Errorf("unexpected Accept: %s", r.Header.Get("Accept"))
					}
					w.Header().Set("Content-Type", contentType)
					fmt.Fprint(w, body)
				}))
				defer server.Close()
				client, err := NewClient(server.URL, "test-key")
				if err != nil {
					t.Fatal(err)
				}
				report, err := client.GetErrorReport(context.Background(), ErrorReportOptions{
					DataSource: source, View: tc.view, GroupBy: tc.group, Format: tc.format,
					TimeMode: ErrorReportTimeModeRelative, Window: "24h",
				})
				if err != nil {
					t.Fatal(err)
				}
				if report.DataSource != source || requests != 1 {
					t.Fatalf("source=%q requests=%d", report.DataSource, requests)
				}
				raw := report.RawJSON
				if tc.format == ReportFormatCSV {
					raw = report.RawCSV
				}
				if string(raw) != body {
					t.Fatalf("payload changed: %s", raw)
				}
				if tc.name == "nodes" {
					n := report.Nodes[0]
					if source == DataSourceOOB {
						if n.NodeName != "machine-1" || n.BMCMAC != "02:00:0a:00:00:0a" || n.Hostname != "" {
							t.Fatalf("lost OOB identity: %#v", n)
						}
					} else if n.Hostname != "gpu-001" || n.NodeName != "" || n.BMCMAC != "" {
						t.Fatalf("unexpected in-band identity: %#v", n)
					}
					if n := report.Nodes[1]; n.Hostname != "" || n.NodeName != "" || n.BMCMAC != "" {
						t.Fatalf("invented missing identity: %#v", n)
					}
				}
			})
		}
	}
}

func TestErrorReportRejectsInvalidSourcesBeforeHTTP(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer server.Close()
	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []DataSource{DataSourceAll, "inband,oob", "invalid"} {
		_, err := client.GetErrorReport(context.Background(), ErrorReportOptions{
			DataSource: source, View: ErrorReportViewOverview, Window: "24h", TimeMode: ErrorReportTimeModeRelative,
		})
		if err == nil || !strings.Contains(err.Error(), "data source") {
			t.Fatalf("source %q: %v", source, err)
		}
	}
	if requests != 0 {
		t.Fatalf("invalid sources made %d requests", requests)
	}
}
