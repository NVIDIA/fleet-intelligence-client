// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package cmdutil

import (
	"errors"
	"strings"

	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"

	"github.com/spf13/cobra"
)

// The relative/absolute time range is spelled the same way by `event`,
// `xidburst`, and `report error`, so the flags and the rules for combining
// them live here rather than once per command. What each value may contain is
// the SDK's rule, not restated here.

// RegisterTimeRangeFlags registers the shared relative/absolute time-range
// flags.
func RegisterTimeRangeFlags(cmd *cobra.Command, window, start, end *string) {
	cmd.Flags().StringVar(window, "window", "", "Relative time window; valid units: ns, us, µs, ms, s, m, h")
	cmd.Flags().StringVar(start, "start", "", "Absolute start time in RFC3339 format")
	cmd.Flags().StringVar(end, "end", "", "Absolute end time in RFC3339 format")
}

// ValidateTimeRangeFlags mirrors the SDK time-range rules at the CLI layer for
// early errors that name the flag the user typed.
func ValidateTimeRangeFlags(window, start, end string) error {
	window = strings.TrimSpace(window)
	start = strings.TrimSpace(start)
	end = strings.TrimSpace(end)

	hasWindow := window != ""
	hasStart := start != ""
	hasEnd := end != ""

	if !hasWindow && !hasStart && !hasEnd {
		return errors.New("a time range is required: use --window for a relative range, or --start and --end for an absolute range")
	}
	if hasWindow && (hasStart || hasEnd) {
		return errors.New("--window cannot be used with --start or --end")
	}
	if hasWindow {
		_, err := NormalizeWindowFlag(window)
		return err
	}
	if !hasStart || !hasEnd {
		return errors.New("--start and --end must be used together")
	}
	if err := ValidateRFC3339Flag("--start", start); err != nil {
		return err
	}
	return ValidateRFC3339Flag("--end", end)
}

// NormalizeWindowFlag trims a --window value and checks it against the SDK's
// rule, returning the value to send to the backend.
func NormalizeWindowFlag(window string) (string, error) {
	window = strings.TrimSpace(window)
	if err := nvfleetint.ValidateWindow(window); err != nil {
		return "", err
	}
	return window, nil
}

// ValidateStepFlag trims a --step value and checks it against the SDK's rule.
func ValidateStepFlag(step string) error {
	return nvfleetint.ValidateStep(strings.TrimSpace(step))
}
