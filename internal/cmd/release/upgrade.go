// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package release

import (
	"context"
	"fmt"
	"io"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdutil"
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

// NewUpgradeCmd creates the upgrade command.
func NewUpgradeCmd(build BuildInfo) *cobra.Command {
	common := cmdutil.NewCommon()
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
			commonFlags := cmdutil.ResolveCommon(cmd, common)
			if err := cmdutil.ValidateReadFlags(commonFlags); err != nil {
				return err
			}
			flags.versionSet = cmd.Flags().Changed("version")
			return runUpgrade(cmd, build, commonFlags.Output, flags)
		},
	}
	cmdutil.RegisterOutputFlag(cmd, common)
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
func runUpgrade(cmd *cobra.Command, build BuildInfo, output string, flags upgradeFlags) error {
	if !updatecheck.IsReleaseVersion(build.Version) {
		return fmt.Errorf("cannot upgrade a locally built binary (version %q); "+
			"install a release first with: %s", build.Version, updatecheck.ManualUpgradeCommand())
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	lookupCtx, cancel := context.WithTimeout(ctx, updatecheck.DefaultTimeout)
	defer cancel()

	target, installed, err := resolveUpgradeTarget(lookupCtx, build, flags)
	if err != nil {
		return err
	}
	if installed {
		return writeUpgradeResult(cmd, output, upgradeOutput{
			Name: "nvfleetint",
			From: build.Version,
			To:   target.Version,
		}, upgradeSkippedMessage(build, flags.versionSet, target.Version))
	}

	plan, err := updateChecker.NewUpgradePlan(build.Version, target)
	if err != nil {
		return err
	}
	// Both checks run before the prompt: a confirmation is only worth asking for
	// once the upgrade is known to be possible.
	if err := plan.CheckWritable(); err != nil {
		return err
	}
	if err := plan.CheckInstallerAvailable(ctx); err != nil {
		return err
	}

	// Prompts and installer output go to stderr so `-o json` stdout stays
	// parseable.
	progress := cmd.ErrOrStderr()
	if !flags.yes {
		if err := cmdutil.Confirm(cmd.InOrStdin(), progress, plan.Summary()); err != nil {
			return err
		}
	}
	fmt.Fprintln(progress)

	if err := upgradeRunner(ctx, plan, progress); err != nil {
		return err
	}

	return writeUpgradeResult(cmd, output, upgradeOutput{
		Name:       "nvfleetint",
		From:       build.Version,
		To:         target.Version,
		InstallDir: plan.InstallDir,
		Upgraded:   true,
	}, fmt.Sprintf("Upgraded %s -> %s\n", updatecheck.DisplayVersion(build.Version), target.Version))
}

// resolveUpgradeTarget resolves the release to install and reports whether the
// running build is already on it.
//
// Without --version that is the newest published release. With one it is that
// release, resolved against GitHub so a version that does not exist fails here
// rather than as a 404 inside the installer, halfway through. An older release
// is a legitimate target — pinning a known-good build is the reason to name one
// — so only the version already installed is treated as nothing to do.
func resolveUpgradeTarget(ctx context.Context, build BuildInfo, flags upgradeFlags) (updatecheck.Release, bool, error) {
	if !flags.versionSet {
		result, err := updatecheck.Check(ctx, updateChecker, build.Version)
		if err != nil {
			return updatecheck.Release{}, false, err
		}
		return result.Release, !result.Available, nil
	}

	release, err := updateChecker.Release(ctx, flags.version)
	if err != nil {
		return updatecheck.Release{}, false, err
	}
	installed, err := updatecheck.IsSame(release.Version, build.Version)
	if err != nil {
		return updatecheck.Release{}, false, err
	}
	return release, installed, nil
}

// upgradeSkippedMessage explains why nothing was installed. A named version
// that is already installed is not the same news as being on the newest
// release, so the two are worded apart.
func upgradeSkippedMessage(build BuildInfo, versionSet bool, target string) string {
	if !versionSet {
		return fmt.Sprintf("nvfleetint %s is already the newest release.\n", updatecheck.DisplayVersion(build.Version))
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
