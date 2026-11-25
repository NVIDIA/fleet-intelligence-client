// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package xidburst

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
)

func TestXIDBurstDescribeSparseDetails(t *testing.T) {
	number := 48
	burst := nvfleetint.XIDBurstDetails{XIDBurst: nvfleetint.XIDBurst{
		XIDNumbers:       []nvfleetint.XIDBurstXID{{}, {XIDNumber: &number, Description: "Memory error"}},
		DeviceIDs:        map[string][]int{"z-device": {94}, "a-device": {48}},
		SuggestedActions: []nvfleetint.SuggestedAction{{Code: "DRAIN", Action: "Drain node"}},
	}}
	rows := xidBurstDescribeRows(burst)
	if got := rows[13]; !reflect.DeepEqual(got, []string{"XIDS", "48"}) {
		t.Errorf("XIDS row = %v, want [XIDS 48]", got)
	}
	wantDetails := [][]string{
		{"XID 48", "Memory error"},
		{"DEVICE a-device", "48"},
		{"DEVICE z-device", "94"},
		{"SUGGESTED ACTION", "DRAIN: Drain node"},
	}
	if got := rows[19:]; !reflect.DeepEqual(got, wantDetails) {
		t.Errorf("detail rows = %v, want %v", got, wantDetails)
	}
	if got := xidBurstDeviceRows(map[string][]int{}); len(got) != 0 {
		t.Errorf("empty device map produced rows: %v", got)
	}
	var out bytes.Buffer
	if err := writeXIDBurstDescribeTable(&out, burst); err != nil {
		t.Fatalf("render describe table: %v", err)
	}
	for _, want := range []string{"XID 48", "Memory error", "DEVICE a-device", "DEVICE z-device", "SUGGESTED ACTION", "DRAIN: Drain node"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("describe output missing %q: %s", want, out.String())
		}
	}
}
