// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package output

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// Verifies writer errors are returned
func TestWriteRawJSONReturnsWriterError(t *testing.T) {
	if err := WriteRawJSON(failingWriter{}, []byte("{}")); err == nil {
		t.Fatal("expected writer error")
	}
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
