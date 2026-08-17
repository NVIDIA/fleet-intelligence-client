// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package output

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
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
	enabled := true
	disabled := false
	location := &nvfleetint.GeoLocation{City: "Santa Clara", Country: "US"}
	regionalLocation := &nvfleetint.GeoLocation{Region: "us-west-1", City: "Ignored"}

	tests := []struct {
		name string
		got  string
		want string
	}{
		{name: "display empty", got: DisplayString("  "), want: "-"},
		{name: "display value", got: DisplayString("gpu"), want: "gpu"},
		{name: "truncate short", got: Truncate("short", 60), want: "short"},
		{name: "truncate exact", got: Truncate("abcde", 5), want: "abcde"},
		{name: "truncate long", got: Truncate("abcdefghij", 5), want: "abcd…"},
		{name: "truncate multibyte", got: Truncate("héllo wörld", 6), want: "héllo…"},
		{name: "truncate zero max", got: Truncate("abcde", 0), want: "abcde"},
		{name: "truncate one max", got: Truncate("abcde", 1), want: "…"},
		{name: "name and id", got: FormatNameAndID("East", "cz-1"), want: "East (cz-1)"},
		{name: "name and id missing name", got: FormatNameAndID("", "cz-1"), want: "cz-1"},
		{name: "name or id", got: FormatNameOrID("", "ng-1"), want: "ng-1"},
		{name: "optional int", got: FormatOptionalInt(&count), want: "7"},
		{name: "optional int nil", got: FormatOptionalInt(nil), want: "-"},
		{name: "optional bool true", got: FormatOptionalBool(&enabled), want: "true"},
		{name: "optional bool false", got: FormatOptionalBool(&disabled), want: "false"},
		{name: "optional bool nil", got: FormatOptionalBool(nil), want: "-"},
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
	// Page carries the SDK's 0-based page number; the footer presents it 1-based.
	if err := WritePaginationFooter(&out, Pagination{Page: 0, PageSize: 20, Total: 50}); err != nil {
		t.Fatalf("write footer failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{"ID", "NAME", "cz-1", "East", "Page: 1  Total Pages: 3  Page Size: 20  Total Entries: 50"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q: %q", want, got)
		}
	}
}

// Verifies pagination footers never display a page beyond the available pages.
func TestWritePaginationFooterBoundsPage(t *testing.T) {
	tests := []struct {
		name string
		page Pagination
		want string
	}{
		{
			name: "empty first page",
			page: Pagination{Page: 0, PageSize: 20, Total: 0},
			want: "Page: 0  Total Pages: 0  Page Size: 20  Total Entries: 0\n",
		},
		{
			name: "empty requested page",
			page: Pagination{Page: 1, PageSize: 20, Total: 0},
			want: "Page: 0  Total Pages: 0  Page Size: 20  Total Entries: 0\n",
		},
		{
			name: "past last page",
			page: Pagination{Page: 9, PageSize: 20, Total: 50},
			want: "Page: 3  Total Pages: 3  Page Size: 20  Total Entries: 50\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := WritePaginationFooter(&out, tt.page); err != nil {
				t.Fatalf("write footer failed: %v", err)
			}
			if got := out.String(); got != tt.want {
				t.Fatalf("unexpected footer: got %q want %q", got, tt.want)
			}
		})
	}
}

// Verifies multi-line cells are flattened so they do not break column alignment
func TestWriteTableFlattensMultilineCells(t *testing.T) {
	var out bytes.Buffer
	if err := WriteTable(&out, []string{"ID", "MESSAGE", "STATE"}, [][]string{
		{"e1", "Component: agent\n\nStatus: unhealthy", "Critical"},
		{"e2", "single line", "Warning"},
	}); err != nil {
		t.Fatalf("write table failed: %v", err)
	}

	got := out.String()
	// Each input row must render as exactly one physical line (plus the header).
	if lines := strings.Count(strings.TrimRight(got, "\n"), "\n"); lines != 2 {
		t.Fatalf("expected 3 lines (header + 2 rows), got %d:\n%s", lines+1, got)
	}
	if !strings.Contains(got, "Component: agent  Status: unhealthy") {
		t.Fatalf("newlines not flattened to spaces: %q", got)
	}
	if strings.Contains(got, "Component: agent\n") {
		t.Fatalf("embedded newline leaked into table: %q", got)
	}
}

// Verifies indented rows are aligned as columns under a common indent, with no
// header row, and that multi-line cells are flattened as they are in tables.
func TestWriteIndentedRows(t *testing.T) {
	var out bytes.Buffer
	if err := WriteIndentedRows(&out, "  ", [][]string{
		{"ng-1", "Training", "(in East)"},
		{"ng-longer-id", "Serving\nteam", "(in West)"},
		{"solo"},
	}); err != nil {
		t.Fatalf("write indented rows failed: %v", err)
	}

	got := out.String()
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected one line per row, got %d:\n%s", len(lines), got)
	}
	for _, line := range lines {
		if !strings.HasPrefix(line, "  ") {
			t.Fatalf("row is not indented: %q", line)
		}
	}
	// The second column starts at the same offset on every row that has one.
	if strings.Index(lines[0], "Training") != strings.Index(lines[1], "Serving") {
		t.Fatalf("columns are not aligned:\n%s", got)
	}
	if strings.Contains(got, "Serving\nteam") {
		t.Fatalf("multi-line cell was not flattened:\n%s", got)
	}
}

