// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package cmdutil

import (
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
)

// Verifies the location label prefers a region and falls back to city/country
func TestFormatLocation(t *testing.T) {
	tests := []struct {
		name     string
		location *nvfleetint.Location
		want     string
	}{
		{name: "city country", location: &nvfleetint.Location{City: "Santa Clara", Country: "US"}, want: "Santa Clara, US"},
		{name: "region wins", location: &nvfleetint.Location{Region: "us-west-1", City: "Ignored"}, want: "us-west-1"},
		{name: "city only", location: &nvfleetint.Location{City: "Santa Clara"}, want: "Santa Clara"},
		{name: "empty", location: &nvfleetint.Location{}, want: "-"},
		{name: "nil", location: nil, want: "-"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatLocation(tt.location); got != tt.want {
				t.Fatalf("unexpected label: got %q want %q", got, tt.want)
			}
		})
	}
}
