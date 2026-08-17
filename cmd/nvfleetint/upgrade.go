// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"io"

	"github.com/NVIDIA/fleet-intelligence-client/internal/clihelpers"
	clioutput "github.com/NVIDIA/fleet-intelligence-client/internal/output"
	"github.com/NVIDIA/fleet-intelligence-client/internal/updatecheck"

	"github.com/spf13/cobra"
)

// upgradeRunner performs the upgrade. Tests replace it, since the real one
// downloads and executes the release installer.
var upgradeRunner = func(ctx context.Context, plan updatecheck.UpgradePlan, progress io.Writer) error {
	return plan.Run(ctx, progress)
}

type upgradeOutput struct {
	Name       string `json:"name"`
	From       string `json:"from"`
	To         string `json:"to"`
	InstallDir string `json:"installDir,omitempty"`
	Upgraded   bool   `json:"upgraded"`
}

// upgradeFlags holds the flags specific to the upgrade command.
type upgradeFlags struct {
	version string
	// versionSet distinguishes an omitted --version, which means the newest
	// release, from an empty one, which is a mistake worth reporting.
	versionSet bool
	yes        bool
}

// Creates the upgrade command
func newUpgradeCmd() *cobra.Command {
	common := newCommonFlags()
	flags := upgradeFlags{}
	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Install a published release over the running binary",
		Long: "Install a published release over the running binary.\n\n" +
			"Without --version this installs the newest published release, and does\n" +
			"nothing when the running build is already on it. With --version it installs\n" +
			"exactly that release, including an older one; a version that was never\n" +
			"published is refused before anything is downloaded.\n\n" +
			"The upgrade runs the installer published with the target release, which\n" +
			"verifies the release checksum and code signature. It asks for confirmation\n" +
			"unless --yes is given.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			commonFlags := resolveCommonFlags(cmd, common)
			if err := validateReadCommonFlags(commonFlags); err != nil {
				return err
			}
			flags.versionSet = cmd.Flags().Changed("version")
			return runUpgrade(cmd, commonFlags.output, flags)
		},
	}
	registerOutputFlag(cmd, common)
	cmd.Flags().StringVar(&flags.version, "version", "",
		"Release to install, e.g. v1.2.0; defaults to the newest published release")
	cmd.Flags().BoolVar(&flags.yes, "yes", false, "Skip the confirmation prompt")
	return cmd
}

// runUpgrade installs the target release over the running binary.
//
// Unlike the passive check in `version`, every failure here is reported: the
// user asked for an upgrade, so silence would leave them believing they had
// been upgraded.
func runUpgrade(cmd *cobra.Command, output string, flags upgradeFlags) error {
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

	target, installed, err := resolveUpgradeTarget(lookupCtx, flags)
	if err != nil {
		return err
	}
	if installed {
		return writeUpgradeResult(cmd, output, upgradeOutput{
			Name: "nvfleetint",
			From: version,
			To:   target.Version,
		}, upgradeSkippedMessage(flags.versionSet, target.Version))
	}

	plan, err := updatecheck.NewUpgradePlan(version, target)
	if err != nil {
		return err
	}
	if err := plan.CheckWritable(); err != nil {
		return err
	}

	// Prompts and installer output go to stderr so `-o json` stdout stays
	// parseable.
	progress := cmd.ErrOrStderr()
	if !flags.yes {
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
		To:         target.Version,
		InstallDir: plan.InstallDir,
		Upgraded:   true,
	}, fmt.Sprintf("Upgraded %s -> %s\n", updatecheck.DisplayVersion(version), target.Version))
}

// resolveUpgradeTarget resolves the release to install and reports whether the
// running build is already on it.
//
// Without --version that is the newest published release. With one it is that
// release, resolved against GitHub so a version that does not exist fails here
// rather than as a 404 inside the installer, halfway through. An older release
// is a legitimate target — pinning a known-good build is the reason to name one
// — so only the version already installed is treated as nothing to do.
func resolveUpgradeTarget(ctx context.Context, flags upgradeFlags) (updatecheck.Release, bool, error) {
	if !flags.versionSet {
		result, err := updatecheck.Check(ctx, updateChecker, version)
		if err != nil {
			return updatecheck.Release{}, false, err
		}
		return result.Release, !result.Available, nil
	}

	release, err := updateChecker.Release(ctx, flags.version)
	if err != nil {
		return updatecheck.Release{}, false, err
	}
	installed, err := updatecheck.IsSame(release.Version, version)
	if err != nil {
		return updatecheck.Release{}, false, err
	}
	return release, installed, nil
}

// upgradeSkippedMessage explains why nothing was installed. A named version
// that is already installed is not the same news as being on the newest
// release, so the two are worded apart.
func upgradeSkippedMessage(versionSet bool, target string) string {
	if !versionSet {
		return fmt.Sprintf("nvfleetint %s is already the newest release.\n", updatecheck.DisplayVersion(version))
	}
	return fmt.Sprintf("nvfleetint %s is already installed.\n", target)
}

// writeUpgradeResult renders the outcome in the requested format.
func writeUpgradeResult(cmd *cobra.Command, output string, result upgradeOutput, message string) error {
	if output == clioutput.FormatJSON {
		return clioutput.WriteJSON(cmd.OutOrStdout(), result)
	}
	fmt.Fprint(cmd.OutOrStdout(), message)
	return nil
}
