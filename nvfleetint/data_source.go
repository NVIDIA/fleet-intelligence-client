// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

// DataSource selects a node or inventory source, or identifies an alert or event producer.
type DataSource string

const (
	DataSourceInband DataSource = "inband"
	DataSourceOOB    DataSource = "oob"
	dataSourceValues            = "inband or oob"
)

// Valid reports whether the source is one of the supported explicit values.
func (source DataSource) Valid() bool {
	switch source {
	case DataSourceInband, DataSourceOOB:
		return true
	default:
		return false
	}
}

// Validate checks an optional source. Empty preserves the endpoint's default.
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
