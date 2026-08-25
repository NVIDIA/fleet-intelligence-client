// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package cmdutil

import (
	"strings"
	"testing"
)

func TestRequireSingleArg(t *testing.T) {
	validate := RequireSingleArg("node UUID")

	if err := validate(nil, []string{"only-one"}); err != nil {
		t.Fatalf("expected exactly one arg to pass, got: %v", err)
	}

	err := validate(nil, nil)
	if err == nil || !strings.Contains(err.Error(), "node UUID is required") {
		t.Fatalf("expected missing-arg error, got: %v", err)
	}

	err = validate(nil, []string{"a", "b"})
	if err == nil || !strings.Contains(err.Error(), "only one node UUID may be given, got 2") {
		t.Fatalf("expected too-many-args error, got: %v", err)
	}
}
