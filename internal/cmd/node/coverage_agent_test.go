// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package node

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdutil"
	clioutput "github.com/NVIDIA/fleet-intelligence-client/internal/output"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
)

// These cases protect combined versus source-specific detail behavior for
// Node describe source selection, including optional OOB data (docs/cli.md).
func TestNodeDescribeSourceFailuresAndJSON(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		statuses   map[string]int
		counts     map[string]int
		wantErrors []string
		jsonMode   string
	}{
		{name: "combined inband forbidden", statuses: map[string]int{"inband": 403, "oob": 200}, counts: map[string]int{"inband": 1, "oob": 1}, wantErrors: []string{"fetch in-band node view", "source access denied"}},
		{name: "combined oob forbidden", statuses: map[string]int{"inband": 200, "oob": 403}, counts: map[string]int{"inband": 1, "oob": 1}, wantErrors: []string{"fetch OOB node view", "source access denied"}},
		{name: "both missing", statuses: map[string]int{"inband": 404, "oob": 404}, counts: map[string]int{"inband": 1, "oob": 1}, wantErrors: []string{`node "node-1" was not found in either the in-band or OOB view`}},
		{name: "explicit inband forbidden", args: []string{"--data-source", "inband"}, statuses: map[string]int{"inband": 403}, counts: map[string]int{"inband": 1, "oob": 0}, wantErrors: []string{"source access denied"}},
		{name: "oob section without inventory", args: []string{"--data-source", "oob", "--section", "systems"}, statuses: map[string]int{"oob": 200}, counts: map[string]int{"inband": 0, "oob": 1}, wantErrors: []string{"does not have an OOB view for the requested section"}},
		{name: "combined json with only oob", args: []string{"--output", "json"}, statuses: map[string]int{"inband": 404, "oob": 200}, counts: map[string]int{"inband": 1, "oob": 1}, jsonMode: "combined"},
		{name: "explicit inband json", args: []string{"--data-source", "inband", "--output", "json"}, statuses: map[string]int{"inband": 200}, counts: map[string]int{"inband": 1, "oob": 0}, jsonMode: "inband"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			var mu sync.Mutex
			counts := map[string]int{"inband": 0, "oob": 0}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/nodes/node-1" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
					http.Error(w, "unexpected request", http.StatusBadRequest)
					return
				}
				sources := r.URL.Query()["dataSource"]
				if len(sources) != 1 || (sources[0] != "inband" && sources[0] != "oob") {
					t.Errorf("unexpected dataSource query: %q", sources)
					http.Error(w, "unexpected source", http.StatusBadRequest)
					return
				}
				source := sources[0]
				mu.Lock()
				counts[source]++
				mu.Unlock()
				status, ok := tc.statuses[source]
				if !ok {
					t.Errorf("unexpected source request: %s", source)
					http.Error(w, "unexpected source", http.StatusBadRequest)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				switch status {
				case http.StatusForbidden:
					_, _ = w.Write([]byte(`{"error":"source access denied"}`))
				case http.StatusNotFound:
					_, _ = w.Write([]byte(`{"error":"source not found"}`))
				default:
					if source == "inband" {
						_, _ = w.Write([]byte(`{"nodeUUID":"node-1","dataSource":"inband","hostname":"gpu-001","futureField":{"preserved":true}}`))
					} else {
						_, _ = w.Write([]byte(`{"nodeUUID":"node-1","dataSource":"oob"}`))
					}
				}
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")

			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs(append([]string{"node", "describe", "node-1"}, tc.args...))
			err := cmd.Execute()
			if len(tc.wantErrors) > 0 {
				if err == nil {
					t.Fatal("expected command error")
				}
				for _, want := range tc.wantErrors {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error %q does not contain %q", err, want)
					}
				}
				if out.Len() != 0 {
					t.Errorf("expected empty stdout, got %q", out.String())
				}
			} else if err != nil {
				t.Errorf("command failed: %v", err)
			} else {
				var got map[string]any
				if err := json.Unmarshal(out.Bytes(), &got); err != nil {
					t.Errorf("decode output: %v", err)
				} else if tc.jsonMode == "combined" {
					if _, exists := got["inband"]; exists {
						t.Errorf("unexpected inband object: %#v", got)
					}
					oob, ok := got["oob"].(map[string]any)
					if !ok || oob["nodeUUID"] != "node-1" || oob["dataSource"] != "oob" {
						t.Errorf("unexpected OOB object: %#v", got["oob"])
					}
				} else {
					future, ok := got["futureField"].(map[string]any)
					if got["nodeUUID"] != "node-1" || got["hostname"] != "gpu-001" || !ok || future["preserved"] != true {
						t.Errorf("unexpected inband JSON: %#v", got)
					}
					for _, key := range []string{"inband", "oob"} {
						if _, exists := got[key]; exists {
							t.Errorf("unexpected %s wrapper: %#v", key, got)
						}
					}
				}
			}
			mu.Lock()
			defer mu.Unlock()
			for _, source := range []string{"inband", "oob"} {
				if counts[source] != tc.counts[source] {
					t.Errorf("%s requests = %d, want %d", source, counts[source], tc.counts[source])
				}
			}
		})
	}
}

