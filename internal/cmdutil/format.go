// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package cmdutil

import (
	"strings"

	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
)

// This file holds the formatters that take SDK types. Everything they produce
// is plain text, but the SDK dependency is what keeps them out of
// internal/output, which formats values rather than models.

// FormatGeoLocation formats the most useful location label available.
func FormatGeoLocation(location *nvfleetint.GeoLocation) string {
	if location == nil {
		return "-"
	}
	if strings.TrimSpace(location.Region) != "" {
		return location.Region
	}

	parts := make([]string, 0, 2)
	if strings.TrimSpace(location.City) != "" {
		parts = append(parts, location.City)
	}
	if strings.TrimSpace(location.Country) != "" {
		parts = append(parts, location.Country)
	}
	if len(parts) > 0 {
		return strings.Join(parts, ", ")
	}

	return "-"
}
