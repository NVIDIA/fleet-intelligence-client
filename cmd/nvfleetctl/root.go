package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	clihelpers "gitlab-master.nvidia.com/gpu-health/fleet-intelligence-client-go/cmd/nvfleetctl/helpers"
	clioutput "gitlab-master.nvidia.com/gpu-health/fleet-intelligence-client-go/internal/output"
	"gitlab-master.nvidia.com/gpu-health/fleet-intelligence-client-go/pkg/fleetintelligence"

	"github.com/spf13/cobra"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

// Stores shared output and pagination flag values
type commonFlags struct {
	output   string
	all      bool
	page     int
	pageSize int
	timeout  time.Duration
}

// Stores common flag values plus explicit-set state
type resolvedCommonFlags struct {
	output      string
	all         bool
	page        int
	pageSize    int
	timeout     time.Duration
	outputSet   bool
	allSet      bool
	pageSet     bool
	pageSizeSet bool
	timeoutSet  bool
}

// Runs the CLI with the provided context and arguments
func execute(ctx context.Context, args []string) error {
	cmd := newRootCmd()
	cmd.SetContext(ctx)
	cmd.SetArgs(args)

	return cmd.Execute()
}

// Creates the root nvfleetctl command
func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "nvfleetctl",
		Short:         "Fleet Intelligence CLI",
		Long:          "Fleet Intelligence CLI for the Fleet Intelligence customer API.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.AddCommand(newAuthCmd())
	cmd.AddCommand(newComputeZoneCmd())
	cmd.AddCommand(newNodeGroupCmd())
	cmd.AddCommand(newNodeCmd())
	cmd.AddCommand(newAlertCmd())
	cmd.AddCommand(newReportCmd())
	cmd.AddCommand(newVersionCmd())

	return cmd
}

// Creates common flags with default values
func newCommonFlags() *commonFlags {
	return &commonFlags{
		output:  clioutput.FormatTable,
		timeout: fleetintelligence.DefaultTimeout,
	}
}

// Registers the output flag on a command
func registerOutputFlag(cmd *cobra.Command, flags *commonFlags) {
	cmd.Flags().StringVarP(&flags.output, "output", "o", flags.output, "Output format: table or json")
}

// Registers pagination flags on a command
func registerPaginationFlags(cmd *cobra.Command, flags *commonFlags) {
	cmd.Flags().BoolVar(&flags.all, "all", false, "Fetch all pages")
	cmd.Flags().IntVar(&flags.page, "page", 0, "Page number to fetch")
	cmd.Flags().IntVar(&flags.pageSize, "page-size", 0, "Page size to fetch")
}

// Registers the request timeout flag on a command
func registerTimeoutFlag(cmd *cobra.Command, flags *commonFlags) {
	cmd.Flags().DurationVar(&flags.timeout, "timeout", flags.timeout, "Request timeout (e.g. 30s, 2m); must be greater than 0")
}

// Registers output and pagination flags on a list command
func registerListCommonFlags(cmd *cobra.Command, flags *commonFlags) {
	registerOutputFlag(cmd, flags)
	registerPaginationFlags(cmd, flags)
	registerTimeoutFlag(cmd, flags)
}

// Registers output flags on a read command
func registerReadCommonFlags(cmd *cobra.Command, flags *commonFlags) {
	registerOutputFlag(cmd, flags)
	registerTimeoutFlag(cmd, flags)
}

// Returns common flag values and whether pagination flags were supplied
func resolveCommonFlags(cmd *cobra.Command, flags *commonFlags) resolvedCommonFlags {
	return resolvedCommonFlags{
		output:      flags.output,
		all:         flags.all,
		page:        flags.page,
		pageSize:    flags.pageSize,
		timeout:     flags.timeout,
		outputSet:   cmd.Flags().Changed("output"),
		allSet:      cmd.Flags().Changed("all"),
		pageSet:     cmd.Flags().Changed("page"),
		pageSizeSet: cmd.Flags().Changed("page-size"),
		timeoutSet:  cmd.Flags().Changed("timeout"),
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
	if flags.pageSet && flags.page < 0 {
		return errors.New("--page must be greater than or equal to 0")
	}
	if flags.pageSizeSet && (flags.pageSize < clihelpers.MinPageSize || flags.pageSize > clihelpers.MaxPageSize) {
		return fmt.Errorf("--page-size must be between %d and %d", clihelpers.MinPageSize, clihelpers.MaxPageSize)
	}
	if flags.timeout <= 0 {
		return errors.New("--timeout must be greater than 0")
	}
	return nil
}

// Checks common flags for non-paginated read commands
func validateReadCommonFlags(flags resolvedCommonFlags) error {
	if !clioutput.IsValidFormat(flags.output) {
		return fmt.Errorf("invalid output %q: expected table or json", flags.output)
	}
	if flags.timeout <= 0 {
		return errors.New("--timeout must be greater than 0")
	}
	return nil
}

// Applies explicitly supplied pagination flags to request options
func applyPagination(flags resolvedCommonFlags, setPage func(*int), setPageSize func(*int)) {
	if flags.pageSet {
		page := flags.page
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

// Creates the version command
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

// Writes binary version details
func writeVersion(w io.Writer) {
	fmt.Fprintf(w, "nvfleetctl %s\ncommit: %s\nbuilt: %s\n", version, commit, buildDate)
}
