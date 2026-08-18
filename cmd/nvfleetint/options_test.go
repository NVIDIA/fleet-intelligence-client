// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
)

// Verifies a sort field the CLI's allowlist does not take is called out rather
// than offered under --sort-by, so a backend that grows a field for a view the
// CLI cannot request never advertises a value that would be rejected.
func TestOptionsRendererSeparatesUnacceptedSortFields(t *testing.T) {
	renderer := optionsRenderer{
		consumers: []string{"node list"},
		filters:   map[string]optionFlag{"gpuTypes": {name: "--gpu-type"}},
		sortAccepted: func(field string) bool {
			return field == "hostname"
		},
	}

	var out bytes.Buffer
	err := renderer.write(&out, nvfleetint.FilterOptions{
		Filters: nvfleetint.Filters{Fields: []nvfleetint.FilterField{
			{Name: "gpuTypes", Options: []nvfleetint.FilterOption{{Value: "NVIDIA-H100"}}},
		}},
		Sorting: nvfleetint.SortingOptions{
			Fields:   []string{"hostname", "backendOnlyField"},
			Orders:   []string{"asc"},
			Defaults: nvfleetint.SortingDefaults{Field: "hostname", Order: "asc"},
		},
	})
	if err != nil {
		t.Fatalf("render options failed: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "Returned by the API but not accepted by 'node list' --sort-by: backendOnlyField") {
		t.Fatalf("unaccepted sort field not called out:\n%s", got)
	}
	// The rejected field is named once, in the trailing note, not in the list of
	// values the flag takes.
	if strings.Contains(got, "\n  backendOnlyField\n") {
		t.Fatalf("unaccepted sort field offered under --sort-by:\n%s", got)
	}
}

// Verifies the consuming commands are listed in prose, including the fallback
// used when a renderer names none.
func TestOptionsRendererConsumerList(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		consumers []string
		want      string
	}{
		{name: "none", want: "this command"},
		{name: "one", consumers: []string{"node list"}, want: "'node list'"},
		{name: "two", consumers: []string{"alert summary", "alert node"}, want: "'alert summary' and 'alert node'"},
		{
			name:      "three",
			consumers: []string{"a", "b", "c"},
			want:      "'a', 'b' and 'c'",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			renderer := optionsRenderer{consumers: testCase.consumers}
			if got := renderer.consumerList(); got != testCase.want {
				t.Fatalf("consumer list = %q, want %q", got, testCase.want)
			}
		})
	}
}
