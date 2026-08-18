// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
)

// Verifies table output, filters, and sort flags
func TestNodeGroupListTableFiltersAndSort(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/nodegroups" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		query := r.URL.Query()
		if got := query.Get("view"); got != "detail" {
			t.Fatalf("unexpected view: %q", got)
		}
		if got := query.Get("includeMetrics"); got != "false" {
			t.Fatalf("unexpected includeMetrics: %q", got)
		}
		if got := query["computeZoneIds"]; !slices.Equal(got, []string{"cz-1", "cz-2"}) {
			t.Fatalf("unexpected computeZoneIds: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query["computeZoneNames"]; !slices.Equal(got, []string{"East"}) {
			t.Fatalf("unexpected computeZoneNames: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query["nodeGroupIds"]; !slices.Equal(got, []string{"ng-1", "ng-2"}) {
			t.Fatalf("unexpected nodeGroupIds: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query["healthStatuses"]; !slices.Equal(got, []string{"Healthy", "Degraded"}) {
			t.Fatalf("unexpected healthStatuses: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query["gpuTypes"]; !slices.Equal(got, []string{"NVIDIA-H100", "NVIDIA-A100"}) {
			t.Fatalf("unexpected gpuTypes: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query.Get("sortBy"); got != "health" {
			t.Fatalf("unexpected sortBy: %q", got)
		}
		if got := query.Get("order"); got != "asc" {
			t.Fatalf("unexpected order: %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodeGroups":[{"id":"ng-1","name":"Training","computeZoneId":"cz-1","computeZoneName":"East","healthState":"Healthy","healthPercentage":95.5,"nodesCount":8}],"hasMore":false,"page":0,"pageSize":20,"total":1}`))
	}))
	defer server.Close()

	saveTestConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"nodegroup", "list", "--include-metrics=false", "--compute-zone-ids", "cz-1,cz-2", "--compute-zone-names", "East", "--nodegroup-ids", "ng-1,ng-2", "--health", "Healthy,Degraded", "--gpu-type", "NVIDIA-H100,NVIDIA-A100", "--sort-by", "health", "--order", "asc"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{"ID", "NAME", "COMPUTE ZONE", "HEALTH", "HEALTH PERCENTAGE", "NODE COUNT", "ng-1", "Training", "East", "Healthy", "95.5%", "8"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q: %q", want, got)
		}
	}
}

// Verifies all-page JSON output
func TestNodeGroupListAllJSONMergesRawItems(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/nodegroups" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("view"); got != "basic" {
			t.Fatalf("unexpected view: %q", got)
		}
		if got := r.URL.Query().Get("pageSize"); got != "1" {
			t.Fatalf("unexpected pageSize: %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("page") {
		case "0":
			requests++
			_, _ = w.Write([]byte(`{"nodeGroups":[{"id":"ng-1","name":"Training","extra":"kept"}],"hasMore":true,"page":0,"pageSize":1,"total":2}`))
		case "1":
			requests++
			_, _ = w.Write([]byte(`{"nodeGroups":[{"id":"ng-2","name":"Inference"}],"hasMore":false,"page":1,"pageSize":1,"total":2}`))
		default:
			t.Fatalf("unexpected page: %q", r.URL.Query().Get("page"))
		}
	}))
	defer server.Close()

	saveTestConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"nodegroup", "list", "--view", "basic", "--all", "--output", "json", "--page-size", "1"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	var got struct {
		Items      []map[string]any `json:"items"`
		Pagination struct {
			Page         int  `json:"page"`
			PageSize     int  `json:"pageSize"`
			Total        int  `json:"total"`
			HasMore      bool `json:"hasMore"`
			PagesFetched int  `json:"pagesFetched"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, out.String())
	}
	if requests != 2 {
		t.Fatalf("unexpected request count: %d", requests)
	}
	if len(got.Items) != 2 || got.Items[0]["id"] != "ng-1" || got.Items[0]["extra"] != "kept" {
		t.Fatalf("unexpected merged items: %#v", got.Items)
	}
	if got.Pagination.Page != 1 || got.Pagination.PageSize != 1 || got.Pagination.Total != 2 || got.Pagination.HasMore || got.Pagination.PagesFetched != 2 {
		t.Fatalf("unexpected pagination: %#v", got.Pagination)
	}
}

// Verifies basic node group rows
func TestWriteNodeGroupBasicTable(t *testing.T) {
	var out bytes.Buffer
	err := writeNodeGroupTable(&out, string(nvfleetint.NodeGroupViewBasic), []nvfleetint.NodeGroup{
		{ID: "ng-1", Name: "Training"},
	})
	if err != nil {
		t.Fatalf("write table failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{"ID", "NAME", "ng-1", "Training"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q: %q", want, got)
		}
	}
}

// Verifies node group list help explains sort behavior
func TestNodeGroupListHelpExplainsOrderDefaultSort(t *testing.T) {
	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"nodegroup", "list", "--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("help command failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"Run 'nvfleetint nodegroup options' to list accepted values for --compute-zone-ids, --nodegroup-ids, --health, --gpu-type, --sort-by, and --order.",
		"Sort order for --sort-by; node groups default --sort-by to health",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("help output missing %q: %q", want, got)
		}
	}
}

// Verifies local flag validation
func TestNodeGroupListRejectsInvalidFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "view", args: []string{"nodegroup", "list", "--view", "wide"}, want: "invalid view"},
		{name: "health", args: []string{"nodegroup", "list", "--health", "Broken"}, want: "invalid health"},
		{name: "sort", args: []string{"nodegroup", "list", "--sort-by", "name"}, want: "invalid sort-by"},
		{name: "order", args: []string{"nodegroup", "list", "--order", "up"}, want: "invalid order"},
		{name: "basic filter", args: []string{"nodegroup", "list", "--view", "basic", "--health", "Healthy"}, want: "basic node group view is incompatible"},
		{name: "basic compute zone name", args: []string{"nodegroup", "list", "--view", "basic", "--compute-zone-names", "East"}, want: "basic node group view is incompatible with compute zone name"},
		{name: "basic include metrics", args: []string{"nodegroup", "list", "--view", "basic", "--include-metrics=false"}, want: "basic node group view is incompatible with --include-metrics"},
		{name: "basic health sort", args: []string{"nodegroup", "list", "--view", "basic", "--sort-by", "health"}, want: "basic node group view is incompatible with sort"},
		{name: "basic nodes sort", args: []string{"nodegroup", "list", "--view", "basic", "--sort-by", "nodes"}, want: "basic node group view is incompatible with sort"},
		{name: "compute zone IDs", args: []string{"nodegroup", "list", "--compute-zone-ids", "cz-1,,cz-2"}, want: "empty values are not allowed"},
		{name: "gpu types", args: []string{"nodegroup", "list", "--gpu-type", "NVIDIA-H100,,NVIDIA-A100"}, want: "empty values are not allowed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newRootCmd()
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetArgs(tt.args)

			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("unexpected error: got %v want %q", err, tt.want)
			}
		})
	}
}

