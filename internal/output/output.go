// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

// Package output formats nvfleetint command responses. It deals in plain
// values only: formatters that take SDK models live in internal/cmdutil.
package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
	"unicode/utf8"
)

const (
	FormatTable = "table"
	FormatJSON  = "json"
)

// Represents pagination metadata for table footers
type Pagination struct {
	Page     int
	PageSize int
	Total    int
}

// Reports whether format is a supported output format
func IsValidFormat(format string) bool {
	return format == FormatTable || format == FormatJSON
}

// Writes value as a single JSON document
func WriteJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	return encoder.Encode(value)
}

// Writes a raw JSON payload with a trailing newline
func WriteRawJSON(w io.Writer, data []byte) error {
	data = bytes.TrimRight(data, "\n")
	if len(data) == 0 {
		data = []byte("{}")
	}
	if _, err := w.Write(data); err != nil {
		return err
	}
	_, err := fmt.Fprintln(w)
	return err
}

// Truncate shortens a single-line display value to at most maxRunes runes,
// replacing the trailing overflow with an ellipsis. It counts runes (not
// bytes) so multi-byte text is not cut mid-character. Table output is a
// human-friendly summary; the full value remains available via -o json.
func Truncate(value string, maxRunes int) string {
	if maxRunes <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	if maxRunes == 1 {
		return "…"
	}
	return string(runes[:maxRunes-1]) + "…"
}

// Returns a printable placeholder for empty strings
func DisplayString(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

// Formats a display name with its stable identifier
func FormatNameAndID(name, id string) string {
	name = strings.TrimSpace(name)
	id = strings.TrimSpace(id)
	switch {
	case name != "" && id != "":
		return fmt.Sprintf("%s (%s)", name, id)
	case name != "":
		return name
	case id != "":
		return id
	default:
		return "-"
	}
}

// Returns the display name when present or the ID otherwise
func FormatNameOrID(name, id string) string {
	name = strings.TrimSpace(name)
	id = strings.TrimSpace(id)
	if name != "" {
		return name
	}
	return DisplayString(id)
}

// Formats optional integers for table output
func FormatOptionalInt(value *int) string {
	if value == nil {
		return "-"
	}
	return strconv.Itoa(*value)
}

// Formats optional booleans for table output. A nil value renders as "-" so a
// field the backend omitted is distinguishable from one reported as false.
func FormatOptionalBool(value *bool) string {
	if value == nil {
		return "-"
	}
	return strconv.FormatBool(*value)
}

// Formats optional percentage values for table output
func FormatOptionalPercentage(value *float32) string {
	if value == nil {
		return "-"
	}
	return strconv.FormatFloat(float64(*value), 'f', -1, 32) + "%"
}

// Formats non-empty strings as a comma-separated list
func FormatStringList(values []string) string {
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			cleaned = append(cleaned, value)
		}
	}
	if len(cleaned) == 0 {
		return "-"
	}
	return strings.Join(cleaned, ", ")
}

// Spaces separating one rendered column from the next. Shared so the tables
// written here and the indented rows laid out by hand space alike.
const tableCellPadding = 2

// Writes headers and rows as tab-aligned text
func WriteTable(w io.Writer, headers []string, rows [][]string) error {
	tw := tabwriter.NewWriter(w, 0, 0, tableCellPadding, ' ', 0)
	if err := writeTableRow(tw, headers); err != nil {
		return err
	}
	for _, row := range rows {
		if err := writeTableRow(tw, row); err != nil {
			return err
		}
	}
	return tw.Flush()
}

// Total width an indented row is laid out for. A column that would push the
// line past this is wrapped rather than left to fold at the terminal edge,
// which would restart the text at column zero and destroy the alignment the
// section depends on.
const indentedRowWidth = 100

// Narrowest a wrapped column may be squeezed to. When the other columns are
// wide enough that less than this is left — a UUID and a membership tag on the
// same row — wrapping would produce a ragged sliver of text, so the row is
// written long instead.
const minWrappedColumnWidth = 24