// nodeRenderWriter records actual writes so failures can target a boundary
// even when tabwriter emits a table in multiple writes.
type nodeRenderWriter struct {
	output bytes.Buffer
	writes []string
	failAt int
	err    error
	after  int
}

func (w *nodeRenderWriter) Write(p []byte) (int, error) {
	if w.failAt >= 0 && len(w.writes) >= w.failAt {
		if len(w.writes) > w.failAt {
			w.after++
		}
		w.writes = append(w.writes, string(p))
		return 0, w.err
	}
	w.writes = append(w.writes, string(p))
	return w.output.Write(p)
}

func TestNodeListTableRenderingBoundaries(t *testing.T) {
	result := nodeListOutput{
		View:  string(nvfleetint.NodeViewBasic),
		Nodes: []nvfleetint.Node{{UUID: "node-list-1", Hostname: "host-list-1"}},
		Page:  &clioutput.Pagination{Page: 0, PageSize: 1, Total: 2},
	}
	common := cmdutil.Resolved{Output: clioutput.FormatTable}
	control := &nodeRenderWriter{failAt: -1}
	if err := writeNodeListOutput(control, common, result); err != nil {
		t.Fatalf("render list control: %v", err)
	}
	if !strings.Contains(control.output.String(), "node-list-1") || !strings.Contains(control.output.String(), "Page: 1") {
		t.Fatalf("control missing node or footer: %q", control.output.String())
	}
	if len(control.writes) == 0 || strings.Contains(control.writes[0], "Page:") {
		t.Fatalf("first write is not a table boundary: %q", control.writes)
	}
	sentinel := errors.New("list table write failed")
	w := &nodeRenderWriter{failAt: 0, err: sentinel}
	if err := writeNodeListOutput(w, common, result); !errors.Is(err, sentinel) {
		t.Errorf("error = %v, want sentinel", err)
	}
	if w.after != 0 || len(w.writes) != 1 || w.output.Len() != 0 {
		t.Errorf("writes after table failure: writes=%q accepted=%q", w.writes, w.output.String())
	}

	result.Page = nil
	var noPage bytes.Buffer
	if err := writeNodeListOutput(&noPage, common, result); err != nil {
		t.Fatalf("render list without page: %v", err)
	}
	if !strings.Contains(noPage.String(), "node-list-1") || strings.Contains(noPage.String(), "Page:") {
		t.Errorf("unexpected unpaginated output: %q", noPage.String())
	}
}

