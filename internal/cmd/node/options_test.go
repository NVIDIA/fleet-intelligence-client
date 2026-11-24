// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package node

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
)

func TestNodeListOptionsSupportsNodeName(t *testing.T) {
	opts, err := nodeListOptions(nodeListFlags{
		view:     "detail",
		nodeName: " machine-001 ",
		sortBy:   "nodeName",
	})
	if err != nil {
		t.Fatalf("node list options failed: %v", err)
	}
	if opts.NodeName != "machine-001" || opts.SortBy != nvfleetint.NodeSortByNodeName {
		t.Fatalf("unexpected node-name options: %#v", opts)
	}
}

// Verifies node options render nested compute zones under the flags that accept
// them, and that JSON output stays the raw backend payload.
func TestNodeOptionsOutput(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	body := `{"filters":{"fields":[` +
		`{"name":"computeZones","options":[{"id":"cz-1","value":"East","options":[{"id":"ng-1","value":"Training"}]}]},` +
		`{"name":"gpuTypes","options":["NVIDIA-H100"]},` +
		`{"name":"brandNewFilter","options":["surprise"]}` +
		`]},"sorting":{"fields":["hostname","nodeGroup","computeZone","verificationCheck","integrityCheck"],"orders":["asc","desc"],"defaults":{"field":"integrityCheck","order":"asc"}}}`

	var dataSources []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/nodes/options" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		dataSources = append(dataSources, r.URL.Query().Get("dataSource"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var tableOut bytes.Buffer
	tableCmd := newRootCmd()
	tableCmd.SetOut(&tableOut)
	tableCmd.SetArgs([]string{"node", "options", "--data-source", "oob"})
	if err := tableCmd.Execute(); err != nil {
		t.Fatalf("node options command failed: %v", err)
	}

	got := tableOut.String()
	for _, want := range []string{
		"Filters for 'node list'",
		"--compute-zone-ids",
		// Nested node groups are promoted into their own section, tagged with
		// the compute zone they belong to.
		"\n--nodegroup-ids\n  ng-1  Training  (in East)\n",
		"cz-1", "East",
		"--gpu-type",
		"brandNewFilter  (no flag on 'node list')",
		// The backend spells two sort fields differently from the CLI.
		"nodegroup", "computezone", "verificationCheck",
		// The backend's default sort field is the deprecated "integrityCheck"
		// alias; naming it as the default would offer it as a choice.
		"\n--sort-by\n", "(default: asc)",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("table output missing %q:\n%s", want, got)
		}
	}
	// integrityCheck is a deprecated alias the API still advertises; offering it
	// would point new scripts at a field that is about to be removed, and it
	// must not be named as the --sort-by default either.
	for _, unwanted := range []string{"nodeGroup,", "computeZone,", "integrityCheck"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("table output should not offer %q:\n%s", unwanted, got)
		}
	}
	// The compute zone section lists only zones; the node group is not left
	// nested beneath its parent.
	if strings.Contains(got, "\n    ng-1") {
		t.Fatalf("node group should be promoted, not nested:\n%s", got)
	}

	var jsonOut bytes.Buffer
	jsonCmd := newRootCmd()
	jsonCmd.SetOut(&jsonOut)
	jsonCmd.SetArgs([]string{"node", "options", "--data-source", "inband", "--output", "json"})
	if err := jsonCmd.Execute(); err != nil {
		t.Fatalf("node options JSON command failed: %v", err)
	}
	if strings.TrimSpace(jsonOut.String()) != body {
		t.Fatalf("JSON output is not the raw payload:\n%s", jsonOut.String())
	}
	if len(dataSources) != 2 || dataSources[0] != "oob" || dataSources[1] != "inband" {
		t.Fatalf("unexpected dataSource values: %#v", dataSources)
	}
}

// Verifies a backend that returns the child list as its own field wins over
// promotion, so one flag never gets two sections, and that nested values under
// a field with no promotion target are still shown rather than dropped.
func TestNodeOptionsPromotionEdgeCases(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	body := `{"filters":{"fields":[` +
		`{"name":"computeZones","options":[{"id":"cz-1","value":"East","options":[{"id":"ng-1","value":"Training"}]}]},` +
		`{"name":"nodeGroups","options":[{"id":"ng-1","value":"Training"},{"id":"ng-2","value":"Serving"}]},` +
		`{"name":"brandNewFilter","options":[{"id":"p-1","value":"Parent","options":[{"id":"c-1","value":"Child"}]}]}` +
		`]},"sorting":{"fields":["hostname"],"orders":["asc"],"defaults":{"field":"hostname","order":"asc"}}}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"node", "options", "--data-source", "inband"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("node options command failed: %v", err)
	}

	got := out.String()
	if count := strings.Count(got, "\n--nodegroup-ids\n"); count != 1 {
		t.Fatalf("expected exactly one --nodegroup-ids section, got %d:\n%s", count, got)
	}
	// The flat field supplied it, so the section carries ng-2 and no membership
	// column promoted from the compute zone.
	if !strings.Contains(got, "ng-2") || strings.Contains(got, "(in East)") {
		t.Fatalf("flat nodeGroups field should win over promotion:\n%s", got)
	}
	// A nested value with nowhere to be promoted stays indented under its parent.
	if !strings.Contains(got, "\n    c-1") {
		t.Fatalf("unpromotable nested value was dropped:\n%s", got)
	}
	// The node groups nested under the compute zone belong to --nodegroup-ids,
	// so they must not be listed under --compute-zone-ids, where passing one
	// silently matches nothing. The section points at the flag instead.
	computeZones := cmdtest.SectionBody(t, got, "--compute-zone-ids")
	if strings.Contains(computeZones, "ng-1") {
		t.Fatalf("node group listed under --compute-zone-ids:\n%s", computeZones)
	}
	if !strings.Contains(computeZones, "Values nested under these are listed under --nodegroup-ids.") {
		t.Fatalf("--compute-zone-ids section does not point at --nodegroup-ids:\n%s", computeZones)
	}
}

func TestNodeOptionsSingleSourceHTTPFailures(t *testing.T) {
	for _, tc := range []struct {
		name       string
		args       []string
		wantView   string
		wantSource string
	}{
		{name: "explicit inband", args: []string{"node", "options", "--data-source", "inband"}, wantView: "detail", wantSource: "inband"},
		{name: "combined basic", args: []string{"node", "options", "--view", "basic"}, wantView: "basic"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			type request struct {
				method, path, view, source string
				hasSource                  bool
			}
			var mu sync.Mutex
			var requests []request
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query()
				mu.Lock()
				requests = append(requests, request{r.Method, r.URL.Path, q.Get("view"), q.Get("dataSource"), q.Has("dataSource")})
				mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"distinctive options denial"}`))
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")

			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), "distinctive options denial") {
				t.Errorf("error = %v, want server denial", err)
			}
			if out.Len() != 0 {
				t.Errorf("expected empty stdout, got %q", out.String())
			}
			mu.Lock()
			defer mu.Unlock()
			want := request{http.MethodGet, "/v1/nodes/options", tc.wantView, tc.wantSource, tc.wantSource != ""}
			if len(requests) != 1 || requests[0] != want {
				t.Errorf("requests = %+v, want [%+v]", requests, want)
			}
		})
	}
}