// Writes rows as aligned text under a common indent, with no header row. Used
// for sectioned output where a heading line already names the columns.
//
// When one column holds paragraphs — a sentence-long action description beside
// a short code — it is wrapped on word boundaries and its continuation lines
// are indented to hang under the column, so the paragraph stays visually inside
// its cell and any columns after it stay aligned.
func WriteIndentedRows(w io.Writer, indent string, rows [][]string) error {
	sanitized := make([][]string, 0, len(rows))
	for _, row := range rows {
		cells := make([]string, len(row))
		for i, cell := range row {
			cells[i] = sanitizeTableCell(cell)
		}
		sanitized = append(sanitized, cells)
	}

	layout := newIndentedLayout(indent, sanitized)
	for _, row := range sanitized {
		if err := layout.writeRow(w, row); err != nil {
			return err
		}
	}
	return nil
}

// Holds the column geometry shared by every row of one section, so the rows
// align with each other rather than each being measured alone.
type indentedLayout struct {
	indent string
	// widths are the aligned column widths.
	widths []int
	// wrapIndex is the column whose text is wrapped, or -1 when every row
	// already fits and nothing needs wrapping.
	wrapIndex int
	// wrapWidth is the width the wrapped column is laid out in. It is fixed for
	// the section so the columns that follow it start at the same offset on
	// every row.
	wrapWidth int
}

// Measures a section: how wide each column must be, and which column — if any —
// has to give up width so the rows fit.
func newIndentedLayout(indent string, rows [][]string) indentedLayout {
	layout := indentedLayout{indent: indent, wrapIndex: -1}
	// A cell that ends its row is excluded from its column's width, the way
	// tabwriter excludes it, so a short row cannot stretch a column it never
	// fills. The full widths, which do include those cells, decide what needs
	// wrapping.
	full := columnWidths(rows, false)
	layout.widths = columnWidths(rows, true)
	if len(full) == 0 {
		return layout
	}
	// A column made up only of trailing cells has no aligned width of its own;
	// give it a slot so every column can be indexed.
	for len(layout.widths) < len(full) {
		layout.widths = append(layout.widths, 0)
	}

	// The widest column is the one holding paragraphs; ties go to the later
	// column, which is the one nearer the edge.
	widest := 0
	for i, width := range full {
		if width >= full[widest] {
			widest = i
		}
	}

	fixed := displayWidth(indent) + tableCellPadding*(len(full)-1)
	for i, width := range full {
		if i != widest {
			fixed += width
		}
	}
	if fixed+full[widest] <= indentedRowWidth {
		return layout
	}
	available := indentedRowWidth - fixed
	if widest == 0 {
		// The first column has no column to its left to hang under, so its
		// continuation lines are stepped in instead; that width comes out of
		// the text. Without the step a wrapped value would read as two values.
		available -= tableCellPadding
	}
	if available >= minWrappedColumnWidth {
		layout.wrapIndex = widest
		layout.wrapWidth = available
		// The wrapped column is laid out at its wrapped width, not its natural
		// one, so that whatever follows it lines up.
		layout.widths[widest] = available
	}
	return layout
}

// Measures each column across every row. When trailingExcluded is set, a cell
// that ends its row is skipped, matching how tabwriter treats a trailing cell.
func columnWidths(rows [][]string, trailingExcluded bool) []int {
	var widths []int
	for _, row := range rows {
		limit := len(row)
		if trailingExcluded {
			limit--
		}
		for i := 0; i < limit; i++ {
			for len(widths) <= i {
				widths = append(widths, 0)
			}
			if width := displayWidth(row[i]); width > widths[i] {
				widths[i] = width
			}
		}
	}
	return widths
}

// Writes one row, wrapping its paragraph column when the section needs it.
func (layout indentedLayout) writeRow(w io.Writer, row []string) error {
	if len(row) == 0 {
		return nil
	}
	// A row that stops before the wrapped column, or one in a section that
	// needs no wrapping, is a single line of padded cells.
	if layout.wrapIndex < 0 || len(row) <= layout.wrapIndex {
		return writeIndentedLine(w, layout.indent+layout.padCells(row, 0, len(row)))
	}

	prefix := layout.indent + layout.padCells(row, 0, layout.wrapIndex)
	tail := layout.padCells(row, layout.wrapIndex+1, len(row))
	lines := wrapCell(row[layout.wrapIndex], layout.wrapWidth)
	for i, line := range lines {
		// Only the first line carries the leading columns; the rest hang under
		// where the wrapped column starts.
		leading := prefix
		if i > 0 {
			leading = strings.Repeat(" ", layout.hangingIndent(prefix))
		}
		// Anything after the wrapped column stays on the row's first line, so
		// that column reads across rows the way a table column does instead of
		// drifting down with each paragraph's length.
		if i == 0 && tail != "" {
			line = pad(line, layout.wrapWidth) + strings.Repeat(" ", tableCellPadding) + tail
		}
		if err := writeIndentedLine(w, leading+line); err != nil {
			return err
		}
	}
	return nil
}