func TestNodeListDetailInfersOOBColumns(t *testing.T) {
	result := nodeListOutput{
		View:  "detail",
		Nodes: []nvfleetint.Node{{UUID: "node-oob-1", NodeName: "bmc-node-1"}},
	}
	var out bytes.Buffer
	if err := writeNodeListOutput(&out, cmdutil.Resolved{Output: clioutput.FormatTable}, result); err != nil {
		t.Fatalf("render inferred OOB list: %v", err)
	}
	for _, want := range []string{"NODE NAME", "NODE KIND", "BMC MAC", "BMC IP", "node-oob-1", "bmc-node-1"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q: %q", want, out.String())
		}
	}
	if strings.Contains(out.String(), "FIRMWARE CHECK") || strings.Contains(out.String(), "HOSTNAME") {
		t.Errorf("in-band columns in OOB output: %q", out.String())
	}
}

func TestNodeDescribeSummaryWriteBoundaries(t *testing.T) {
	inband := nvfleetint.NodeDetails{Node: nvfleetint.Node{UUID: "inband-node", DataSource: "inband"}}
	oob := nvfleetint.NodeDetails{Node: nvfleetint.Node{UUID: "oob-node", DataSource: "oob"}}
	result := nodeDescribeResult{Inband: &inband, OOB: &oob}
	options := nodeDescribeRenderOptions{showSummary: true}
	control := &nodeRenderWriter{failAt: -1}
	if err := writeNodeDescribeResultTable(control, result, "", options); err != nil {
		t.Fatalf("render combined control: %v", err)
	}
	full := control.output.String()
	positions := []int{strings.Index(full, "In-band\n"), strings.Index(full, "inband-node"), strings.Index(full, "\nOut-of-band\n"), strings.Index(full, "oob-node")}
	if positions[0] != 0 || positions[1] <= positions[0] || positions[2] <= positions[1] || positions[3] <= positions[2] {
		t.Fatalf("unexpected combined section order: %q", full)
	}
	for _, tc := range []struct {
		name   string
		offset int
		write  string
	}{
		{"in-band heading", 0, "In-band\n"},
		{"separator", positions[2], "\n"},
		{"out-of-band heading", positions[2] + 1, "Out-of-band\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			index, offset := -1, 0
			for i, part := range control.writes {
				if offset == tc.offset && part == tc.write {
					index = i
					break
				}
				offset += len(part)
			}
			if index < 0 {
				t.Fatalf("boundary %q at %d not found in writes %q", tc.write, tc.offset, control.writes)
			}
			sentinel := errors.New("describe section write failed")
			w := &nodeRenderWriter{failAt: index, err: sentinel}
			if err := writeNodeDescribeResultTable(w, result, "", options); !errors.Is(err, sentinel) {
				t.Errorf("error = %v, want sentinel", err)
			}
			if w.after != 0 || len(w.writes) != index+1 || w.output.String() != full[:tc.offset] {
				t.Errorf("unexpected writes after failure: accepted=%q writes=%q", w.output.String(), w.writes)
			}
		})
	}

	t.Run("explicit inband summary table", func(t *testing.T) {
		var expected bytes.Buffer
		if err := writeNodeDescribeResultTable(&expected, result, nvfleetint.DataSourceInband, options); err != nil {
			t.Fatalf("render explicit control: %v", err)
		}
		if !strings.Contains(expected.String(), "inband-node") || strings.Contains(expected.String(), "oob-node") {
			t.Fatalf("unexpected explicit output: %q", expected.String())
		}
		sentinel := errors.New("summary table write failed")
		w := &nodeRenderWriter{failAt: 0, err: sentinel}
		if err := writeNodeDescribeResultTable(w, result, nvfleetint.DataSourceInband, options); !errors.Is(err, sentinel) {
			t.Errorf("error = %v, want sentinel", err)
		}
		if w.after != 0 || len(w.writes) != 1 || w.output.Len() != 0 {
			t.Errorf("unexpected writes after summary failure: %q", w.writes)
		}
	})
}

type nodeSectionFailureWriter struct {
	accepted bytes.Buffer
	heading  string
	err      error
	reached  bool
	after    int
}

