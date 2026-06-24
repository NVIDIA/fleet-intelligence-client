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

// Package output formats nvfleetctl command responses.
package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/NVIDIA/fleet-intelligence-client/pkg/fleetintelligence"
)

const (
	FormatTable = "table"
	FormatJSON  = "json"
)

// Represents pagination metadata for table footers
type Pagination struct {
	Page     int
	PageSize int
	Total    int
	HasMore  bool
}

// Reports whether format is a supported output format
func IsValidFormat(format string) bool {
	return format == FormatTable || format == FormatJSON
}

// Writes value as a single JSON document
func WriteJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	return encoder.Encode(value)
}

// Writes a raw JSON payload with a trailing newline
func WriteRawJSON(w io.Writer, data []byte) error {
	data = bytes.TrimRight(data, "\n")
	if len(data) == 0 {
		data = []byte("{}")
	}
	if _, err := w.Write(data); err != nil {
		return err
	}
	_, err := fmt.Fprintln(w)
	return err
}

// Returns a printable placeholder for empty strings
func DisplayString(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

// Formats a display name with its stable identifier
func FormatNameAndID(name, id string) string {
	name = strings.TrimSpace(name)
	id = strings.TrimSpace(id)
	switch {
	case name != "" && id != "":
		return fmt.Sprintf("%s (%s)", name, id)
	case name != "":
		return name
	case id != "":
		return id
	default:
		return "-"
	}
}

// Returns the display name when present or the ID otherwise
func FormatNameOrID(name, id string) string {
	name = strings.TrimSpace(name)
	id = strings.TrimSpace(id)
	if name != "" {
		return name
	}
	return DisplayString(id)
}

// Formats optional integers for table output
func FormatOptionalInt(value *int) string {
	if value == nil {
		return "-"
	}
	return strconv.Itoa(*value)
}

// Formats optional percentage values for table output
func FormatOptionalPercentage(value *float32) string {
	if value == nil {
		return "-"
	}
	return strconv.FormatFloat(float64(*value), 'f', -1, 32) + "%"
}

// Formats non-empty strings as a comma-separated list
func FormatStringList(values []string) string {
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			cleaned = append(cleaned, value)
		}
	}
	if len(cleaned) == 0 {
		return "-"
	}
	return strings.Join(cleaned, ", ")
}

// Formats the most useful location label available
func FormatGeoLocation(location *fleetintelligence.GeoLocation) string {
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

// Writes headers and rows as tab-aligned text
func WriteTable(w io.Writer, headers []string, rows [][]string) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if err := writeTableRow(tw, headers); err != nil {
		return err
	}
	for _, row := range rows {
		if err := writeTableRow(tw, row); err != nil {
			return err
		}
	}
	return tw.Flush()
}

// Writes the standard single-page pagination footer
func WritePaginationFooter(w io.Writer, page Pagination) error {
	_, err := fmt.Fprintf(w, "Page: %d  Page Size: %d  Total: %d  Has More: %t\n", page.Page, page.PageSize, page.Total, page.HasMore)
	return err
}

// Writes one tab-separated table row
func writeTableRow(w io.Writer, fields []string) error {
	_, err := fmt.Fprintln(w, strings.Join(fields, "\t"))
	return err
}
