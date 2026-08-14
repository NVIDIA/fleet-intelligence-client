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

// updateChecker is the release lookup used by `version`. Tests point it at an
// httptest server instead of GitHub.
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
	checkUpdate := true
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Long: "Print version information.\n\n" +
			"Unless --check-update=false or " + updatecheck.EnvDisable + " is set, this also asks\n" +
			"GitHub for the newest published release and prints an upgrade notice when the\n" +
			"running build is behind. The lookup is best effort: it never fails the command.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			commonFlags := resolveCommonFlags(cmd, common)
			if err := validateReadCommonFlags(commonFlags); err != nil {
				return err
			}

			result := latestReleaseCheck(cmd.Context(), checkUpdate)

			if commonFlags.output == clioutput.FormatJSON {
				return clioutput.WriteJSON(cmd.OutOrStdout(), versionOutput{
					Name:        "nvfleetint",
					Version:     version,
					Commit:      commit,
					BuildDate:   buildDate,
					UpdateCheck: result,
				})
			}

			writeVersion(cmd.OutOrStdout())
			if result != nil {
				// The notice goes to stderr so piping `version` stays clean.
				fmt.Fprint(cmd.ErrOrStderr(), updatecheck.Notice(*result, version))
			}
			return nil
		},
	}
	registerOutputFlag(cmd, common)
	cmd.Flags().BoolVar(&checkUpdate, "check-update", true,
		"Check GitHub for a newer release; set "+updatecheck.EnvDisable+"=1 to disable by default")
	return cmd
}

// latestReleaseCheck runs the release lookup, returning nil when the check is
// turned off, when this build has no comparable version, or when the lookup
// fails. A failure is deliberately silent: being offline or rate-limited by
// GitHub is not a reason for `nvfleetint version` to report an error.
func latestReleaseCheck(ctx context.Context, enabled bool) *updatecheck.Result {
	if !enabled || updatecheck.Disabled() {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	ctx, cancel := context.WithTimeout(ctx, updatecheck.DefaultTimeout)
	defer cancel()

	result, err := updatecheck.Check(ctx, updateChecker, version)
	if err != nil || result.Version == "" {
		return nil
	}
	return &result
}

// Writes binary version details
func writeVersion(w io.Writer) {
	fmt.Fprintf(w, "nvfleetint %s\ncommit: %s\nbuilt: %s\n", version, commit, buildDate)
}