func (w *nodeSectionFailureWriter) Write(p []byte) (int, error) {
	if w.reached {
		w.after++
		return 0, w.err
	}
	if bytes.Contains(p, []byte(w.heading)) {
		w.reached = true
		return 0, w.err
	}
	return w.accepted.Write(p)
}

func TestNodeDescribeOOBSectionWriteErrors(t *testing.T) {
	var oob nvfleetint.NodeDetails
	if err := json.Unmarshal([]byte(completeOOBNodeJSON), &oob); err != nil {
		t.Fatalf("decode OOB fixture: %v", err)
	}
	if oob.OOBInventory == nil {
		t.Fatal("OOB fixture has no inventory")
	}
	options, err := parseOOBInventorySections(nodeDescribeFlags{sections: "all"}, nvfleetint.DataSourceOOB, "table")
	if err != nil {
		t.Fatalf("parse sections: %v", err)
	}

	titles := []string{"Managers", "Systems", "CPUs", "GPUs", "Memory Modules", "Chassis", "PCIe Devices", "Power Supplies", "Fabrics", "Switches", "Firmware"}
	result := nodeDescribeResult{OOB: &oob}
	var control bytes.Buffer
	if err := writeNodeDescribeResultTable(&control, result, nvfleetint.DataSourceOOB, options); err != nil {
		t.Fatalf("render OOB control: %v", err)
	}
	for _, want := range append(append([]string{}, titles...), "manager-1", "system-1", "cpu-1", "gpu-1", "dimm-1", "chassis-1", "pcie-1", "psu-1", "fabric-1", "switch-1", "firmware-1") {
		if !strings.Contains(control.String(), want) {
			t.Errorf("OOB control missing %q", want)
		}
	}

	checkFailure := func(t *testing.T, result nodeDescribeResult, source nvfleetint.DataSource, expected, title string) {
		t.Helper()
		heading := "\n" + title + "\n"
		index := strings.Index(expected, heading)
		if index < 0 {
			t.Fatalf("control missing heading %q", heading)
		}
		sentinel := errors.New("section write failed")
		writer := &nodeSectionFailureWriter{heading: heading, err: sentinel}
		err := writeNodeDescribeResultTable(writer, result, source, options)
		if !errors.Is(err, sentinel) {
			t.Errorf("error = %v, want sentinel", err)
		}
		if !writer.reached {
			t.Errorf("writer did not reach heading %q", heading)
		}
		if writer.after != 0 {
			t.Errorf("writer received %d writes after failure", writer.after)
		}
		if got := writer.accepted.String(); got != expected[:index] {
			t.Errorf("accepted output differs from control prefix before %q: got %q", title, got)
		}
	}
	for _, title := range titles {
		t.Run(title, func(t *testing.T) {
			checkFailure(t, result, nvfleetint.DataSourceOOB, control.String(), title)
		})
	}

	t.Run("combined sources", func(t *testing.T) {
		inband := nvfleetint.NodeDetails{}
		inband.DataSource = string(nvfleetint.DataSourceInband)
		combined := nodeDescribeResult{Inband: &inband, OOB: &oob}
		var combinedControl bytes.Buffer
		if err := writeNodeDescribeResultTable(&combinedControl, combined, "", options); err != nil {
			t.Fatalf("render combined control: %v", err)
		}
		checkFailure(t, combined, "", combinedControl.String(), "Switches")
	})
}