// Verifies node group options render compute zones under their new flag while
// still pointing nested node groups at --nodegroup-ids.
func TestNodeGroupOptionsOutput(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	body := `{"filters":{"fields":[` +
		`{"name":"computeZones","options":[{"id":"cz-1","value":"East","options":[{"id":"ng-1","value":"Training"}]}]},` +
		`{"name":"healthStatuses","options":["Healthy","Degraded"]}` +
		`]},"sorting":{"fields":["health","nodes"],"orders":["asc","desc"],"defaults":{"field":"health","order":"desc"}}}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/nodegroups/options" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	saveTestConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"nodegroup", "options"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("nodegroup options command failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"Filters for 'nodegroup list'",
		"\n--compute-zone-ids\n  cz-1  East\n",
		// Nested node groups are promoted into the flag that accepts them.
		"\n--nodegroup-ids\n  ng-1  Training  (in East)\n",
		"--health",
		"--sort-by  (default: health)", "\n  health\n  nodes\n",
		"--order  (default: desc)", "\n  asc\n  desc\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("table output missing %q:\n%s", want, got)
		}
	}

	var jsonOut bytes.Buffer
	jsonCmd := newRootCmd()
	jsonCmd.SetOut(&jsonOut)
	jsonCmd.SetArgs([]string{"nodegroup", "options", "--output", "json"})
	if err := jsonCmd.Execute(); err != nil {
		t.Fatalf("nodegroup options JSON command failed: %v", err)
	}
	if strings.TrimSpace(jsonOut.String()) != body {
		t.Fatalf("JSON output is not the raw payload:\n%s", jsonOut.String())
	}
}
