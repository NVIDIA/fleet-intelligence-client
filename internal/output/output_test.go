// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package output

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/pkg/fleetintelligence"
)

// Returns an error for every write
type failingWriter struct{}

// Always fails
func (failingWriter) Write(_ []byte) (int, error) {
	return 0, errors.New("write failed")
}

// Verifies supported output formats
func TestIsValidFormat(t *testing.T) {
	if !IsValidFormat(FormatTable) || !IsValidFormat(FormatJSON) {
		t.Fatal("expected table and json to be valid")
	}
	if IsValidFormat("yaml") {
		t.Fatal("expected yaml to be invalid")
	}
}

// Verifies JSON encoding
func TestWriteJSON(t *testing.T) {
	var out bytes.Buffer
	if err := WriteJSON(&out, map[string]string{"status": "ok"}); err != nil {
		t.Fatalf("write JSON failed: %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != `{"status":"ok"}` {
		t.Fatalf("unexpected JSON: %q", got)
	}
}

// Verifies raw JSON passthrough and empty fallback
func TestWriteRawJSON(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
		want string
	}{
		{name: "payload", raw: []byte("{\"ok\":true}\n\n"), want: "{\"ok\":true}\n"},
		{name: "empty", raw: nil, want: "{}\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := WriteRawJSON(&out, tt.raw); err != nil {
				t.Fatalf("write raw JSON failed: %v", err)
			}
			if out.String() != tt.want {
				t.Fatalf("unexpected raw JSON: got %q want %q", out.String(), tt.want)
			}
		})
	}
}

// Verifies writer errors are returned
func TestWriteRawJSONReturnsWriterError(t *testing.T) {
	if err := WriteRawJSON(failingWriter{}, []byte("{}")); err == nil {
		t.Fatal("expected writer error")
	}
}

// Verifies display formatting helpers
func TestFormatHelpers(t *testing.T) {
	count := 7
	percentage := float32(95)
	percentageDecimal := float32(95.5)
	zeroPercentage := float32(0)
	location := &fleetintelligence.GeoLocation{City: "Santa Clara", Country: "US"}
	regionalLocation := &fleetintelligence.GeoLocation{Region: "us-west-1", City: "Ignored"}

	tests := []struct {
		name string
		got  string
		want string
	}{
		{name: "display empty", got: DisplayString("  "), want: "-"},
		{name: "display value", got: DisplayString("gpu"), want: "gpu"},
		{name: "name and id", got: FormatNameAndID("East", "cz-1"), want: "East (cz-1)"},
		{name: "name and id missing name", got: FormatNameAndID("", "cz-1"), want: "cz-1"},
		{name: "name or id", got: FormatNameOrID("", "ng-1"), want: "ng-1"},
		{name: "optional int", got: FormatOptionalInt(&count), want: "7"},
		{name: "optional int nil", got: FormatOptionalInt(nil), want: "-"},
		{name: "optional percentage", got: FormatOptionalPercentage(&percentage), want: "95%"},
		{name: "optional percentage decimal", got: FormatOptionalPercentage(&percentageDecimal), want: "95.5%"},
		{name: "optional percentage zero", got: FormatOptionalPercentage(&zeroPercentage), want: "0%"},
		{name: "optional percentage nil", got: FormatOptionalPercentage(nil), want: "-"},
		{name: "string list", got: FormatStringList([]string{"prod", " ", "h100"}), want: "prod, h100"},
		{name: "string list empty", got: FormatStringList(nil), want: "-"},
		{name: "geolocation city country", got: FormatGeoLocation(location), want: "Santa Clara, US"},
		{name: "geolocation region", got: FormatGeoLocation(regionalLocation), want: "us-west-1"},
		{name: "geolocation nil", got: FormatGeoLocation(nil), want: "-"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("unexpected value: got %q want %q", tt.got, tt.want)
			}
		})
	}
}

// Verifies table rendering and pagination footer
func TestWriteTableAndPaginationFooter(t *testing.T) {
	var out bytes.Buffer
	if err := WriteTable(&out, []string{"ID", "NAME"}, [][]string{{"cz-1", "East"}}); err != nil {
		t.Fatalf("write table failed: %v", err)
	}
	if err := WritePaginationFooter(&out, Pagination{Page: 1, PageSize: 20, Total: 50}); err != nil {
		t.Fatalf("write footer failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{"ID", "NAME", "cz-1", "East", "Page: 1  Total Pages: 3  Page Size: 20  Total Entries: 50"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q: %q", want, got)
		}
	}
}
