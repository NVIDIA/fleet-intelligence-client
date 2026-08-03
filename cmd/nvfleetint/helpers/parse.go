// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

// Package helpers contains shared nvfleetint command helpers.
package helpers

import (
	"fmt"
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
