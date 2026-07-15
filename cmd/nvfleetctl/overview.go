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

package main

import (
	"io"
	"strconv"
	"strings"

	clioutput "github.com/NVIDIA/fleet-intelligence-client/internal/output"
	"github.com/NVIDIA/fleet-intelligence-client/pkg/fleetintelligence"

	"github.com/spf13/cobra"
)

// Stores local flag values for overview
type overviewFlags struct {
	includeMetrics bool
}

// Creates the top-level overview command
func newOverviewCmd() *cobra.Command {
	flags := overviewFlags{includeMetrics: true}
	common := newCommonFlags()

	cmd := &cobra.Command{
		Use:   "overview",
		Short: "Show the system and fleet overview",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runOverview(cmd, flags, resolveCommonFlags(cmd, common))
		},
	}

	cmd.Flags().BoolVar(&flags.includeMetrics, "include-metrics", flags.includeMetrics, "Include fleet metrics in the response; use --include-metrics=false to omit")
	registerReadCommonFlags(cmd, common)

	return cmd
}

// Validates flags, calls the SDK, and writes output
func runOverview(cmd *cobra.Command, flags overviewFlags, common resolvedCommonFlags) error {
	if err := validateReadCommonFlags(common); err != nil {
		return err
	}

	client, err := newConfiguredClient(commonClientOptions(common)...)
	if err != nil {
		return err
	}

	opts := fleetintelligence.OverviewOptions{}
	if cmd.Flags().Changed("include-metrics") {
		opts.IncludeMetrics = &flags.includeMetrics
	}

	overview, err := client.GetOverview(cmd.Context(), opts)
	if err != nil {
		return err
	}

	if common.output == clioutput.FormatJSON {
		return clioutput.WriteRawJSON(cmd.OutOrStdout(), overview.RawJSON)
	}
	return writeOverviewTable(cmd.OutOrStdout(), overview)
}

// Renders overview summary and metrics as a table
func writeOverviewTable(w io.Writer, overview fleetintelligence.Overview) error {
	return clioutput.WriteTable(w, []string{"FIELD", "VALUE"}, overviewRows(overview))
}

// Converts overview data into table rows
func overviewRows(overview fleetintelligence.Overview) [][]string {
	rows := [][]string{
		{"NODES", clioutput.FormatOptionalInt(overview.NodesCount)},
		{"GPUS", clioutput.FormatOptionalInt(overview.GPUsCount)},
		{"CPU CORES", clioutput.FormatOptionalInt(overview.CPUCoresCount)},
		{"HEALTHY NODES", clioutput.FormatOptionalInt(overview.HealthyNodeCount)},
		{"DEGRADED NODES", clioutput.FormatOptionalInt(overview.DegradedNodeCount)},
		{"UNHEALTHY NODES", clioutput.FormatOptionalInt(overview.UnhealthyNodeCount)},
		{"UNKNOWN NODES", clioutput.FormatOptionalInt(overview.UnknownNodeCount)},
		{"HEALTH PERCENTAGE", clioutput.FormatOptionalPercentage(overview.HealthPercentage)},
		{"NODE GROUPS", clioutput.FormatOptionalInt(overview.NodeGroupCount)},
		{"COMPUTE ZONES", clioutput.FormatOptionalInt(overview.ComputeZoneCount)},
	}
	for _, metric := range overview.Metrics {
		rows = append(rows, []string{metricLabel(metric), metricValue(metric)})
	}
	return rows
}

// Builds a table label for a metric. The metric name is preserved verbatim
// (not upper-cased) so identifiers stay copy-pasteable and do not collide with
// the fixed upper-case summary labels above.
func metricLabel(metric fleetintelligence.OverviewMetric) string {
	label := metric.Name
	if strings.TrimSpace(label) == "" {
		label = metric.Description
	}
	return clioutput.DisplayString(label)
}

// Formats a metric value with its unit. A "%" unit is appended without a space
// to match the summary health-percentage formatting ("70%"); other units are
// space-separated ("58 C").
func metricValue(metric fleetintelligence.OverviewMetric) string {
	if metric.Value == nil {
		return "-"
	}
	value := strconv.FormatFloat(float64(*metric.Value), 'f', -1, 32)
	switch unit := strings.TrimSpace(metric.Unit); unit {
	case "":
		return value
	case "%":
		return value + unit
	default:
		return value + " " + unit
	}
}
