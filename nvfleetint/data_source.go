// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

// DataSource selects an inventory source or an alert, event, error report, or XID observation path.
type DataSource string

const (
	DataSourceInband DataSource = "inband"
	DataSourceOOB    DataSource = "oob"
	// DataSourceAll selects both observation paths on XID burst list/options only.
	DataSourceAll    DataSource = "all"
	dataSourceValues            = "inband or oob"
)

// Valid reports whether the value is a concrete source (inband or oob).
// The all selector is validated separately by endpoints that support it.
func (source DataSource) Valid() bool {
	switch source {
	case DataSourceInband, DataSourceOOB:
		return true
	default:
		return false
	}
}

// Validate checks an optional concrete source. Empty preserves the endpoint's
// default; XID burst options validate their additional all selector separately.
func (source DataSource) Validate() error {
	if source != "" && !source.Valid() {
		return invalidOption("dataSource", "data source", string(source), dataSourceValues)
	}
	return nil
}

// Lists may be omitted, but each supplied source must be explicit and valid.
func validateDataSources(sources []DataSource) error {
	for _, source := range sources {
		if !source.Valid() {
			return invalidOption("dataSources", "data source", string(source), dataSourceValues)
		}
	}
	return nil
}
