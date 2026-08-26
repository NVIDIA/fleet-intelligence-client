// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package cmdutil

import (
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
)

// Verifies the location label prefers a region and falls back to city/country
func TestFormatGeoLocation(t *testing.T) {
	tests := []struct {
		name     string
		location *nvfleetint.GeoLocation
		want     string
	}{
		{name: "city country", location: &nvfleetint.GeoLocation{City: "Santa Clara", Country: "US"}, want: "Santa Clara, US"},
		{name: "region wins", location: &nvfleetint.GeoLocation{Region: "us-west-1", City: "Ignored"}, want: "us-west-1"},
		{name: "city only", location: &nvfleetint.GeoLocation{City: "Santa Clara"}, want: "Santa Clara"},
		{name: "empty", location: &nvfleetint.GeoLocation{}, want: "-"},
		{name: "nil", location: nil, want: "-"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatGeoLocation(tt.location); got != tt.want {
				t.Fatalf("unexpected label: got %q want %q", got, tt.want)
			}
		})
	}
}
