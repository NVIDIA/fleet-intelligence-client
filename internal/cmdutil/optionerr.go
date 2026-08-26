// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package cmdutil

import (
	"errors"
	"fmt"

	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
)

// OptionFlagName names the flag that carries an SDK option, and optionally the
// accepted values to list when the CLI spells them differently from the
// backend.
type OptionFlagName struct {
	Flag string
	// Expected overrides the SDK's list of accepted values. It is set only
	// where the CLI's vocabulary differs, as it does for the node sort field
	// the CLI calls verificationCheck.
	Expected string
}

// RenderOptionError re-renders an SDK option error against the flag that
// carries the option, so that a message names what the user typed rather than
// the SDK field.
//
// Validation itself lives in the SDK: this maps its vocabulary onto the CLI's
// instead of restating the rules, which is what let the two drift apart.
// Errors that are not about one option value pass through unchanged; their
// wording already reads as CLI prose.
func RenderOptionError(err error, names map[string]OptionFlagName) error {
	var optionErr *nvfleetint.InvalidOptionError
	if !errors.As(err, &optionErr) {
		return err
	}
	name, ok := names[optionErr.Option]
	if !ok {
		return err
	}

	expected := optionErr.Expected
	if name.Expected != "" {
		expected = name.Expected
	}
	if expected == "" {
		return fmt.Errorf("invalid %s %q", name.Flag, optionErr.Value)
	}
	return fmt.Errorf("invalid %s %q: expected %s", name.Flag, optionErr.Value, expected)
}