// Protect accurate Node list results and pagination error reporting when a
// request fails, including after a page has already been fetched.
func TestNodeListHTTPFailures(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantPages  []string
		wantErrors []string
	}{
		{name: "single page", args: []string{"--page", "1"}, wantPages: []string{"0"}, wantErrors: []string{"403"}},
		{name: "all pages after first page", args: []string{"--all", "--page-size", "1"}, wantPages: []string{"0", "1"}, wantErrors: []string{"403", "page 1", "after 1 completed pages"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			var mu sync.Mutex
			var pages []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				pages = append(pages, r.URL.Query().Get("page"))
				requestNumber := len(pages)
				mu.Unlock()
				if r.Method != http.MethodGet || r.URL.Path != "/v1/nodes" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				if len(tc.wantPages) == 2 && requestNumber == 1 {
					if got := r.URL.Query().Get("pageSize"); got != "1" {
						t.Errorf("pageSize = %q, want 1", got)
					}
					_, _ = w.Write([]byte(`{"nodes":[{"nodeUUID":"node-first"}],"page":0,"pageSize":1,"total":2,"hasMore":true}`))
					return
				}
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"node access denied"}`))
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")

			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs(append([]string{"node", "list", "--view", "basic", "--output", "json"}, tc.args...))
			err := cmd.Execute()
			if err == nil {
				t.Error("expected command error")
			} else {
				for _, want := range tc.wantErrors {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error %q does not contain %q", err, want)
					}
				}
			}
			if out.Len() != 0 {
				t.Errorf("expected empty output, got %q", out.String())
			}
			mu.Lock()
			defer mu.Unlock()
			if len(pages) != len(tc.wantPages) {
				t.Errorf("requested pages = %q, want %q", pages, tc.wantPages)
			} else {
				for i, want := range tc.wantPages {
					if pages[i] != want {
						t.Errorf("requested pages = %q, want %q", pages, tc.wantPages)
						break
					}
				}
			}
		})
	}
}

func TestNodeRejectsEmptyCommaListEntries(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantFlag string
	}{
		{name: "health", args: []string{"node", "list", "--data-source", "inband", "--health", "Healthy,,Degraded"}, wantFlag: "health"},
		{name: "agent status", args: []string{"node", "list", "--data-source", "inband", "--agent-status", "Online,,Offline"}, wantFlag: "agent-status"},
		{name: "verification check", args: []string{"node", "list", "--data-source", "inband", "--verification-check", "Verified,,Pending"}, wantFlag: "verification-check"},
		{name: "firmware check", args: []string{"node", "list", "--data-source", "inband", "--firmware-check", "Passed,,Failed"}, wantFlag: "firmware-check"},
		{name: "node UUIDs", args: []string{"node", "list", "--data-source", "inband", "--node-uuids", "node-1,,node-2"}, wantFlag: "node-uuids"},
		{name: "sections", args: []string{"node", "describe", "node-1", "--data-source", "oob", "--section", "systems,,chassis"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			var mu sync.Mutex
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				mu.Lock()
				requests++
				mu.Unlock()
				http.Error(w, "unexpected request", http.StatusInternalServerError)
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")

			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if err == nil {
				t.Error("expected command error")
			} else {
				if !strings.Contains(err.Error(), "empty values are not allowed") {
					t.Errorf("error %q does not identify empty list values", err)
				}
				if tc.wantFlag != "" && !strings.Contains(err.Error(), tc.wantFlag) {
					t.Errorf("error %q does not identify flag %q", err, tc.wantFlag)
				}
			}
			if out.Len() != 0 {
				t.Errorf("expected empty stdout, got %q", out.String())
			}
			mu.Lock()
			defer mu.Unlock()
			if requests != 0 {
				t.Errorf("server received %d requests, want none", requests)
			}
		})
	}
}

func TestNodeDescribeInbandResourceSummary(t *testing.T) {
	resourceRows := map[string]string{
		"CPU TYPE":            "Xeon-Example",
		"CPU MANUFACTURER":    "ExampleCorp",
		"CPU ARCHITECTURE":    "arm64",
		"CPU LOGICAL CORES":   "96",
		"MEMORY TOTAL BYTES":  "1099511627776",
		"CONTAINER ROOT DISK": "/dev/example0",
		"DISK DEVICES":        "2",
		"NIC INTERFACES":      "3",
	}
	cases := []struct {
		name      string
		response  string
		populated bool
	}{
		{
			name:      "populated resources",
			response:  `{"nodeUUID":"node-1","dataSource":"inband","resources":{"cpuInfo":{"type":"Xeon-Example","manufacturer":"ExampleCorp","architecture":"arm64","logicalCores":"96"},"memoryInfo":{"totalBytes":"1099511627776"},"diskInfo":{"containerRootDisk":"/dev/example0","blockDevices":[{"name":"disk0"},{"name":"disk1"}]},"nicInfo":{"privateIPInterfaces":[{"interface":"eth0"},{"interface":"eth1"},{"interface":"eth2"}]}}}`,
			populated: true,
		},
		{name: "absent resources", response: `{"nodeUUID":"node-1","dataSource":"inband"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			var mu sync.Mutex
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				requests++
				mu.Unlock()
				if r.Method != http.MethodGet || r.URL.Path != "/v1/nodes/node-1" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				if sources := r.URL.Query()["dataSource"]; len(sources) != 1 || sources[0] != "inband" {
					t.Errorf("dataSource = %q, want [inband]", sources)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.response))
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")

			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs([]string{"node", "describe", "node-1", "--data-source", "inband", "--output", "table"})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("command failed: %v", err)
			}
			mu.Lock()
			gotRequests := requests
			mu.Unlock()
			if gotRequests != 1 {
				t.Errorf("requests = %d, want 1", gotRequests)
			}

			rowValue := func(label string) (string, bool) {
				for _, line := range strings.Split(out.String(), "\n") {
					if strings.HasPrefix(line, label+"  ") {
						return strings.TrimSpace(strings.TrimPrefix(line, label)), true
					}
				}
				return "", false
			}
			if got, ok := rowValue("UUID"); !ok || got != "node-1" {
				t.Errorf("UUID row = %q (present %t), want node-1; output: %s", got, ok, out.String())
			}
			for label, want := range resourceRows {
				got, ok := rowValue(label)
				if tc.populated && (!ok || got != want) {
					t.Errorf("%s row = %q (present %t), want %q; output: %s", label, got, ok, want, out.String())
				} else if !tc.populated && ok {
					t.Errorf("unexpected %s row = %q", label, got)
				}
			}
		})
	}
}

