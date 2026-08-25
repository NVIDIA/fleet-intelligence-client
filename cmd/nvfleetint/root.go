// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmd/alert"
	"github.com/NVIDIA/fleet-intelligence-client/internal/cmd/auth"
	"github.com/NVIDIA/fleet-intelligence-client/internal/cmd/computezone"
	"github.com/NVIDIA/fleet-intelligence-client/internal/cmd/event"
	"github.com/NVIDIA/fleet-intelligence-client/internal/cmd/node"
	"github.com/NVIDIA/fleet-intelligence-client/internal/cmd/nodegroup"
	"github.com/NVIDIA/fleet-intelligence-client/internal/cmd/overview"
	"github.com/NVIDIA/fleet-intelligence-client/internal/cmd/release"
	"github.com/NVIDIA/fleet-intelligence-client/internal/cmd/report"
	"github.com/NVIDIA/fleet-intelligence-client/internal/cmd/tag"
	"github.com/NVIDIA/fleet-intelligence-client/internal/cmd/xidburst"

	"github.com/spf13/cobra"
)

// Runs the CLI with the provided context and arguments
func execute(ctx context.Context, build release.BuildInfo, args []string) error {
	cmd := newRootCmd(build)
	cmd.SetContext(ctx)
	cmd.SetArgs(args)

	return cmd.Execute()
}

// Creates the root nvfleetint command. Each top-level command is a package of
// its own, so this file is only the assembly order.
func newRootCmd(build release.BuildInfo) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "nvfleetint",
		Short:         "Fleet Intelligence CLI",
		Long:          "Fleet Intelligence CLI for the Fleet Intelligence customer API.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.AddCommand(auth.NewCmd())
	cmd.AddCommand(overview.NewCmd())
	cmd.AddCommand(computezone.NewCmd())
	cmd.AddCommand(nodegroup.NewCmd())
	cmd.AddCommand(node.NewCmd())
	cmd.AddCommand(alert.NewCmd())
	cmd.AddCommand(event.NewCmd())
	cmd.AddCommand(xidburst.NewCmd())
	cmd.AddCommand(report.NewCmd())
	cmd.AddCommand(tag.NewCmd())
	cmd.AddCommand(release.NewVersionCmd(build))
	cmd.AddCommand(release.NewUpgradeCmd(build))

	return cmd
}
