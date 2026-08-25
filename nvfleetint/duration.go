// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"fmt"
	"regexp"
	"time"
)

// DurationUnitsMessage states the duration syntax the API accepts. It is the
// second half of every message ValidateWindow and ValidateStep return, so a
// front end that re-words the rule states the units the same way the SDK does.
const DurationUnitsMessage = "expected a positive duration using units ns, us, µs, ms, s, m, or h"

var (
	// maxDuration is the largest value time.ParseDuration can represent, and so
	// the ceiling named when a syntactically valid duration overflows.
	maxDuration = time.Duration(1<<63 - 1)
	// durationPattern accepts the Go duration units the API documents. It is
	// stricter than time.ParseDuration, which also takes "d" and other units the
	// backend rejects, so the check has to happen before parsing rather than
	// through it.
	durationPattern = regexp.MustCompile(`^\+?(?:(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:ns|us|µs|ms|s|m|h))+$`)
)

// ValidateWindow reports whether window is a positive Go duration in the units
// the relative-time endpoints accept. The events, error report, and XID burst
// endpoints all take the same window, so the rule lives here once.
func ValidateWindow(window string) error {
	if !durationPattern.MatchString(window) {
		return fmt.Errorf("invalid window %q: %s", window, DurationUnitsMessage)
	}
	duration, err := time.ParseDuration(window)
	if err != nil {
		return fmt.Errorf("invalid window %q: duration is too large; maximum is %s", window, maxDuration)
	}
	if duration <= 0 {
		return fmt.Errorf("invalid window %q: %s", window, DurationUnitsMessage)
	}
	return nil
}

// ValidateStep reports whether step is a graph bucket width the API accepts: a
// positive duration of at least one minute.
func ValidateStep(step string) error {
	if !durationPattern.MatchString(step) {
		return fmt.Errorf("invalid step %q: %s", step, DurationUnitsMessage)
	}
	duration, err := time.ParseDuration(step)
	if err != nil {
		return fmt.Errorf("invalid step %q: duration is too large; maximum is %s", step, maxDuration)
	}
	if duration < time.Minute {
		return fmt.Errorf("invalid step %q: expected at least 1m", step)
	}
	return nil
}
