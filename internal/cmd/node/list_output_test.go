// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package node

import (
	"bytes"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdutil"
	clioutput "github.com/NVIDIA/fleet-intelligence-client/internal/output"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
)

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
