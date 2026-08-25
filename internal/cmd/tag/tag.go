// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package tag

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdutil"
	clioutput "github.com/NVIDIA/fleet-intelligence-client/internal/output"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"

	"github.com/spf13/cobra"
)

// Stores local flag values for tag set
type tagSetFlags struct {
	tags  string
	clear bool
	yes   bool
}

// Stores local flag values for tag list
type tagListFlags struct {
	prefix      string
	node        string
	nodegroup   string
	computezone string
}

// Creates the top-level tag command group
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tag",
		Short: "Inspect and set tags",
	}

	cmd.AddCommand(newTagListCmd())
	cmd.AddCommand(newTagSetCmd())
	cmdutil.RejectUnknownSubcommands(cmd)

	return cmd
}

// Creates the tag list command
func newTagListCmd() *cobra.Command {
	flags := tagListFlags{}
	common := cmdutil.NewCommon()

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List unique tags",
		Args:  cobra.NoArgs,
		Long: `List the unique customer tags.

Use --prefix to filter by prefix. Scope the results to a single resource with at
most one of --node, --nodegroup, or --computezone; --prefix may be combined with
a resource filter.`,
		Example: `  nvfleetint tag list
  nvfleetint tag list --prefix gpu
  nvfleetint tag list --node 1e9c0d2a-0000-4a1b-9c3d-000000000001`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runTagList(cmd, flags, cmdutil.ResolveCommon(cmd, common))
		},
	}

	cmd.Flags().StringVar(&flags.prefix, "prefix", "", "Filter tags by prefix (case-insensitive)")
	cmd.Flags().StringVar(&flags.node, "node", "", "Filter to tags from a specific node UUID")
	cmd.Flags().StringVar(&flags.nodegroup, "nodegroup", "", "Filter to tags from nodes in a specific node group ID")
	cmd.Flags().StringVar(&flags.computezone, "computezone", "", "Filter to tags from nodes in a specific compute zone ID")
	cmdutil.RegisterReadFlags(cmd, common)

	return cmd
}

// Validates flags, calls the SDK, and writes output
func runTagList(cmd *cobra.Command, flags tagListFlags, common cmdutil.Resolved) error {
	if err := cmdutil.ValidateReadFlags(common); err != nil {
		return err
	}
	if err := validateTagListFlags(flags); err != nil {
		return err
	}

	client, err := cmdutil.New(common)
	if err != nil {
		return err
	}

	tags, err := client.ListTags(cmd.Context(), nvfleetint.TagListOptions{
		Prefix:        strings.TrimSpace(flags.prefix),
		NodeUUID:      strings.TrimSpace(flags.node),
		NodeGroupID:   strings.TrimSpace(flags.nodegroup),
		ComputeZoneID: strings.TrimSpace(flags.computezone),
	})
	if err != nil {
		return err
	}

	if common.Output == clioutput.FormatJSON {
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

// Creates the tag set command
func newTagSetCmd() *cobra.Command {
	flags := tagSetFlags{}
	common := cmdutil.NewCommon()

	cmd := &cobra.Command{
		Use:   "set <node-uuid>",
		Short: "Replace a node's tags",
		Args:  cmdutil.RequireSingleArg("node UUID"),
		Long: `Replace every tag on a node with the tags given.

This replaces rather than adds: a tag the node already carries that is not
listed in --tags is removed. Use --clear to remove all of them. Exactly one of
--tags or --clear is required, and the command confirms before writing unless
--yes is passed.

Tags use lowercase letters, digits, hyphens, and underscores. They must start
and end with a letter or digit, cannot contain consecutive separators, and are
at most 50 characters. The names null, none, undefined, true, and false are
reserved.

Run 'nvfleetint tag list --node <node-uuid>' to see a node's current tags.`,
		Example: `  nvfleetint tag set 1e9c0d2a-0000-4a1b-9c3d-000000000001 --tags gpu-health,burn_in
  nvfleetint tag set 1e9c0d2a-0000-4a1b-9c3d-000000000001 --clear --yes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTagSet(cmd, args[0], flags, cmdutil.ResolveCommon(cmd, common))
		},
	}

	cmd.Flags().StringVar(&flags.tags, "tags", "", "Comma-separated tags the node should carry, replacing its current tags")
	cmd.Flags().BoolVar(&flags.clear, "clear", false, "Remove every tag from the node")
	cmd.Flags().BoolVar(&flags.yes, "yes", false, "Skip the confirmation prompt")
	cmdutil.RegisterReadFlags(cmd, common)

	return cmd
}

// Validates flags, confirms the replacement, calls the SDK, and writes output
func runTagSet(cmd *cobra.Command, nodeUUID string, flags tagSetFlags, common cmdutil.Resolved) error {
	if err := cmdutil.ValidateReadFlags(common); err != nil {
		return err
	}
	tags, err := resolveTagSetTags(cmd, flags)
	if err != nil {
		return err
	}

	nodeUUID = strings.TrimSpace(nodeUUID)
	if nodeUUID == "" {
		return errors.New("node UUID is required")
	}

	client, err := cmdutil.New(common)
	if err != nil {
		return err
	}

	// Confirm after the client is built so a bad profile fails before the
	// question rather than after it.
	if !flags.yes {
		summary := tagSetSummary(nodeUUID, tags)
		if err := cmdutil.Confirm(cmd.InOrStdin(), cmd.ErrOrStderr(), summary); err != nil {
			return err
		}
	}

	result, err := client.SetNodeTags(cmd.Context(), nodeUUID, nvfleetint.SetNodeTagsOptions{Tags: tags})
	if err != nil {
		return err
	}

	if common.Output == clioutput.FormatJSON {
		return clioutput.WriteRawJSON(cmd.OutOrStdout(), result.RawJSON)
	}
	return writeNodeTagsTable(cmd.OutOrStdout(), result)
}

// Resolves --tags and --clear into the tag set to write. Requiring one of the
// two makes clearing a node's tags something the user asks for by name: an
// empty --tags would otherwise be a wipe that looks like a no-op.
func resolveTagSetTags(cmd *cobra.Command, flags tagSetFlags) ([]string, error) {
	tagsSet := cmd.Flags().Changed("tags")
	switch {
	case tagsSet && flags.clear:
		return nil, errors.New("--tags cannot be used with --clear")
	case flags.clear:
		return nil, nil
	case !tagsSet:
		return nil, errors.New("exactly one of --tags or --clear is required")
	}

	tags, err := cmdutil.ParseCommaList(flags.tags)
	if err != nil {
		return nil, fmt.Errorf("invalid --tags: %w", err)
	}
	if len(tags) == 0 {
		return nil, errors.New("--tags requires at least one tag; use --clear to remove every tag")
	}

	return tags, nil
}

// Describes the pending write for the confirmation prompt
func tagSetSummary(nodeUUID string, tags []string) string {
	if len(tags) == 0 {
		return fmt.Sprintf("This removes every tag from node %s.", nodeUUID)
	}
	return fmt.Sprintf(
		"This replaces every tag on node %s with: %s",
		nodeUUID, strings.Join(tags, ", "),
	)
}

// Renders one node's resulting tags
func writeNodeTagsTable(w io.Writer, result nvfleetint.NodeTags) error {
	return clioutput.WriteTable(w,
		[]string{"NODE UUID", "TAGS"},
		[][]string{{clioutput.DisplayString(result.NodeUUID), clioutput.FormatStringList(result.Tags)}},
	)
}