func TestNodeHealthValidationAndHTTPFailure(t *testing.T) {
	cases := []struct {
		name     string
		uuid     string
		flags    []string
		want     []string
		requests int
	}{
		{name: "invalid output", uuid: "node-1", flags: []string{"--output", "yaml"}, want: []string{`invalid output "yaml": expected table or json`}},
		{name: "zero timeout", uuid: "node-1", flags: []string{"--timeout", "0s"}, want: []string{"--timeout must be greater than 0"}},
		{name: "blank UUID", uuid: " \t ", want: []string{"node UUID is required"}},
		{name: "missing profile", uuid: "node-1", flags: []string{"--profile", "nonexistent"}, want: []string{"profile not found", "nonexistent"}},
		{name: "forbidden", uuid: "node-1", want: []string{"403", "health access denied"}, requests: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			var mu sync.Mutex
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				requests++
				mu.Unlock()
				if r.Method != http.MethodGet || r.URL.Path != "/v1/nodes/node-1/health_history" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"health access denied"}`))
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")
			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			args := []string{"node", "health", tc.uuid, "--start", "2026-04-07T00:00:00Z", "--end", "2026-04-14T00:00:00Z"}
			cmd.SetArgs(append(args, tc.flags...))
			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected command error")
			}
			for _, want := range tc.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
			if out.Len() != 0 {
				t.Errorf("expected empty stdout, got %q", out.String())
			}
			mu.Lock()
			defer mu.Unlock()
			if requests != tc.requests {
				t.Errorf("requests = %d, want %d", requests, tc.requests)
			}
		})
	}
}

func TestNodeHealthEmptyHistoryRendering(t *testing.T) {
	for _, tc := range []struct {
		name    string
		body    string
		summary bool
	}{
		{name: "empty summary", body: `{"healthSummary":{},"machineStatus":[]}`, summary: true},
		{name: "absent summary", body: `{"machineStatus":[]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			var mu sync.Mutex
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				requests++
				mu.Unlock()
				if r.Method != http.MethodGet || r.URL.Path != "/v1/nodes/node-1/health_history" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")
			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs([]string{"node", "health", "node-1", "--start", "2026-04-07T00:00:00Z", "--end", "2026-04-14T00:00:00Z"})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("command failed: %v", err)
			}
			mu.Lock()
			gotRequests := requests
			mu.Unlock()
			if gotRequests != 1 {
				t.Errorf("requests = %d, want 1", gotRequests)
			}
			got := out.String()
			if strings.Contains(got, "Health Summary") != tc.summary {
				t.Errorf("unexpected summary heading: %q", got)
			}
			for _, want := range []string{"Machine Status", "No health timeline segments in the requested window."} {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q: %q", want, got)
				}
			}
			if tc.summary {
				for _, state := range []string{"Healthy", "Degraded", "Unhealthy"} {
					found := false
					for _, line := range strings.Split(got, "\n") {
						fields := strings.Fields(line)
						if len(fields) == 3 && fields[0] == state && fields[1] == "-" && fields[2] == "-" {
							found = true
						}
					}
					if !found {
						t.Errorf("missing %s row with absent percentage and duration: %q", state, got)
					}
				}
			}
		})
	}
}

