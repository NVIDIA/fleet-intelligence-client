// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"io"

	clioutput "github.com/NVIDIA/fleet-intelligence-client/internal/output"
	"github.com/NVIDIA/fleet-intelligence-client/internal/updatecheck"

	"github.com/spf13/cobra"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

// updateChecker is the release lookup used by `version` and `upgrade`. Tests
// point it at an httptest server instead of GitHub.
var updateChecker = updatecheck.Checker{}

type versionOutput struct {
	Name        string              `json:"name"`
	Version     string              `json:"version"`
	Commit      string              `json:"commit"`
	BuildDate   string              `json:"buildDate"`
	UpdateCheck *updatecheck.Result `json:"updateCheck,omitempty"`
}

// Creates the version command
func newVersionCmd() *cobra.Command {
	common := newCommonFlags()
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Long: "Print version information.\n\n" +
			"This also asks GitHub for the newest published release and prints an upgrade\n" +
			"notice when the running build is behind. The lookup is best effort: it never\n" +
			"fails the command.\n\n" +
			"Use `nvfleetint upgrade` to install a newer release.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			commonFlags := resolveCommonFlags(cmd, common)
			if err := validateReadCommonFlags(commonFlags); err != nil {
				return err
			}

			result, checkErr := latestReleaseCheck(cmd.Context())

			if commonFlags.output == clioutput.FormatJSON {
				if err := clioutput.WriteJSON(cmd.OutOrStdout(), versionOutput{
					Name:        "nvfleetint",
					Version:     version,
					Commit:      commit,
					BuildDate:   buildDate,
					UpdateCheck: result,
				}); err != nil {
					return err
				}
				writeReleaseCheckFailure(cmd.ErrOrStderr(), checkErr)
				return nil
			}

			writeVersion(cmd.OutOrStdout())
			if result != nil {
				// The notice goes to stderr so piping `version` stays clean.
				fmt.Fprint(cmd.ErrOrStderr(), updatecheck.Notice(*result, version))
			}
			writeReleaseCheckFailure(cmd.ErrOrStderr(), checkErr)
			return nil
		},
	}
	registerOutputFlag(cmd, common)
	return cmd
}

// latestReleaseCheck runs the release lookup. A failure is reported to stderr
// by the caller, but it never fails `nvfleetint version`: being offline or
// rate-limited by GitHub is not a command failure.
func latestReleaseCheck(ctx context.Context) (*updatecheck.Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	ctx, cancel := context.WithTimeout(ctx, updatecheck.DefaultTimeout)
	defer cancel()

	result, err := updatecheck.Check(ctx, updateChecker, version)
	if err != nil || result.Version == "" {
		return nil, err
	}
	return &result, nil
}

func writeReleaseCheckFailure(w io.Writer, err error) {
	if err != nil {
		fmt.Fprintln(w, "Can't check if nvfleetint is up to date. Check your internet connection and try again.")
	}
}

// Writes binary version details
func writeVersion(w io.Writer) {
	fmt.Fprintf(w, "nvfleetint %s\ncommit: %s\nbuilt: %s\n", version, commit, buildDate)
}
