// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/NVIDIA/fleet-intelligence-client/internal/clihelpers"
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

// upgradeRunner performs the upgrade. Tests replace it, since the real one
// downloads and executes the release installer.
var upgradeRunner = func(ctx context.Context, plan updatecheck.UpgradePlan, progress io.Writer) error {
	return plan.Run(ctx, progress)
}

type versionOutput struct {
	Name        string              `json:"name"`
	Version     string              `json:"version"`
	Commit      string              `json:"commit"`
	BuildDate   string              `json:"buildDate"`
	UpdateCheck *updatecheck.Result `json:"updateCheck,omitempty"`
}

type upgradeOutput struct {
	Name       string `json:"name"`
	From       string `json:"from"`
	To         string `json:"to"`
	InstallDir string `json:"installDir,omitempty"`
	Upgraded   bool   `json:"upgraded"`
}

// versionFlags holds the flags specific to the version command.
type versionFlags struct {
	upgrade bool
	yes     bool
}

// Creates the version command
func newVersionCmd() *cobra.Command {
	common := newCommonFlags()
	flags := versionFlags{}
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Long: "Print version information.\n\n" +
			"This also asks GitHub for the newest published release and prints an upgrade\n" +
			"notice when the running build is behind. The lookup is best effort: it never\n" +
			"fails the command.\n\n" +
			"--upgrade installs that release over the running binary by running the\n" +
			"installer published with it, which verifies the release checksum and code\n" +
			"signature. It asks for confirmation unless --yes is given.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			commonFlags := resolveCommonFlags(cmd, common)
			if err := validateReadCommonFlags(commonFlags); err != nil {
				return err
			}
			if flags.yes && !flags.upgrade {
				return errors.New("--yes has no effect without --upgrade")
			}

			if flags.upgrade {
				return runUpgrade(cmd, commonFlags.output, flags.yes)
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
	cmd.Flags().BoolVar(&flags.upgrade, "upgrade", false,
		"Install the newest release over the running binary")
	cmd.Flags().BoolVar(&flags.yes, "yes", false, "Skip the confirmation prompt for --upgrade")
	return cmd
}

// runUpgrade installs the newest release over the running binary.
//
// Unlike the passive check, every failure here is reported: the user asked for
// an upgrade, so silence would leave them believing they had been upgraded.
func runUpgrade(cmd *cobra.Command, output string, yes bool) error {
	if !updatecheck.IsReleaseVersion(version) {
		return fmt.Errorf("cannot upgrade a locally built binary (version %q); "+
			"install a release first with: %s", version, updatecheck.ManualUpgradeCommand())
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	lookupCtx, cancel := context.WithTimeout(ctx, updatecheck.DefaultTimeout)
	defer cancel()

	result, err := updatecheck.Check(lookupCtx, updateChecker, version)
	if err != nil {
		return err
	}

	if !result.Available {
		return writeUpgradeResult(cmd, output, upgradeOutput{
			Name: "nvfleetint",
			From: version,
			To:   result.Version,
		}, fmt.Sprintf("nvfleetint %s is already the newest release.\n", version))
	}

	plan, err := updatecheck.NewUpgradePlan(version, result.Release)
	if err != nil {
		return err
	}
	if err := plan.CheckWritable(); err != nil {
		return err
	}

	// Prompts and installer output go to stderr so `-o json` stdout stays
	// parseable.
	progress := cmd.ErrOrStderr()
	if !yes {
		if err := clihelpers.Confirm(cmd.InOrStdin(), progress, plan.Summary()); err != nil {
			return err
		}
	}
	fmt.Fprintln(progress)

	if err := upgradeRunner(ctx, plan, progress); err != nil {
		return err
	}

	return writeUpgradeResult(cmd, output, upgradeOutput{
		Name:       "nvfleetint",
		From:       version,
		To:         result.Version,
		InstallDir: plan.InstallDir,
		Upgraded:   true,
	}, fmt.Sprintf("Upgraded %s -> %s\n", version, result.Version))
}

// writeUpgradeResult renders the outcome in the requested format.
func writeUpgradeResult(cmd *cobra.Command, output string, result upgradeOutput, message string) error {
	if output == clioutput.FormatJSON {
		return clioutput.WriteJSON(cmd.OutOrStdout(), result)
	}
	fmt.Fprint(cmd.OutOrStdout(), message)
	return nil
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