// Verifies a paragraph in the final column wraps on word boundaries and hangs
// under the column it belongs to, instead of folding back to column zero.
func TestWriteIndentedRowsWrapsFinalColumn(t *testing.T) {
	paragraph := "Drain the node, pull it from the scheduling pool, and open a hardware ticket " +
		"with the burst ID attached so the device can be replaced during the next maintenance window."

	var out bytes.Buffer
	if err := WriteIndentedRows(&out, "  ", [][]string{
		{"PULL_FROM_SERVICE", paragraph},
		{"CHECK_LOGS", "Inspect application logs"},
	}); err != nil {
		t.Fatalf("write indented rows failed: %v", err)
	}

	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) < 3 {
		t.Fatalf("paragraph was not wrapped:\n%s", out.String())
	}
	column := strings.Index(lines[0], "Drain")
	if column <= 0 {
		t.Fatalf("first line does not start the paragraph:\n%s", out.String())
	}
	for _, line := range lines {
		if len([]rune(line)) > indentedRowWidth {
			t.Fatalf("line exceeds the layout width:\n%s", line)
		}
	}
	// Every continuation line hangs under the column, and none of them is blank
	// or starts mid-word with a stray space.
	for _, line := range lines[1 : len(lines)-1] {
		if got := len(line) - len(strings.TrimLeft(line, " ")); got != column {
			t.Fatalf("continuation line indented to %d, want %d:\n%s", got, column, out.String())
		}
	}
	// Wrapping preserves the text; only the line breaks differ.
	joined := strings.Join(strings.Fields(strings.Join(lines[:len(lines)-1], " ")), " ")
	if !strings.Contains(joined, strings.Join(strings.Fields(paragraph), " ")) {
		t.Fatalf("wrapping lost or reordered text:\n%s", out.String())
	}
	// The short row still renders on one line, aligned with the wrapped one.
	last := lines[len(lines)-1]
	if strings.Index(last, "Inspect") != column {
		t.Fatalf("short row is not aligned with the wrapped column:\n%s", out.String())
	}
}

// Verifies a paragraph in a middle column wraps while the column after it stays
// on the row's first line, so that column still reads across rows.
func TestWriteIndentedRowsKeepsTrailingColumnOnFirstLine(t *testing.T) {
	var out bytes.Buffer
	if err := WriteIndentedRows(&out, "  ", [][]string{
		{"ng-1", strings.Repeat("long name ", 12), "(in East)"},
		{"ng-2", "Serving", "(in West)"},
	}); err != nil {
		t.Fatalf("write indented rows failed: %v", err)
	}

	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) < 3 {
		t.Fatalf("middle column was not wrapped:\n%s", out.String())
	}
	if !strings.Contains(lines[0], "(in East)") {
		t.Fatalf("trailing column left the first line:\n%s", out.String())
	}
	// The tag column starts at the same offset on both rows.
	if strings.Index(lines[0], "(in East)") != strings.Index(lines[len(lines)-1], "(in West)") {
		t.Fatalf("trailing column is not aligned across rows:\n%s", out.String())
	}
}

// Verifies rows that already fit are written unwrapped, and that a column left
// too little room to wrap into is written long rather than shredded.
func TestWriteIndentedRowsWrapsOnlyWhenItHelps(t *testing.T) {
	var fits bytes.Buffer
	if err := WriteIndentedRows(&fits, "  ", [][]string{{"ng-1", "Training"}}); err != nil {
		t.Fatalf("write indented rows failed: %v", err)
	}
	if got := strings.Count(strings.TrimRight(fits.String(), "\n"), "\n"); got != 0 {
		t.Fatalf("a row that fits should stay on one line:\n%s", fits.String())
	}

	// The identifier alone leaves less than minWrappedColumnWidth for the text.
	wide := strings.Repeat("x", indentedRowWidth-minWrappedColumnWidth)
	var cramped bytes.Buffer
	if err := WriteIndentedRows(&cramped, "  ", [][]string{{wide, strings.Repeat("word ", 20)}}); err != nil {
		t.Fatalf("write indented rows failed: %v", err)
	}
	if got := strings.Count(strings.TrimRight(cramped.String(), "\n"), "\n"); got != 0 {
		t.Fatalf("a column with no room to wrap should stay long:\n%s", cramped.String())
	}
}

// Verifies a single-column section renders without needing a column to wrap.
func TestWriteIndentedRowsSingleColumn(t *testing.T) {
	var out bytes.Buffer
	if err := WriteIndentedRows(&out, "  ", [][]string{{"(none)"}, {strings.Repeat("word ", 40)}}); err != nil {
		t.Fatalf("write indented rows failed: %v", err)
	}
	if !strings.HasPrefix(out.String(), "  (none)\n") {
		t.Fatalf("unexpected single-column output:\n%s", out.String())
	}
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	for _, line := range lines {
		if len([]rune(line)) > indentedRowWidth {
			t.Fatalf("single-column line exceeds the layout width:\n%s", line)
		}
	}
	// A wrapped value continues at a deeper indent than the values themselves,
	// so its second line cannot be mistaken for a value of its own.
	valueIndent := len(lines[0]) - len(strings.TrimLeft(lines[0], " "))
	for _, line := range lines[2:] {
		if got := len(line) - len(strings.TrimLeft(line, " ")); got <= valueIndent {
			t.Fatalf("continuation line is indented %d, no deeper than a value at %d:\n%s", got, valueIndent, out.String())
		}
	}
}
