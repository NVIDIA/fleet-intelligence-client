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

package helpers

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
