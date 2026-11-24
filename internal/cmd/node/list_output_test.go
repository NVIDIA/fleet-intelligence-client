// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package node

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdutil"
	clioutput "github.com/NVIDIA/fleet-intelligence-client/internal/output"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
)

// Verifies detail lists combine independent in-band and OOB API views by default
func TestNodeListCombinedDetailViews(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("view"); got != "detail" {
			t.Errorf("unexpected view: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("dataSource") {
		case "inband":
			_, _ = w.Write([]byte(`{
				"nodes":[{"nodeUUID":"node-inband","hostname":"gpu-001","dataSource":"inband","healthStatus":"Healthy"}],
				"hasMore":false,"page":0,"pageSize":20,"total":1
			}`))
		case "oob":
			_, _ = w.Write([]byte(`{
				"nodes":[{"nodeUUID":"node-oob","nodeName":"machine-001","nodeKind":"nvswitch","dataSource":"oob","bmcMac":"bmc-001","bmcIP":"192.0.2.10:443","healthStatus":"Degraded"}],
				"hasMore":false,"page":0,"pageSize":20,"total":1
			}`))
		default:
			t.Errorf("unexpected dataSource: %q", r.URL.Query().Get("dataSource"))
			http.Error(w, "missing dataSource", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"node", "list"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("combined table command failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"In-band", "node-inband", "gpu-001", "GPU TYPE",
		"Out-of-band", "node-oob", "machine-001", "NODE NAME", "nvswitch", "NODE KIND", "bmc-001", "BMC MAC",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("combined table missing %q: %q", want, got)
		}
	}

}

// Verifies basic node rows
func TestWriteNodeBasicTable(t *testing.T) {
	var out bytes.Buffer
	err := writeNodeTable(&out, string(nvfleetint.NodeViewBasic), "", []nvfleetint.Node{
		{UUID: "node-1", Hostname: "gpu-001", NodeName: "machine-001", BMCMAC: "bmc-001", BMCIP: "192.0.2.10:443"},
	})
	if err != nil {
		t.Fatalf("write table failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"UUID", "HOSTNAME", "NODE NAME", "BMC MAC", "BMC IP",
		"node-1", "gpu-001", "machine-001", "bmc-001", "192.0.2.10:443",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q: %q", want, got)
		}
	}
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