func TestNodeHealthTableWriteBoundaries(t *testing.T) {
	var history nvfleetint.NodeHealthHistory
	if err := json.Unmarshal([]byte(nodeHealthBody), &history); err != nil {
		t.Fatalf("decode health fixture: %v", err)
	}
	history.Segments = nil
	if history.HealthSummary == nil || history.HealthSummary.HealthyDurationSeconds == nil {
		t.Fatal("fixture has no populated summary")
	}
	control := &nodeRenderWriter{failAt: -1}
	if err := writeNodeHealthTable(control, "node-1", history); err != nil {
		t.Fatalf("render control: %v", err)
	}
	full := control.output.String()
	summary := strings.Index(full, "\nHealth Summary\n")
	machine := strings.Index(full, "\nMachine Status\n")
	empty := strings.Index(full, "No health timeline segments in the requested window.\n")
	if summary <= 0 || machine <= summary || empty <= machine {
		t.Fatalf("unexpected section order: %q", full)
	}
	for _, tc := range []struct {
		name   string
		offset int
		prefix string
	}{
		{name: "metadata table", offset: 0, prefix: "FIELD"},
		{name: "summary heading", offset: summary, prefix: "\nHealth Summary\n"},
		{name: "summary table", offset: summary + len("\nHealth Summary\n"), prefix: "STATE"},
		{name: "machine heading", offset: machine, prefix: "\nMachine Status\n"},
		{name: "empty timeline message", offset: empty, prefix: "No health timeline segments in the requested window.\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			index, offset := -1, 0
			for i, part := range control.writes {
				if offset == tc.offset && strings.HasPrefix(part, tc.prefix) {
					index = i
					break
				}
				offset += len(part)
			}
			if index < 0 {
				t.Fatalf("boundary at %d beginning %q not found in writes %q", tc.offset, tc.prefix, control.writes)
			}
			sentinel := errors.New("health write failed")
			w := &nodeRenderWriter{failAt: index, err: sentinel}
			if err := writeNodeHealthTable(w, "node-1", history); !errors.Is(err, sentinel) {
				t.Errorf("error = %v, want sentinel", err)
			}
			if w.after != 0 || len(w.writes) != index+1 || w.output.String() != full[:tc.offset] {
				t.Errorf("unexpected writes after failure: accepted=%q writes=%q", w.output.String(), w.writes)
			}
		})
	}
}

