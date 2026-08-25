// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package overview

import (
	"io"
	"strconv"
	"strings"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdutil"
	clioutput "github.com/NVIDIA/fleet-intelligence-client/internal/output"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"

	"github.com/spf13/cobra"
)

// Stores local flag values for overview
type overviewFlags struct {
	includeMetrics bool
}

// Creates the top-level overview command
func NewCmd() *cobra.Command {
	flags := overviewFlags{includeMetrics: true}
	common := cmdutil.NewCommon()

	cmd := &cobra.Command{
		Use:   "overview",
		Short: "Show the system and fleet overview",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runOverview(cmd, flags, cmdutil.ResolveCommon(cmd, common))
		},
	}

	cmd.Flags().BoolVar(&flags.includeMetrics, "include-metrics", flags.includeMetrics, "Include fleet metrics in the response; use --include-metrics=false to omit")
	cmdutil.RegisterReadFlags(cmd, common)

	return cmd
}

// Validates flags, calls the SDK, and writes output
func runOverview(cmd *cobra.Command, flags overviewFlags, common cmdutil.Resolved) error {
	if err := cmdutil.ValidateReadFlags(common); err != nil {
		return err
	}

	client, err := cmdutil.New(common)
	if err != nil {
		return err
	}

	opts := nvfleetint.OverviewOptions{}
	if cmd.Flags().Changed("include-metrics") {
		opts.IncludeMetrics = &flags.includeMetrics
	}

	overview, err := client.GetOverview(cmd.Context(), opts)
	if err != nil {
		return err
	}

	if common.Output == clioutput.FormatJSON {
		return clioutput.WriteRawJSON(cmd.OutOrStdout(), overview.RawJSON)
	}
	return writeOverviewTable(cmd.OutOrStdout(), overview)
}

// Renders overview summary and metrics as a table
func writeOverviewTable(w io.Writer, overview nvfleetint.Overview) error {
	return clioutput.WriteTable(w, []string{"FIELD", "VALUE"}, overviewRows(overview))
}

// Converts overview data into table rows
func overviewRows(overview nvfleetint.Overview) [][]string {
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
func metricLabel(metric nvfleetint.OverviewMetric) string {
	label := metric.Name
	if strings.TrimSpace(label) == "" {
		label = metric.Description
	}
	return clioutput.DisplayString(label)
}

// Formats a metric value with its unit. A "%" unit is appended without a space
// to match the summary health-percentage formatting ("70%"); other units are
// space-separated ("58 C").
func metricValue(metric nvfleetint.OverviewMetric) string {
	if metric.Value == nil {
		return "-"
	}
	unit := strings.TrimSpace(metric.Unit)
	if unit == "%" {
		// Reuse the shared percentage formatter so a "%" metric renders
		// identically to the summary health-percentage row ("70%").
		return clioutput.FormatOptionalPercentage(metric.Value)
	}
	value := strconv.FormatFloat(float64(*metric.Value), 'f', -1, 32)
	if unit == "" {
		return value
	}
	return value + " " + unit
}
