// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package output

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

type outputErrorWriter struct {
	accepted bytes.Buffer
	err      error
	failOn   string
	failed   bool
}

func (w *outputErrorWriter) Write(p []byte) (int, error) {
	if w.failOn == "" || strings.Contains(string(p), w.failOn) {
		w.failed = true
		return 0, w.err
	}
	return w.accepted.Write(p)
}

type firstLineWriter struct {
	accepted bytes.Buffer
	err      error
	failed   bool
}

func (w *firstLineWriter) Write(p []byte) (int, error) {
	if w.accepted.Len() > 0 {
		w.failed = true
		return 0, w.err
	}
	return w.accepted.Write(p)
}

func TestOutputRenderingBoundariesAndWriteErrors(t *testing.T) {
	t.Run("name without ID", func(t *testing.T) {
		if got := FormatNameAndID("  East  ", "  "); got != "East" {
			t.Fatalf("FormatNameAndID() = %q, want East", got)
		}
	})

	t.Run("table header and row errors", func(t *testing.T) {
		sentinel := errors.New("table write failed")
		t.Run("header", func(t *testing.T) {
			w := &outputErrorWriter{err: sentinel}
			if err := WriteTable(w, []string{"header"}, nil); !errors.Is(err, sentinel) {
				t.Fatalf("WriteTable() error = %v, want sentinel", err)
			}
		})
		t.Run("row", func(t *testing.T) {
			w := &outputErrorWriter{err: sentinel, failOn: "distinct-row"}
			if err := WriteTable(w, []string{"header"}, [][]string{{"distinct-row"}}); !errors.Is(err, sentinel) {
				t.Fatalf("WriteTable() error = %v, want sentinel", err)
			}
			if got := w.accepted.String(); got != "header\n" || !w.failed {
				t.Fatalf("accepted output = %q, failed = %t; want header followed by row failure", got, w.failed)
			}
		})
	})

	t.Run("empty indented rows", func(t *testing.T) {
		for _, rows := range [][][]string{nil, {}} {
			var out bytes.Buffer
			if err := WriteIndentedRows(&out, "  ", rows); err != nil || out.Len() != 0 {
				t.Fatalf("WriteIndentedRows(%v) output = %q, error = %v", rows, out.String(), err)
			}
		}
		var out bytes.Buffer
		if err := WriteIndentedRows(&out, "  ", [][]string{{}, {"normal"}}); err != nil {
			t.Fatal(err)
		}
		if got := out.String(); got != "  normal\n" {
			t.Fatalf("output = %q, want indented normal line", got)
		}
	})

	t.Run("indented row errors", func(t *testing.T) {
		sentinel := errors.New("indented write failed")
		t.Run("wrapped continuation", func(t *testing.T) {
			cell := strings.Repeat("word ", 60)
			w := &firstLineWriter{err: sentinel}
			if err := WriteIndentedRows(w, "  ", [][]string{{cell}}); !errors.Is(err, sentinel) {
				t.Fatalf("WriteIndentedRows() error = %v, want sentinel", err)
			}
			got := w.accepted.String()
			if !w.failed || !strings.HasPrefix(got, "  word ") || strings.Count(got, "\n") != 1 || !strings.HasSuffix(got, "\n") {
				t.Fatalf("accepted output = %q, failed = %t; want only the first wrapped line", got, w.failed)
			}
		})
		t.Run("immediate failure", func(t *testing.T) {
			w := &outputErrorWriter{err: sentinel}
			if err := WriteIndentedRows(w, "  ", [][]string{{"short"}}); !errors.Is(err, sentinel) {
				t.Fatalf("WriteIndentedRows() error = %v, want sentinel", err)
			}
			if w.accepted.Len() != 0 || !w.failed {
				t.Fatalf("accepted output = %q, failed = %t", w.accepted.String(), w.failed)
			}
		})
	})
}

func TestWrapCellWhitespaceAndIndentedRow(t *testing.T) {
	spaces := strings.Repeat(" ", 12)
	unicodeWhitespace := strings.Repeat("\u2003\u00a0", 6)
	tests := []struct {
		name  string
		input string
		width int
		want  []string
	}{
		{name: "spaces", input: spaces, width: 4, want: []string{spaces}},
		{name: "unicode whitespace", input: unicodeWhitespace, width: 4, want: []string{unicodeWhitespace}},
		{name: "words", input: "alpha beta gamma", width: 10, want: []string{"alpha beta", "gamma"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wrapCell(tt.input, tt.width)
			if len(got) != len(tt.want) {
				t.Fatalf("wrapCell() = %q, want %q", got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("wrapCell() = %q, want %q", got, tt.want)
				}
			}
		})
	}

	t.Run("trailing cell survives whitespace wrapping", func(t *testing.T) {
		const id, trailing = "row-id", "trailing-value"
		row := []string{id, strings.Repeat(" ", 120), trailing}
		var out bytes.Buffer
		if err := WriteIndentedRows(&out, "  ", [][]string{row}); err != nil {
			t.Fatalf("WriteIndentedRows() error = %v", err)
		}
		got := out.String()
		idIndex, trailingIndex := strings.Index(got, id), strings.Index(got, trailing)
		if strings.Count(got, "\n") != 1 || !strings.HasSuffix(got, "\n") || idIndex < 0 || trailingIndex <= idIndex+len(id)-1 {
			t.Fatalf("output = %q, want one line containing ID followed by trailing value", got)
		}
	})
}
