// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	clihelpers "github.com/NVIDIA/fleet-intelligence-client/cmd/nvfleetint/helpers"
	"github.com/NVIDIA/fleet-intelligence-client/internal/config"
	clioutput "github.com/NVIDIA/fleet-intelligence-client/internal/output"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"

	"github.com/spf13/cobra"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

// Stores shared output, pagination, and credential flag values
type commonFlags struct {
	output   string
	all      bool
	page     int
	pageSize int
	timeout  time.Duration
	profile  string
}

// Stores common flag values plus explicit-set state
type resolvedCommonFlags struct {
	output      string
	all         bool
	page        int
	pageSize    int
	timeout     time.Duration
	profile     string
	outputSet   bool
	allSet      bool
	pageSet     bool
	pageSizeSet bool
	timeoutSet  bool
	profileSet  bool
}

// Runs the CLI with the provided context and arguments
func execute(ctx context.Context, args []string) error {
	cmd := newRootCmd()
	cmd.SetContext(ctx)
	cmd.SetArgs(args)

	return cmd.Execute()
}

// Creates the root nvfleetint command
func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "nvfleetint",
		Short:         "Fleet Intelligence CLI",
		Long:          "Fleet Intelligence CLI for the Fleet Intelligence customer API.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.AddCommand(newAuthCmd())
	cmd.AddCommand(newOverviewCmd())
	cmd.AddCommand(newComputeZoneCmd())
	cmd.AddCommand(newNodeGroupCmd())
	cmd.AddCommand(newNodeCmd())
	cmd.AddCommand(newAlertCmd())
	cmd.AddCommand(newEventCmd())
	cmd.AddCommand(newReportCmd())
	cmd.AddCommand(newTagCmd())
	cmd.AddCommand(newVersionCmd())

	return cmd
}

// Creates common flags with default values
func newCommonFlags() *commonFlags {
	return &commonFlags{
		output:  clioutput.FormatTable,
		timeout: nvfleetint.DefaultTimeout,
	}
}

// Registers the output flag on a command
func registerOutputFlag(cmd *cobra.Command, flags *commonFlags) {
	cmd.Flags().StringVarP(&flags.output, "output", "o", flags.output, "Output format: table or json")
}

// Registers pagination flags on a command
func registerPaginationFlags(cmd *cobra.Command, flags *commonFlags) {
	cmd.Flags().BoolVar(&flags.all, "all", false, "Fetch all pages")
	cmd.Flags().IntVar(&flags.page, "page", 1, "Page number to fetch (1-based)")
	cmd.Flags().IntVar(&flags.pageSize, "page-size", 0, "Page size to fetch")
}

// Registers the request timeout flag on a command
func registerTimeoutFlag(cmd *cobra.Command, flags *commonFlags) {
	cmd.Flags().DurationVar(&flags.timeout, "timeout", flags.timeout, "Request timeout (e.g. 30s, 2m); must be greater than 0")
}

// Registers the credential profile selector on an API-backed command. The
// `auth` CRUD commands take the profile they change as a positional argument
// instead, so `--profile` only ever means "credentials for this invocation".
func registerProfileFlag(cmd *cobra.Command, flags *commonFlags) {
	cmd.Flags().StringVar(&flags.profile, "profile", "", "Credential profile to use; defaults to the current profile")
}

// Registers output, pagination, and credential flags on a list command
func registerListCommonFlags(cmd *cobra.Command, flags *commonFlags) {
	registerOutputFlag(cmd, flags)
	registerPaginationFlags(cmd, flags)
	registerTimeoutFlag(cmd, flags)
	registerProfileFlag(cmd, flags)
}

// Registers output and credential flags on a read command
func registerReadCommonFlags(cmd *cobra.Command, flags *commonFlags) {
	registerOutputFlag(cmd, flags)
	registerTimeoutFlag(cmd, flags)
	registerProfileFlag(cmd, flags)
}

// Validates that exactly one positional argument was given, naming it in errors
func requireSingleArg(name string) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		switch {
		case len(args) == 0:
			return fmt.Errorf("%s is required", name)
		case len(args) > 1:
			return fmt.Errorf("only one %s may be given, got %d", name, len(args))
		}
		return nil
	}
}

