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
	"errors"
	"io"
	"strings"

	clioutput "github.com/NVIDIA/fleet-intelligence-client/internal/output"
	"github.com/NVIDIA/fleet-intelligence-client/pkg/fleetintelligence"

	"github.com/spf13/cobra"
)

// Stores local flag values for tag list
type tagListFlags struct {
	prefix      string
	node        string
	nodegroup   string
	computezone string
}

// Creates the top-level tag command group
func newTagCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tag",
		Short: "Inspect tags",
	}

	cmd.AddCommand(newTagListCmd())

	return cmd
}

// Creates the tag list command
func newTagListCmd() *cobra.Command {
	flags := tagListFlags{}
	common := newCommonFlags()

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List unique tags",
		Long: `List the unique customer tags.

Use --prefix to filter by prefix. Scope the results to a single resource with at
most one of --node, --nodegroup, or --computezone; --prefix may be combined with
a resource filter.`,
		Example: `  nvfleetctl tag list
  nvfleetctl tag list --prefix gpu
  nvfleetctl tag list --node 1e9c0d2a-0000-4a1b-9c3d-000000000001`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runTagList(cmd, flags, resolveCommonFlags(cmd, common))
		},
	}

	cmd.Flags().StringVar(&flags.prefix, "prefix", "", "Filter tags by prefix (case-insensitive)")
	cmd.Flags().StringVar(&flags.node, "node", "", "Filter to tags from a specific node UUID")
	cmd.Flags().StringVar(&flags.nodegroup, "nodegroup", "", "Filter to tags from nodes in a specific node group ID")
	cmd.Flags().StringVar(&flags.computezone, "computezone", "", "Filter to tags from nodes in a specific compute zone ID")
	registerReadCommonFlags(cmd, common)

	return cmd
}

// Validates flags, calls the SDK, and writes output
func runTagList(cmd *cobra.Command, flags tagListFlags, common resolvedCommonFlags) error {
	if err := validateReadCommonFlags(common); err != nil {
		return err
	}
	if err := validateTagListFlags(flags); err != nil {
		return err
	}

	client, err := newConfiguredClient(commonClientOptions(common)...)
	if err != nil {
		return err
	}

	tags, err := client.ListTags(cmd.Context(), fleetintelligence.TagListOptions{
		Prefix:        strings.TrimSpace(flags.prefix),
		NodeUUID:      strings.TrimSpace(flags.node),
		NodeGroupID:   strings.TrimSpace(flags.nodegroup),
		ComputeZoneID: strings.TrimSpace(flags.computezone),
	})
	if err != nil {
		return err
	}

	if common.output == clioutput.FormatJSON {
		return clioutput.WriteRawJSON(cmd.OutOrStdout(), tags.RawJSON)
	}
	return writeTagListTable(cmd.OutOrStdout(), tags.Tags)
}

// Rejects more than one resource-scoped tag filter at the CLI layer
func validateTagListFlags(flags tagListFlags) error {
	count := 0
	if strings.TrimSpace(flags.node) != "" {
		count++
	}
	if strings.TrimSpace(flags.nodegroup) != "" {
		count++
	}
	if strings.TrimSpace(flags.computezone) != "" {
		count++
	}
	if count > 1 {
		return errors.New("at most one of --node, --nodegroup, or --computezone may be used")
	}
	return nil
}

// Renders tags as a single-column table
func writeTagListTable(w io.Writer, tags []string) error {
	rows := make([][]string, 0, len(tags))
	for _, tag := range tags {
		rows = append(rows, []string{clioutput.DisplayString(tag)})
	}
	return clioutput.WriteTable(w, []string{"TAG"}, rows)
}
