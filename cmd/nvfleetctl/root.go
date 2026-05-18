package main

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

func execute(ctx context.Context, args []string) error {
	cmd := newRootCmd()
	cmd.SetContext(ctx)
	cmd.SetArgs(args)

	return cmd.Execute()
}

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "nvfleetctl",
		Short:         "Fleet Intelligence CLI",
		Long:          "Fleet Intelligence CLI for the Fleet Intelligence customer API.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.AddCommand(newVersionCmd())

	return cmd
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		RunE: func(cmd *cobra.Command, _ []string) error {
			writeVersion(cmd.OutOrStdout())
			return nil
		},
	}
}

func writeVersion(w io.Writer) {
	fmt.Fprintf(w, "nvfleetctl %s\ncommit: %s\nbuilt: %s\n", version, commit, buildDate)
}
