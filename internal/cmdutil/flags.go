// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package cmdutil

import (
	"errors"
	"fmt"
	"time"

	"github.com/NVIDIA/fleet-intelligence-client/internal/config"
	clioutput "github.com/NVIDIA/fleet-intelligence-client/internal/output"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"

	"github.com/spf13/cobra"
)

// Common stores shared output, pagination, and credential flag values.
type Common struct {
	Output   string
	All      bool
	Page     int
	PageSize int
	Timeout  time.Duration
	Profile  string
}

// Resolved stores common flag values plus explicit-set state.
type Resolved struct {
	Output      string
	All         bool
	Page        int
	PageSize    int
	Timeout     time.Duration
	Profile     string
	OutputSet   bool
	AllSet      bool
	PageSet     bool
	PageSizeSet bool
	TimeoutSet  bool
	ProfileSet  bool
}

// NewCommon creates common flags with default values.
func NewCommon() *Common {
	return &Common{
		Output:  clioutput.FormatTable,
		Timeout: nvfleetint.DefaultTimeout,
	}
}

// RegisterOutputFlag registers the output flag on a command.
func RegisterOutputFlag(cmd *cobra.Command, flags *Common) {
	cmd.Flags().StringVarP(&flags.Output, "output", "o", flags.Output, "Output format: table or json")
}

// RegisterPaginationFlags registers pagination flags on a command.
func RegisterPaginationFlags(cmd *cobra.Command, flags *Common) {
	cmd.Flags().BoolVar(&flags.All, "all", false, "Fetch all pages")
	cmd.Flags().IntVar(&flags.Page, "page", 1, "Page number to fetch (1-based)")
	cmd.Flags().IntVar(&flags.PageSize, "page-size", 0, "Page size to fetch")
}

// RegisterTimeoutFlag registers the request timeout flag on a command.
func RegisterTimeoutFlag(cmd *cobra.Command, flags *Common) {
	cmd.Flags().DurationVar(&flags.Timeout, "timeout", flags.Timeout, "Request timeout (e.g. 30s, 2m); must be greater than 0")
}

// RegisterProfileFlag registers the credential profile selector on an
// API-backed command. The `auth` CRUD commands take the profile they change as
// a positional argument instead, so `--profile` only ever means "credentials
// for this invocation".
func RegisterProfileFlag(cmd *cobra.Command, flags *Common) {
	cmd.Flags().StringVar(&flags.Profile, "profile", "", "Credential profile to use; defaults to the current profile")
}

// RegisterListFlags registers output, pagination, and credential flags on a
// list command.
func RegisterListFlags(cmd *cobra.Command, flags *Common) {
	RegisterOutputFlag(cmd, flags)
	RegisterPaginationFlags(cmd, flags)
	RegisterTimeoutFlag(cmd, flags)
	RegisterProfileFlag(cmd, flags)
}

// RegisterReadFlags registers output and credential flags on a read command.
func RegisterReadFlags(cmd *cobra.Command, flags *Common) {
	RegisterOutputFlag(cmd, flags)
	RegisterTimeoutFlag(cmd, flags)
	RegisterProfileFlag(cmd, flags)
}

// ResolveCommon returns common flag values and whether they were supplied.
func ResolveCommon(cmd *cobra.Command, flags *Common) Resolved {
	return Resolved{
		Output:      flags.Output,
		All:         flags.All,
		Page:        flags.Page,
		PageSize:    flags.PageSize,
		Timeout:     flags.Timeout,
		Profile:     flags.Profile,
		OutputSet:   cmd.Flags().Changed("output"),
		AllSet:      cmd.Flags().Changed("all"),
		PageSet:     cmd.Flags().Changed("page"),
		PageSizeSet: cmd.Flags().Changed("page-size"),
		TimeoutSet:  cmd.Flags().Changed("timeout"),
		ProfileSet:  cmd.Flags().Changed("profile"),
	}
}

// ValidateListFlags checks common flags for list-style commands.
func ValidateListFlags(flags Resolved) error {
	if !clioutput.IsValidFormat(flags.Output) {
		return fmt.Errorf("invalid output %q: expected table or json", flags.Output)
	}
	if flags.All && flags.PageSet {
		return errors.New("--page cannot be used with --all")
	}
	if flags.PageSet && flags.Page < 1 {
		return errors.New("--page must be greater than or equal to 1")
	}
	if flags.PageSizeSet && (flags.PageSize < MinPageSize || flags.PageSize > MaxPageSize) {
		return fmt.Errorf("--page-size must be between %d and %d", MinPageSize, MaxPageSize)
	}
	return ValidateReadFlags(flags)
}

// ValidateReadFlags checks common flags for non-paginated read commands.
func ValidateReadFlags(flags Resolved) error {
	if !clioutput.IsValidFormat(flags.Output) {
		return fmt.Errorf("invalid output %q: expected table or json", flags.Output)
	}
	if flags.Timeout <= 0 {
		return errors.New("--timeout must be greater than 0")
	}
	if flags.ProfileSet {
		return config.ValidateProfileName(flags.Profile)
	}
	return nil
}

// ApplyPagination applies explicitly supplied pagination flags to request
// options.
func ApplyPagination(flags Resolved, setPage func(*int), setPageSize func(*int)) {
	if flags.PageSet {
		// --page is 1-based; the SDK uses a 0-based paging contract.
		page := flags.Page - 1
		setPage(&page)
	}
	if flags.PageSizeSet {
		pageSize := flags.PageSize
		setPageSize(&pageSize)
	} else if flags.All {
		// Default --all to the max page size so fetching every page takes fewer requests
		pageSize := MaxPageSize
		setPageSize(&pageSize)
	}
}