// Reports how far the continuation lines of a wrapped cell are indented: under
// the column itself, or one step further in when that column starts the row and
// there is nothing to its left to hang under.
func (layout indentedLayout) hangingIndent(prefix string) int {
	if layout.wrapIndex == 0 {
		return displayWidth(prefix) + tableCellPadding
	}
	return displayWidth(prefix)
}

// Renders row[from:to] padded to the section's column widths. The last cell of
// the row is written unpadded, since nothing follows it on the line.
func (layout indentedLayout) padCells(row []string, from, to int) string {
	var builder strings.Builder
	for i := from; i < to && i < len(row); i++ {
		if i == len(row)-1 {
			builder.WriteString(row[i])
			break
		}
		builder.WriteString(pad(row[i], layout.widths[i]))
		builder.WriteString(strings.Repeat(" ", tableCellPadding))
	}
	return builder.String()
}

// Writes one laid-out line, dropping the padding that would otherwise trail it.
func writeIndentedLine(w io.Writer, line string) error {
	_, err := fmt.Fprintln(w, strings.TrimRight(line, " "))
	return err
}

// Splits text into lines that fit width, breaking on spaces. A single word
// longer than the width keeps its own line rather than being cut, so an ID or a
// URL stays selectable in a terminal.
func wrapCell(text string, width int) []string {
	if width <= 0 || displayWidth(text) <= width {
		return []string{text}
	}

	var lines []string
	var line strings.Builder
	for _, word := range strings.Fields(text) {
		switch {
		case line.Len() == 0:
			line.WriteString(word)
		case displayWidth(line.String())+1+displayWidth(word) <= width:
			line.WriteString(" ")
			line.WriteString(word)
		default:
			lines = append(lines, line.String())
			line.Reset()
			line.WriteString(word)
		}
	}
	if line.Len() > 0 {
		lines = append(lines, line.String())
	}
	if len(lines) == 0 {
		return []string{text}
	}
	return lines
}

// Right-pads a value to width, leaving it alone when it is already wider.
func pad(value string, width int) string {
	if missing := width - displayWidth(value); missing > 0 {
		return value + strings.Repeat(" ", missing)
	}
	return value
}

// Counts the characters a string occupies, so multi-byte values pad correctly.
func displayWidth(value string) int {
	return utf8.RuneCountInString(value)
}

// Writes the standard single-page pagination footer. Page carries the SDK's
// 0-based page number and is presented as the CLI's 1-based page.
func WritePaginationFooter(w io.Writer, page Pagination) error {
	totalPages := TotalPages(page.Total, page.PageSize)
	displayPage := OneBasedPage(page.Page, page.PageSize, page.Total)
	_, err := fmt.Fprintf(w, "Page: %d  Total Pages: %d  Page Size: %d  Total Entries: %d\n", displayPage, totalPages, page.PageSize, page.Total)
	return err
}

// Collapses characters that would corrupt tabwriter's structure — tabs (column
// separators), newlines and carriage returns (row separators) — into single
// spaces so a multi-line field renders on one table line.
var cellSanitizer = strings.NewReplacer("\t", " ", "\r\n", " ", "\n", " ", "\r", " ")

// Flattens a value to a single table-cell-safe line
func sanitizeTableCell(value string) string {
	return cellSanitizer.Replace(value)
}

// Writes one tab-separated table row, flattening any multi-line cells so they
// do not break column alignment
func writeTableRow(w io.Writer, fields []string) error {
	sanitized := make([]string, len(fields))
	for i, field := range fields {
		sanitized[i] = sanitizeTableCell(field)
	}
	_, err := fmt.Fprintln(w, strings.Join(sanitized, "\t"))
	return err
}
