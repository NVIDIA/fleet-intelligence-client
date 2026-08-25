// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package cmdutil

import (
	"slices"
	"strings"
	"testing"
)

// Verifies comma-separated flag parsing
func TestParseCommaList(t *testing.T) {
	values, err := ParseCommaList("node-1, node-2")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if !slices.Equal(values, []string{"node-1", "node-2"}) {
		t.Fatalf("unexpected values: %#v", values)
	}

	values, err = ParseCommaList(" ")
	if err != nil {
		t.Fatalf("empty parse failed: %v", err)
	}
	if values != nil {
		t.Fatalf("expected nil values for empty input, got %#v", values)
	}
}

// Verifies empty list members are rejected
func TestParseCommaListRejectsEmptyItems(t *testing.T) {
	_, err := ParseCommaList("node-1,,node-2")
	if err == nil {
		t.Fatal("expected parse error")
	}
	if !strings.Contains(err.Error(), "empty values are not allowed") {
		t.Fatalf("unexpected error: %v", err)
	}
}
