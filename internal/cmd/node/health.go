// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package node

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdutil"
	clioutput "github.com/NVIDIA/fleet-intelligence-client/internal/output"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"

	"github.com/spf13/cobra"
)

// Stores local flag values for node health
type nodeHealthFlags struct {
	start string
	end   string
}

// Creates the node health command
func newNodeHealthCmd() *cobra.Command {
	flags := nodeHealthFlags{}
	common := cmdutil.NewCommon()

	cmd := &cobra.Command{
		Use:   "health <uuid>",
		Short: "Show a node's health history",
		Long: `Show the health status timeline and summary for a node over a time window.

Both --start and --end are required and must be RFC3339 timestamps.`,
		Example: `  nvfleetint node health 1e9c0d2a-0000-4a1b-9c3d-000000000001 --start 2026-04-07T00:00:00Z --end 2026-04-14T00:00:00Z`,
		Args:    cmdutil.RequireSingleArg("node UUID"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runNodeHealth(cmd, args[0], flags, cmdutil.ResolveCommon(cmd, common))
		},
	}

	cmd.Flags().StringVar(&flags.start, "start", "", "Start time in RFC3339 format (required)")
	cmd.Flags().StringVar(&flags.end, "end", "", "End time in RFC3339 format (required)")
	cmdutil.RegisterReadFlags(cmd, common)

	return cmd
}

// Validates args and flags, calls the SDK, and writes output
func runNodeHealth(cmd *cobra.Command, nodeUUID string, flags nodeHealthFlags, common cmdutil.Resolved) error {
	if err := cmdutil.ValidateReadFlags(common); err != nil {
		return err
	}

	nodeUUID = strings.TrimSpace(nodeUUID)
	if nodeUUID == "" {
		return errors.New("node UUID is required")
	}

	start := strings.TrimSpace(flags.start)
	end := strings.TrimSpace(flags.end)
	if start == "" || end == "" {
		return errors.New("--start and --end are required")
	}
	if err := cmdutil.ValidateRFC3339Flag("--start", start); err != nil {
		return err
	}
	if err := cmdutil.ValidateRFC3339Flag("--end", end); err != nil {
		return err
	}

	client, err := cmdutil.New(common)
	if err != nil {
		return err
	}

	history, err := client.NodeHealthHistory(cmd.Context(), nodeUUID, nvfleetint.NodeHealthHistoryOptions{
		StartTime: start,
		EndTime:   end,
	})
	if err != nil {
		return err
	}

	if common.Output == clioutput.FormatJSON {
		return clioutput.WriteRawJSON(cmd.OutOrStdout(), history.RawJSON)
	}
	return writeNodeHealthTable(cmd.OutOrStdout(), nodeUUID, history)
}

// Renders node health metadata, summary, and timeline as separate tables.
// The three tables mirror the three JSON keys: enrolledAt (meta), healthSummary
// (labeled "Health Summary"), and machineStatus (labeled "Machine Status").
func writeNodeHealthTable(w io.Writer, nodeUUID string, history nvfleetint.NodeHealthHistory) error {
	if err := clioutput.WriteTable(w, []string{"FIELD", "VALUE"}, nodeHealthMetaRows(nodeUUID, history)); err != nil {
		return err
	}
	if history.HealthSummary != nil {
		if _, err := fmt.Fprintln(w, "\nHealth Summary"); err != nil {
			return err
		}
		if err := clioutput.WriteTable(w, []string{"STATE", "PERCENTAGE", "DURATION"}, nodeHealthSummaryRows(history.HealthSummary)); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w, "\nMachine Status"); err != nil {
		return err
	}
	if len(history.Segments) == 0 {
		_, err := fmt.Fprintln(w, "No health timeline segments in the requested window.")
		return err
	}
	return clioutput.WriteTable(w, []string{"STATUS", "START TIME", "END TIME"}, nodeHealthSegmentRows(history.Segments))
}

// Converts node identity and enrollment metadata into FIELD/VALUE rows
func nodeHealthMetaRows(nodeUUID string, history nvfleetint.NodeHealthHistory) [][]string {
	return [][]string{
		{"UUID", clioutput.DisplayString(nodeUUID)},
		{"ENROLLED AT", clioutput.DisplayString(history.EnrolledAt)},
	}
}

// Converts a health summary into per-state percentage/duration rows
func nodeHealthSummaryRows(summary *nvfleetint.NodeHealthSummary) [][]string {
	return [][]string{
		{"Healthy", clioutput.FormatOptionalPercentage(summary.HealthyPercentage), formatDurationSeconds(summary.HealthyDurationSeconds)},
		{"Degraded", clioutput.FormatOptionalPercentage(summary.DegradedPercentage), formatDurationSeconds(summary.DegradedDurationSeconds)},
		{"Unhealthy", clioutput.FormatOptionalPercentage(summary.UnhealthyPercentage), formatDurationSeconds(summary.UnhealthyDurationSeconds)},
	}
}

// Converts health timeline segments into table rows
func nodeHealthSegmentRows(segments []nvfleetint.NodeHealthSegment) [][]string {
	rows := make([][]string, 0, len(segments))
	for _, segment := range segments {
		rows = append(rows, []string{
			clioutput.DisplayString(segment.Status),
			clioutput.DisplayString(segment.StartTime),
			clioutput.DisplayString(segment.EndTime),
		})
	}
	return rows
}

// Formats a duration expressed in seconds as a human-readable string. The
// value is scaled in floating point before conversion so fractional seconds
// are preserved (time.Duration(*seconds) would truncate them to whole seconds).
func formatDurationSeconds(seconds *float32) string {
	if seconds == nil {
		return "-"
	}
	return time.Duration(float64(*seconds) * float64(time.Second)).String()
}
