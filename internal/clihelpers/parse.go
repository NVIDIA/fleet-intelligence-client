// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

// Package clihelpers contains shared nvfleetint command helpers.
package clihelpers

import (
	"fmt"
	"strconv"
	"strings"
)

// Splits a comma-separated flag value into trimmed values
func ParseCommaList(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" {
			return nil, fmt.Errorf("invalid comma-separated list %q: empty values are not allowed", raw)
		}
		values = append(values, value)
	}

	return values, nil
}

// Enum is a CLI-facing API enum: a string type that can report whether the
// backend accepts a given value.
type Enum interface {
	~string
	Valid() bool
}

// ParseEnumList splits a comma-separated flag value and converts each entry
// into T, rejecting any value the API does not accept. flag names the flag in
// the error and expected lists the accepted values, so the message reads in the
// CLI's own vocabulary rather than the backend's.
func ParseEnumList[T Enum](flag, raw, expected string) ([]T, error) {
	values, err := ParseCommaList(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %w", flag, err)
	}
	if len(values) == 0 {
		return nil, nil
	}

	out := make([]T, 0, len(values))
	for _, value := range values {
		converted := T(value)
		if !converted.Valid() {
			return nil, fmt.Errorf("invalid %s %q: expected %s", flag, value, expected)
		}
		out = append(out, converted)
	}

	return out, nil
}

// ParseIntList splits a comma-separated flag value into integers, rejecting
// negative values. flag names the flag in the error.
func ParseIntList(flag, raw string) ([]int, error) {
	values, err := ParseCommaList(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %w", flag, err)
	}
	if len(values) == 0 {
		return nil, nil
	}

	out := make([]int, 0, len(values))
	for _, value := range values {
		number, err := strconv.Atoi(value)
		if err != nil {
			return nil, fmt.Errorf("invalid %s %q: expected an integer", flag, value)
		}
		if number < 0 {
			return nil, fmt.Errorf("invalid %s %q: expected a non-negative integer", flag, value)
		}
		out = append(out, number)
	}

	return out, nil
}