// Validates that at most one positional argument was given. The caller supplies
// the meaning of an omitted argument; only the too-many case is an error here,
// and it is worded exactly as in requireSingleArg.
func optionalSingleArg(name string) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) > 1 {
			return fmt.Errorf("only one %s may be given, got %d", name, len(args))
		}
		return nil
	}
}

// Returns common flag values and whether pagination flags were supplied
func resolveCommonFlags(cmd *cobra.Command, flags *commonFlags) resolvedCommonFlags {
	return resolvedCommonFlags{
		output:      flags.output,
		all:         flags.all,
		page:        flags.page,
		pageSize:    flags.pageSize,
		timeout:     flags.timeout,
		profile:     flags.profile,
		outputSet:   cmd.Flags().Changed("output"),
		allSet:      cmd.Flags().Changed("all"),
		pageSet:     cmd.Flags().Changed("page"),
		pageSizeSet: cmd.Flags().Changed("page-size"),
		timeoutSet:  cmd.Flags().Changed("timeout"),
		profileSet:  cmd.Flags().Changed("profile"),
	}
}

// Checks common flags for list-style commands
func validateListCommonFlags(flags resolvedCommonFlags) error {
	if !clioutput.IsValidFormat(flags.output) {
		return fmt.Errorf("invalid output %q: expected table or json", flags.output)
	}
	if flags.all && flags.pageSet {
		return errors.New("--page cannot be used with --all")
	}
	if flags.pageSet && flags.page < 1 {
		return errors.New("--page must be greater than or equal to 1")
	}
	if flags.pageSizeSet && (flags.pageSize < clihelpers.MinPageSize || flags.pageSize > clihelpers.MaxPageSize) {
		return fmt.Errorf("--page-size must be between %d and %d", clihelpers.MinPageSize, clihelpers.MaxPageSize)
	}
	return validateReadCommonFlags(flags)
}

// Checks common flags for non-paginated read commands
func validateReadCommonFlags(flags resolvedCommonFlags) error {
	if !clioutput.IsValidFormat(flags.output) {
		return fmt.Errorf("invalid output %q: expected table or json", flags.output)
	}
	if flags.timeout <= 0 {
		return errors.New("--timeout must be greater than 0")
	}
	if flags.profileSet {
		return config.ValidateProfileName(flags.profile)
	}
	return nil
}

// Applies explicitly supplied pagination flags to request options
func applyPagination(flags resolvedCommonFlags, setPage func(*int), setPageSize func(*int)) {
	if flags.pageSet {
		// --page is 1-based; the SDK uses a 0-based paging contract.
		page := flags.page - 1
		setPage(&page)
	}
	if flags.pageSizeSet {
		pageSize := flags.pageSize
		setPageSize(&pageSize)
	} else if flags.all {
		// Default --all to the max page size so fetching every page takes fewer requests
		pageSize := clihelpers.MaxPageSize
		setPageSize(&pageSize)
	}
}

// Writes paginated list output as JSON, presenting the page number with the
// CLI's 1-based contract. rawJSON is a single API page; jsonValue is the merged
// result produced for --all.
func writePaginatedListJSON(w io.Writer, rawJSON []byte, jsonValue any) error {
	if rawJSON != nil {
		return clioutput.WriteRawJSON(w, clihelpers.OneIndexRawPage(rawJSON))
	}
	if merged, ok := jsonValue.(clihelpers.MergedJSONResult); ok {
		merged.Pagination.Page++
		return clioutput.WriteJSON(w, merged)
	}
	return clioutput.WriteJSON(w, jsonValue)
}

// Creates the version command
func newVersionCmd() *cobra.Command {
	common := newCommonFlags()
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		RunE: func(cmd *cobra.Command, _ []string) error {
			commonFlags := resolveCommonFlags(cmd, common)
			if err := validateReadCommonFlags(commonFlags); err != nil {
				return err
			}
			if commonFlags.output == clioutput.FormatJSON {
				return clioutput.WriteJSON(cmd.OutOrStdout(), versionOutput{
					Name:      "nvfleetint",
					Version:   version,
					Commit:    commit,
					BuildDate: buildDate,
				})
			}
			writeVersion(cmd.OutOrStdout())
			return nil
		},
	}
	registerOutputFlag(cmd, common)
	return cmd
}

type versionOutput struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"buildDate"`
}

// Writes binary version details
func writeVersion(w io.Writer) {
	fmt.Fprintf(w, "nvfleetint %s\ncommit: %s\nbuilt: %s\n", version, commit, buildDate)
}
