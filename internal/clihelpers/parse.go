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

// ParseTypedList splits a comma-separated flag value into a string-based API
// type. flag names the flag when the list itself is malformed.
//
// It deliberately does not check the values: the SDK validates them, so the
// accepted values are stated in one place and the two layers cannot drift.
func ParseTypedList[T ~string](flag, raw string) ([]T, error) {
	values, err := ParseCommaList(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %w", flag, err)
	}
	if len(values) == 0 {
		return nil, nil
	}

	out := make([]T, 0, len(values))
	for _, value := range values {
		out = append(out, T(value))
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
