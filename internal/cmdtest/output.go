// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package cmdtest

import (
	"strings"
	"testing"
)

// Returns the indented body of one rendered options section.
func SectionBody(t *testing.T, output, heading string) string {
	t.Helper()
	// A heading is either the bare flag or the flag plus a parenthesized note.
	start := strings.Index(output, "\n"+heading+"\n")
	if start < 0 {
		start = strings.Index(output, "\n"+heading+" ")
	}
	if start < 0 {
		t.Fatalf("section %q not found in:\n%s", heading, output)
	}
	rest := output[start+1:]
	if end := strings.Index(rest, "\n\n"); end >= 0 {
		return rest[:end]
	}
	return rest
}