func TestNodeCommandEarlyValidation(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want []string
	}{
		{name: "blank describe UUID", args: []string{"node", "describe", " \t "}, want: []string{"node UUID is required"}},
		{name: "invalid describe source", args: []string{"node", "describe", "node-1", "--data-source", "bmc"}, want: []string{"data-source", "bmc"}},
		{name: "zero options timeout", args: []string{"node", "options", "--timeout", "0"}, want: []string{"--timeout must be greater than 0"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			var mu sync.Mutex
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				mu.Lock()
				requests++
				mu.Unlock()
				http.Error(w, "unexpected request", http.StatusInternalServerError)
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")

			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected validation error")
			}
			for _, want := range tc.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
			if out.Len() != 0 {
				t.Errorf("expected empty stdout, got %q", out.String())
			}
			mu.Lock()
			defer mu.Unlock()
			if requests != 0 {
				t.Errorf("requests = %d, want none", requests)
			}
		})
	}
}

func TestNodeCommandsRequireCredentials(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "list", args: []string{"node", "list"}},
		{name: "describe", args: []string{"node", "describe", "node-1"}},
		{name: "options", args: []string{"node", "options"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			t.Setenv("NVFLEETINT_API_KEY", "")
			t.Setenv("NVFLEETINT_API_URL", "")
			t.Setenv("NVFLEETINT_SERVICE_KEY", "")
			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected missing credential error")
			}
			for _, want := range []string{"no profile configured", "auth add", "NVFLEETINT_API_KEY"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
			if out.Len() != 0 {
				t.Errorf("expected empty stdout, got %q", out.String())
			}
		})
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

func TestNodeDescribeCombinedSelectedInventorySections(t *testing.T) {
	for _, tc := range []struct {
		name         string
		inbandStatus int
	}{
		{name: "inband present", inbandStatus: http.StatusOK},
		{name: "inband missing", inbandStatus: http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			var mu sync.Mutex
			counts := map[string]int{"inband": 0, "oob": 0}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/nodes/node-1" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
					http.Error(w, "unexpected request", http.StatusBadRequest)
					return
				}
				sources := r.URL.Query()["dataSource"]
				if len(sources) != 1 || (sources[0] != "inband" && sources[0] != "oob") {
					t.Errorf("unexpected dataSource query: %q", sources)
					http.Error(w, "unexpected source", http.StatusBadRequest)
					return
				}
				source := sources[0]
				mu.Lock()
				counts[source]++
				mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				if source == "oob" {
					_, _ = w.Write([]byte(completeOOBNodeJSON))
				} else if tc.inbandStatus == http.StatusNotFound {
					w.WriteHeader(http.StatusNotFound)
					_, _ = w.Write([]byte(`{"error":"source not found"}`))
				} else {
					_, _ = w.Write([]byte(`{"nodeUUID":"node-1","dataSource":"inband","hostname":"distinctive-inband-host"}`))
				}
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")

			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs([]string{"node", "describe", "node-1", "--section", "systems,chassis"})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("command failed: %v", err)
			}
			mu.Lock()
			for _, source := range []string{"inband", "oob"} {
				if counts[source] != 1 {
					t.Errorf("%s requests = %d, want 1", source, counts[source])
				}
			}
			mu.Unlock()
			got := out.String()
			for _, want := range []string{"Systems", "system-1", "CPUs", "cpu-1", "GPUs", "gpu-1", "Chassis", "chassis-1"} {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q: %q", want, got)
				}
			}
			for _, unwanted := range []string{"distinctive-inband-host", "In-band", "Out-of-band"} {
				if strings.Contains(got, unwanted) {
					t.Errorf("output contains %q: %q", unwanted, got)
				}
			}
			for _, line := range strings.Split(got, "\n") {
				fields := strings.Fields(line)
				if len(fields) > 0 && fields[0] == "FIELD" {
					t.Errorf("output contains summary header: %q", line)
				}
				for _, heading := range []string{"Managers", "Firmware", "Fabrics"} {
					if strings.TrimSpace(line) == heading {
						t.Errorf("output contains unselected heading %q", heading)
					}
				}
			}
		})
	}
}
