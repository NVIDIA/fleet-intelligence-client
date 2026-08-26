// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package cmdutil

import (
	"fmt"

	"github.com/spf13/cobra"
)

// RequireSingleArg validates that exactly one positional argument was given,
// naming it in errors.
func RequireSingleArg(name string) cobra.PositionalArgs {
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

// OptionalSingleArg validates that at most one positional argument was given.
// The caller supplies the meaning of an omitted argument; only the too-many
// case is an error here, and it is worded exactly as in RequireSingleArg.
func OptionalSingleArg(name string) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) > 1 {
			return fmt.Errorf("only one %s may be given, got %d", name, len(args))
		}
		return nil
	}
}

// RejectUnknownSubcommands makes a resource group such as `node` reject an
// unknown subcommand instead of printing its help and exiting 0, so a typo'd
// command in a script fails loudly rather than reporting success having done
// nothing. Both fields are needed: cobra skips Args validation on a command
// that has no Run of its own.
//
// Only groups need this. Cobra applies the same check to the root command
// automatically (where it also suggests near-misses), and every leaf command
// declares its own Args.
func RejectUnknownSubcommands(cmd *cobra.Command) {
	cmd.Args = cobra.NoArgs
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return cmd.Help()
	}
}
