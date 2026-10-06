// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNodeSourceRequestsAndResponses(t *testing.T) {
	for _, tc := range []struct {
		name   string
		source NodeDataSource
		want   string
	}{
		{name: "default"},
		{name: "inband", source: NodeDataSourceInband, want: "inband"},
		{name: "oob", source: NodeDataSourceOOB, want: "oob"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			source := tc.want
			if source == "" {
				source = "inband"
			}
			node := fmt.Sprintf(`{"nodeUUID":"n1","dataSource":%q,"agentType":"obsolete","gpuType":"H100","gpuCount":8,"nodeName":"machine","bmcHostname":"bmc","bmcIP":"192.0.2.1","enrolledAt":"2026-01-01T00:00:00Z","removedAt":"2026-09-01T00:00:00Z","verificationCheck":"Verified","location":{"country":"US"}}`, source)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if got := r.URL.Query().Get("dataSource"); got != tc.want {
					t.Errorf("source = %q, want %q", got, tc.want)
				}
				if r.URL.Query().Has("agentType") {
					t.Error("request sends deprecated agentType")
				}
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/v1/nodes/n1":
					fmt.Fprint(w, node)
				case "/v1/nodes/options":
					if r.URL.Query().Get("view") != "basic" {
						t.Error("missing basic options view")
					}
					fmt.Fprint(w, `{"filters":{"fields":[]},"sorting":{"fields":["gpuType"]}}`)
				default:
					fmt.Fprintf(w, `{"nodes":[%s],"total":1,"page":0,"pageSize":20}`, node)
				}
			}))
			defer server.Close()
			client, err := NewClient(server.URL, "test-key")
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			page, err := client.ListNodes(ctx, ListNodesOptions{DataSource: tc.source})
			if err != nil {
				t.Fatal(err)
			}
			detail, err := client.DescribeNodeWithOptions(ctx, "n1", DescribeNodeOptions{DataSource: tc.source})
			if err != nil {
				t.Fatal(err)
			}
			for _, n := range []Node{page.Nodes[0], detail.Node} {
				if n.DataSource != source || n.GPUType != "H100" || n.GPUCount == nil || *n.GPUCount != 8 {
					t.Fatalf("lost node source/GPU fields: %#v", n)
				}
				encoded, err := json.Marshal(n)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(encoded), `"agentType"`) {
					t.Fatalf("node exposes removed agentType: %s", encoded)
				}
			}
			if _, err := client.GetNodeFilterOptionsWithOptions(ctx, NodeFilterOptionsRequest{View: NodeViewBasic, DataSource: tc.source}); err != nil {
				t.Fatal(err)
			}
			report, err := client.GetInventoryReport(ctx, InventoryReportOptions{DataSource: tc.source})
			if err != nil {
				t.Fatal(err)
			}
			if source == "oob" {
				n := report.Nodes[0]
				if report.DataSource != NodeDataSourceOOB || n.NodeName != "machine" || n.BMCHostname != "bmc" || n.BMCIP != "192.0.2.1" || n.EnrolledAt == "" || n.RemovedAt == "" || n.VerificationCheck != "Verified" || n.Location == nil {
					t.Fatalf("lost OOB report fields: %#v", report)
				}
			}
			if requests != 4 {
				t.Fatalf("requests = %d, want 4", requests)
			}
		})
	}
}

func TestNodeSourcesRejectInvalidValuesBeforeHTTP(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer server.Close()
	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		source NodeDataSource
		want   string
	}{
		{"all", "invalid node data source"},
		{"bad", "invalid node data source"},
		{"sideband", "invalid node data source"},
	} {
		ctx := context.Background()
		_, listErr := client.ListNodes(ctx, ListNodesOptions{DataSource: tc.source})
		_, detailErr := client.DescribeNodeWithOptions(ctx, "n1", DescribeNodeOptions{DataSource: tc.source})
		_, optionsErr := client.GetNodeFilterOptionsWithOptions(ctx, NodeFilterOptionsRequest{DataSource: tc.source})
		_, reportErr := client.GetInventoryReport(ctx, InventoryReportOptions{DataSource: tc.source})
		for _, err := range []error{listErr, detailErr, optionsErr, reportErr} {
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		}
	}
	if requests != 0 {
		t.Fatalf("invalid options made %d HTTP requests", requests)
	}
	if err := (ListNodesOptions{AgentStatuses: []NodeAgentStatus{NodeAgentUnknown}}).Validate(); err == nil || !strings.Contains(err.Error(), "Online or Offline") {
		t.Fatalf("unknown status validation: %v", err)
	}
	if err := (NodeFilterOptionsRequest{View: "bad"}).Validate(); err == nil {
		t.Fatal("invalid options view accepted")
	}
}

func TestOOBInventoryReportDownloadSource(t *testing.T) {
	for _, signed := range []bool{false, true} {
		t.Run(fmt.Sprintf("signed=%v", signed), func(t *testing.T) {
			body := "nodeUUID,nodeName,bmcHostname\nn1,machine,bmc\n"
			contentType := "text/csv"
			if signed {
				body = "signed bundle bytes"
				contentType = "application/zip"
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query()
				if q.Get("dataSource") != "oob" || q.Get("format") != "csv" || q.Has("agentType") {
					t.Errorf("unexpected query: %s", r.URL.RawQuery)
				}
				if signed && q.Get("signed") != "true" {
					t.Error("missing signed flag")
				}
				w.Header().Set("Content-Type", contentType)
				fmt.Fprint(w, body)
			}))
			defer server.Close()
			client, err := NewClient(server.URL, "test-key")
			if err != nil {
				t.Fatal(err)
			}
			report, err := client.GetInventoryReport(context.Background(), InventoryReportOptions{DataSource: NodeDataSourceOOB, Format: ReportFormatCSV, Signed: signed})
			if err != nil {
				t.Fatal(err)
			}
			got := report.RawCSV
			if signed {
				got = report.RawSigned
			}
			if string(got) != body {
				t.Fatalf("download changed: %q", got)
			}
		})
	}
}
