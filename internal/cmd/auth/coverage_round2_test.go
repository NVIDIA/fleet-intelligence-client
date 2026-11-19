// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"errors"
	"fmt"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/config"
)

func TestRound2ProfileListHint(t *testing.T) {
	if got := withProfileListHint(nil); got != nil {
		t.Fatalf("nil error changed: %v", got)
	}

	unrelated := errors.New("configuration unavailable")
	if got := withProfileListHint(unrelated); got != unrelated {
		t.Fatalf("unrelated error changed: got %v, want original error", got)
	}

	missing := fmt.Errorf("lookup: %w", config.ErrProfileNotFound)
	got := withProfileListHint(missing)
	if !errors.Is(got, config.ErrProfileNotFound) || !errors.Is(got, missing) {
		t.Fatalf("missing-profile error identity was lost: %v", got)
	}
	want := missing.Error() + "; run `nvfleetint auth list` to see the configured profiles"
	if got.Error() != want {
		t.Fatalf("hint = %q, want %q", got.Error(), want)
	}
}
