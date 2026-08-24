// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/NVIDIA/fleet-intelligence-client/internal/clihelpers"
	clioutput "github.com/NVIDIA/fleet-intelligence-client/internal/output"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"

	"github.com/spf13/cobra"
)

const reportDurationUnitsMessage = "expected a positive duration using units ns, us, µs, ms, s, m, or h"

const reportInventorySortByList = "hostname, nodeUUID, nodegroup, computezone, gpuType, gpuCount, publicIP, privateIP, integrityCheck, or geoLocation"

var (
	maxReportWindow       = time.Duration(1<<63 - 1)
	reportDurationPattern = regexp.MustCompile(`^\+?(?:(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:ns|us|µs|ms|s|m|h))+$`)
)

// Stores local flag values for inventory reports
type reportInventoryFlags struct {
	format         string
	signed         bool
	outputPath     string
	computeZoneIDs string
	nodeGroupIDs   string
	tags           string
	start          string
	end            string
	sortBy         string
	order          string
}

// Stores local flag values for error reports
type reportErrorFlags struct {
	view           string
	groupBy        string
	format         string
	computeZoneIDs string
	nodeGroupIDs   string
	tags           string
	errors         string
	severities     string
	step           string
	window         string
	start          string
	end            string
}

// Stores local flag values for report verification
type reportVerifyFlags struct {
	csv    string
	bundle string
	key    string
}

type commandStatusOutput struct {
	Status string `json:"status"`
}

type signedReportOutput struct {
	Status string `json:"status"`
	Path   string `json:"path"`
}

// Stores data ready for inventory report rendering
type reportInventoryOutput struct {
	Report    nvfleetint.InventoryReport
	JSONValue any
	RawJSON   []byte
	Page      *clioutput.Pagination
}

// Stores data ready for error report rendering
type reportErrorOutput struct {
	Report    nvfleetint.ErrorReport
	JSONValue any
	RawJSON   []byte
	Page      *clioutput.Pagination
}

// Creates the top-level report command group
func newReportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Generate fleet reports",
	}

	cmd.AddCommand(newReportInventoryCmd())
	cmd.AddCommand(newReportErrorCmd())
	cmd.AddCommand(newReportVerifyCmd())
	rejectUnknownSubcommands(cmd)

	return cmd
}

// Creates the inventory report command
func newReportInventoryCmd() *cobra.Command {
	flags := reportInventoryFlags{
		format: string(nvfleetint.ReportFormatJSON),
	}
	common := newCommonFlags()

	cmd := &cobra.Command{
		Use:   "inventory",
		Short: "Generate an inventory report",
		Args:  cobra.NoArgs,
		Long: `Generate an inventory report for the fleet.

Use --signed with --format csv to download a signed CSV bundle (a zip
containing the CSV plus a cosign-verifiable signature). The bundle is
written to the current directory unless --output-path is provided.`,
		Example: `  # Inventory report as a table (default)
  nvfleetint report inventory

  # Download a signed CSV bundle into the current directory
  nvfleetint report inventory --format csv --signed

  # Download a signed CSV bundle to a specific path
  nvfleetint report inventory --format csv --signed --output-path ./reports/inventory.zip`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runReportInventory(cmd, flags, resolveCommonFlags(cmd, common))
		},
	}

	cmd.Flags().StringVar(&flags.format, "format", flags.format, "Report format: json or csv")
	cmd.Flags().BoolVar(&flags.signed, "signed", false, "Download a signed CSV bundle (zip of CSV plus signature); requires --format csv")
	cmd.Flags().StringVar(&flags.outputPath, "output-path", "", "Destination file or directory for the signed bundle; defaults to the current directory")
	cmd.Flags().StringVar(&flags.computeZoneIDs, "compute-zone-ids", "", "Comma-separated compute zone IDs to filter")
	cmd.Flags().StringVar(&flags.nodeGroupIDs, "nodegroup-ids", "", "Comma-separated node group IDs to filter")
	cmd.Flags().StringVar(&flags.tags, "tags", "", "Comma-separated tags to filter")
	cmd.Flags().StringVar(&flags.start, "start", "", "Absolute start time in RFC3339 format")
	cmd.Flags().StringVar(&flags.end, "end", "", "Absolute end time in RFC3339 format")
	cmd.Flags().StringVar(&flags.sortBy, "sort-by", "", "Sort field: "+reportInventorySortByList)
	cmd.Flags().StringVar(&flags.order, "order", "", "Sort order: asc or desc")
	registerListCommonFlags(cmd, common)

	return cmd
}

// Creates the error report command
func newReportErrorCmd() *cobra.Command {
	flags := reportErrorFlags{
		view:   string(nvfleetint.ErrorReportViewOverview),
		format: string(nvfleetint.ReportFormatJSON),
	}
	common := newCommonFlags()

	cmd := &cobra.Command{
		Use:   "error",
		Short: "Generate an error report",
		Args:  cobra.NoArgs,
		Long: `Generate an error report for the fleet.

Views:
  overview  Summary totals for the time range (default).
  list      Per-error or per-node breakdown; requires --group-by.
  graph     Time series of error counts.

A time range is always required: use --window for a relative range, or
--start and --end for an absolute range.`,
		Example: `  # Summary totals over the last 24 hours (default view)
  nvfleetint report error --window 24h

  # Errors grouped by type over the last 7 days
  nvfleetint report error --view list --group-by error --window 168h

  # Errors grouped by node for an absolute range
  nvfleetint report error --view list --group-by node --start 2026-05-01T00:00:00Z --end 2026-05-08T00:00:00Z

  # Error count time series
  nvfleetint report error --view graph --window 24h`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runReportError(cmd, flags, resolveCommonFlags(cmd, common))
		},
	}

	cmd.Flags().StringVar(&flags.view, "view", flags.view, "Report view: list, graph, or overview")
	cmd.Flags().StringVar(&flags.groupBy, "group-by", "", "Group list view by: error or node")
	cmd.Flags().StringVar(&flags.format, "format", flags.format, "Report format: json or csv")
	cmd.Flags().StringVar(&flags.computeZoneIDs, "compute-zone-ids", "", "Comma-separated compute zone IDs to filter")
	cmd.Flags().StringVar(&flags.nodeGroupIDs, "nodegroup-ids", "", "Comma-separated node group IDs to filter")
	cmd.Flags().StringVar(&flags.tags, "tags", "", "Comma-separated tags to filter")
	cmd.Flags().StringVar(&flags.errors, "errors", "", "Comma-separated error types to filter")
	cmd.Flags().StringVar(&flags.severities, "severities", "", "Comma-separated severities to filter: Critical, Fatal, Info, or Warning")
	cmd.Flags().StringVar(&flags.step, "step", "", "Graph query step width; minimum 1m")
	cmd.Flags().StringVar(&flags.window, "window", "", "Relative time window; valid units: ns, us, µs, ms, s, m, h")
	cmd.Flags().StringVar(&flags.start, "start", "", "Absolute start time in RFC3339 format")
	cmd.Flags().StringVar(&flags.end, "end", "", "Absolute end time in RFC3339 format")
	registerListCommonFlags(cmd, common)

	return cmd
}

// Creates the report verify command
func newReportVerifyCmd() *cobra.Command {
	flags := reportVerifyFlags{}
	common := newCommonFlags()

	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify a signed inventory report",
		Args:  cobra.NoArgs,
		Long: `Verify a signed inventory report downloaded with
"report inventory --format csv --signed".

That command downloads a zip (inventory-report.zip by default). Unzip it
first; it expands to a folder named inventory_report_<timestamp>/ containing
two files that share the same stem:

  inventory_report_<timestamp>.csv         the report
  inventory_report_<timestamp>.sig.bundle  its Sigstore signature

Pass the .csv to --csv and the .sig.bundle to --bundle.

Verification is built in; no external tools are required. By default the
signing key is fetched from the configured API. Pass --key to verify fully
offline with a previously downloaded public key.`,
		Example: `  # Unzip the downloaded bundle, then verify (key fetched from the API)
  unzip inventory-report.zip
  nvfleetint report verify \
    --csv <report>.csv \
    --bundle <report>.sig.bundle

  # Verify offline with a previously downloaded public key
  nvfleetint report verify \
    --csv <report>.csv \
    --bundle <report>.sig.bundle \
    --key signing-key.pub`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runReportVerify(cmd, flags, resolveCommonFlags(cmd, common))
		},
	}

	cmd.Flags().StringVar(&flags.csv, "csv", "", "Path to the report CSV file to verify")
	cmd.Flags().StringVar(&flags.bundle, "bundle", "", "Path to the .sig.bundle signature file")
	cmd.Flags().StringVar(&flags.key, "key", "", "Path to a PEM public key for offline verification; defaults to the key fetched from the API")
	registerOutputFlag(cmd, common)
	registerTimeoutFlag(cmd, common)
	// This command registers its flags by hand rather than through
	// registerReadCommonFlags, so --profile has to be added explicitly.
	registerProfileFlag(cmd, common)

	return cmd
}

// Validates flags, loads the artifacts and key, and verifies the signature
func runReportVerify(cmd *cobra.Command, flags reportVerifyFlags, common resolvedCommonFlags) error {
	if err := validateReportVerifyFlags(flags, common); err != nil {
		return err
	}

	csv, err := readVerifyFile("--csv", flags.csv)
	if err != nil {
		return err
	}
	bundle, err := readVerifyFile("--bundle", flags.bundle)
	if err != nil {
		return err
	}

	key, err := reportVerifyKey(cmd, flags, common)
	if err != nil {
		return err
	}

	if err := nvfleetint.VerifySignedReport(csv, bundle, key); err != nil {
		return reportVerifyError(flags, err)
	}

	if common.output == clioutput.FormatJSON {
		return clioutput.WriteJSON(cmd.OutOrStdout(), commandStatusOutput{Status: "verified"})
	}
	fmt.Fprintln(cmd.OutOrStdout(), "Verified OK")
	return nil
}

// Reads a file named by a verify flag, reporting which flag and path failed and
// making clear that the flag expects a file path.
func readVerifyFile(flag, path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%s file %q does not exist (%s expects a path to a file)", flag, path, flag)
		}
		return nil, fmt.Errorf("cannot read %s file %q: %w", flag, path, err)
	}
	return data, nil
}

// Translates verification errors into clear, actionable messages that name the
// offending file instead of leaking the underlying sigstore/proto details.
func reportVerifyError(flags reportVerifyFlags, err error) error {
	switch {
	case errors.Is(err, nvfleetint.ErrInvalidBundle):
		return fmt.Errorf("%q is not a valid signature bundle; pass the report's .sig.bundle file to --bundle", flags.bundle)
	case errors.Is(err, nvfleetint.ErrInvalidKey):
		if flags.key == "" {
			return errors.New("the signing key fetched from the API is not a valid PEM public key")
		}
		return fmt.Errorf("%q is not a valid PEM public key; pass the signing key (e.g. signing-key.pub) to --key", flags.key)
	case errors.Is(err, nvfleetint.ErrVerificationFailed):
		return fmt.Errorf("verification failed: %q does not match the signature in %q\n"+
			"The report may have been modified, or --csv and --bundle may point to the wrong files", flags.csv, flags.bundle)
	default:
		return err
	}
}

// Loads the verification key from --key or fetches it from the configured API
func reportVerifyKey(cmd *cobra.Command, flags reportVerifyFlags, common resolvedCommonFlags) ([]byte, error) {
	if flags.key != "" {
		return readVerifyFile("--key", flags.key)
	}

	client, err := newConfiguredClient(common)
	if err != nil {
		return nil, err
	}
	return client.FetchSigningKey(cmd.Context())
}

// Checks report verify flags
func validateReportVerifyFlags(flags reportVerifyFlags, common resolvedCommonFlags) error {
	if err := validateReadCommonFlags(common); err != nil {
		return err
	}
	if strings.TrimSpace(flags.csv) == "" {
		return errors.New("--csv is required")
	}
	if strings.TrimSpace(flags.bundle) == "" {
		return errors.New("--bundle is required")
	}
	return nil
}

// Validates flags, calls the SDK, and writes inventory report output
func runReportInventory(cmd *cobra.Command, flags reportInventoryFlags, common resolvedCommonFlags) error {
	if err := validateReportInventoryFlags(flags, common); err != nil {
		return err
	}

	computeZoneIDs, err := clihelpers.ParseCommaList(flags.computeZoneIDs)
	if err != nil {
		return err
	}
	nodeGroupIDs, err := clihelpers.ParseCommaList(flags.nodeGroupIDs)
	if err != nil {
		return err
	}
	tags, err := clihelpers.ParseCommaList(flags.tags)
	if err != nil {
		return err
	}

	client, err := newConfiguredClient(common)
	if err != nil {
		return err
	}

	opts := nvfleetint.InventoryReportOptions{
		Format:         nvfleetint.ReportFormat(flags.format),
		Signed:         flags.signed,
		ComputeZoneIDs: computeZoneIDs,
		NodeGroupIDs:   nodeGroupIDs,
		Tags:           tags,
		StartTime:      strings.TrimSpace(flags.start),
		EndTime:        strings.TrimSpace(flags.end),
		SortBy:         nvfleetint.InventoryReportSortBy(strings.TrimSpace(flags.sortBy)),
		Order:          nvfleetint.InventoryReportSortOrder(strings.TrimSpace(flags.order)),
	}
	applyPagination(common, func(page *int) { opts.Page = page }, func(pageSize *int) { opts.PageSize = pageSize })

	if flags.signed {
		report, err := client.GetInventoryReport(cmd.Context(), opts)
		if err != nil {
			return err
		}
		path, err := writeSignedReport(flags.outputPath, report.Filename, report.RawSigned)
		if err != nil {
			return err
		}
		if common.output == clioutput.FormatJSON {
			return clioutput.WriteJSON(cmd.OutOrStdout(), signedReportOutput{
				Status: "written",
				Path:   path,
			})
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Signed report written to %s\n", path)
		return nil
	}

	if nvfleetint.ReportFormat(flags.format) == nvfleetint.ReportFormatCSV {
		report, err := client.GetInventoryReport(cmd.Context(), opts)
		if err != nil {
			return err
		}
		return writeRawReportBytes(cmd.OutOrStdout(), report.RawCSV)
	}

	if common.all {
		var nodes []nvfleetint.InventoryNode
		result, err := clihelpers.FetchAllPages("nodes",
			func(pageNumber int) (nvfleetint.InventoryReport, error) {
				opts.Page = &pageNumber
				return client.GetInventoryReport(cmd.Context(), opts)
			},
			func(page nvfleetint.InventoryReport) { nodes = append(nodes, page.Nodes...) },
		)
		if err != nil {
			return err
		}
		return writeReportInventoryOutput(cmd.OutOrStdout(), common, reportInventoryOutput{
			Report: nvfleetint.InventoryReport{
				Nodes: nodes,
			},
			JSONValue: result,
		})
	}

	report, err := client.GetInventoryReport(cmd.Context(), opts)
	if err != nil {
		return err
	}
	return writeReportInventoryOutput(cmd.OutOrStdout(), common, reportInventoryOutput{
		Report:  report,
		RawJSON: report.RawJSON,
		Page: &clioutput.Pagination{
			Page:     report.Page,
			PageSize: report.PageSize,
			Total:    report.Total,
		},
	})
}

// Validates flags, calls the SDK, and writes error report output
func runReportError(cmd *cobra.Command, flags reportErrorFlags, common resolvedCommonFlags) error {
	if err := validateReportErrorFlags(flags, common); err != nil {
		return err
	}

	client, err := newConfiguredClient(common)
	if err != nil {
		return err
	}

	opts := errorReportOptions(flags)
	applyPagination(common, func(page *int) { opts.Page = page }, func(pageSize *int) { opts.PageSize = pageSize })

	if nvfleetint.ReportFormat(flags.format) == nvfleetint.ReportFormatCSV {
		report, err := client.GetErrorReport(cmd.Context(), opts)
		if err != nil {
			return err
		}
		return writeRawReportBytes(cmd.OutOrStdout(), report.RawCSV)
	}

	if common.all {
		return runReportErrorAll(cmd, client, opts, flags, common)
	}

	report, err := client.GetErrorReport(cmd.Context(), opts)
	if err != nil {
		return err
	}
	return writeReportErrorOutput(cmd.OutOrStdout(), common, reportErrorOutput{
		Report:  report,
		RawJSON: report.RawJSON,
		Page:    errorReportPagination(report),
	})
}

// Fetches all pages for error list reports and writes merged output
func runReportErrorAll(cmd *cobra.Command, client *nvfleetint.Client, opts nvfleetint.ErrorReportOptions, flags reportErrorFlags, common resolvedCommonFlags) error {
	var errorsByType []nvfleetint.ErrorReportError
	var nodes []nvfleetint.ErrorReportNode
	itemKey := errorReportItemKey(nvfleetint.ErrorReportGroupBy(flags.groupBy))

	// Only the slice matching itemKey is ever populated for a given group-by,
	// so both are collected and the empty one falls away.
	result, err := clihelpers.FetchAllPages(itemKey,
		func(pageNumber int) (nvfleetint.ErrorReport, error) {
			opts.Page = &pageNumber
			return client.GetErrorReport(cmd.Context(), opts)
		},
		func(page nvfleetint.ErrorReport) {
			errorsByType = append(errorsByType, page.Errors...)
			nodes = append(nodes, page.Nodes...)
		},
	)
	if err != nil {
		return err
	}

	return writeReportErrorOutput(cmd.OutOrStdout(), common, reportErrorOutput{
		Report: nvfleetint.ErrorReport{
			View:    nvfleetint.ErrorReportView(flags.view),
			GroupBy: nvfleetint.ErrorReportGroupBy(flags.groupBy),
			Errors:  errorsByType,
			Nodes:   nodes,
		},
		JSONValue: result,
	})
}

// Checks inventory report flags
func validateReportInventoryFlags(flags reportInventoryFlags, common resolvedCommonFlags) error {
	if err := validateListCommonFlags(common); err != nil {
		return err
	}
	if !nvfleetint.ReportFormat(flags.format).Valid() {
		return fmt.Errorf("invalid format %q: expected json or csv", flags.format)
	}
	if flags.signed && nvfleetint.ReportFormat(flags.format) != nvfleetint.ReportFormatCSV {
		return errors.New("--signed requires --format csv")
	}
	if flags.outputPath != "" && !flags.signed {
		return errors.New("--output-path can only be used with --signed")
	}
	for _, value := range []struct {
		name string
		raw  string
	}{
		{name: "compute-zone-ids", raw: flags.computeZoneIDs},
		{name: "nodegroup-ids", raw: flags.nodeGroupIDs},
		{name: "tags", raw: flags.tags},
	} {
		if _, err := clihelpers.ParseCommaList(value.raw); err != nil {
			return fmt.Errorf("invalid %s: %w", value.name, err)
		}
	}
	if err := validateReportInventoryTimeFlags(flags); err != nil {
		return err
	}
	if strings.TrimSpace(flags.sortBy) != "" && !nvfleetint.InventoryReportSortBy(strings.TrimSpace(flags.sortBy)).Valid() {
		return fmt.Errorf("invalid sort-by %q: expected %s", flags.sortBy, reportInventorySortByList)
	}
	if strings.TrimSpace(flags.order) != "" && !nvfleetint.InventoryReportSortOrder(strings.TrimSpace(flags.order)).Valid() {
		return fmt.Errorf("invalid order %q: expected asc or desc", flags.order)
	}
	if nvfleetint.ReportFormat(flags.format) == nvfleetint.ReportFormatCSV && !flags.signed {
		if common.outputSet {
			return errors.New("--output cannot be used with --format csv; use --signed to download a signed bundle (returns JSON status), or omit --format csv to get a JSON inventory report")
		}
		if common.allSet || common.pageSet || common.pageSizeSet {
			return errors.New("pagination flags cannot be used with --format csv")
		}
	}
	return nil
}

// Checks inventory report absolute time flags
func validateReportInventoryTimeFlags(flags reportInventoryFlags) error {
	hasStart := strings.TrimSpace(flags.start) != ""
	hasEnd := strings.TrimSpace(flags.end) != ""
	if hasStart != hasEnd {
		return errors.New("--start and --end must be used together")
	}
	if hasStart {
		if err := validateRFC3339Flag("--start", flags.start); err != nil {
			return err
		}
		if err := validateRFC3339Flag("--end", flags.end); err != nil {
			return err
		}
	}
	return nil
}

// Checks error report flags
func validateReportErrorFlags(flags reportErrorFlags, common resolvedCommonFlags) error {
	if err := validateListCommonFlags(common); err != nil {
		return err
	}

	view := nvfleetint.ErrorReportView(flags.view)
	groupBy := nvfleetint.ErrorReportGroupBy(flags.groupBy)
	format := nvfleetint.ReportFormat(flags.format)

	if strings.TrimSpace(flags.view) == "" {
		return errors.New("--view is required")
	}
	if !view.Valid() {
		return fmt.Errorf("invalid view %q: expected list, graph, or overview", flags.view)
	}
	if !format.Valid() {
		return fmt.Errorf("invalid format %q: expected json or csv", flags.format)
	}
	if strings.TrimSpace(flags.groupBy) != "" && !groupBy.Valid() {
		return fmt.Errorf("invalid group-by %q: expected error or node", flags.groupBy)
	}
	for _, value := range []struct {
		name string
		raw  string
	}{
		{name: "compute-zone-ids", raw: flags.computeZoneIDs},
		{name: "nodegroup-ids", raw: flags.nodeGroupIDs},
		{name: "tags", raw: flags.tags},
		{name: "errors", raw: flags.errors},
	} {
		if _, err := clihelpers.ParseCommaList(value.raw); err != nil {
			return fmt.Errorf("invalid %s: %w", value.name, err)
		}
	}
	if _, err := parseErrorSeverityList(flags.severities); err != nil {
		return err
	}
	if strings.TrimSpace(flags.step) != "" {
		if view != nvfleetint.ErrorReportViewGraph {
			return errors.New("--step can only be used with --view graph")
		}
		if err := validateReportStepFlag(flags.step); err != nil {
			return err
		}
	}
	if strings.TrimSpace(flags.errors) != "" && (view != nvfleetint.ErrorReportViewList || groupBy != nvfleetint.ErrorReportGroupByNode) {
		return errors.New("--errors can only be used with --view list --group-by node")
	}

	if err := validateReportErrorViewFlags(view, groupBy, format, flags, common); err != nil {
		return err
	}
	return validateReportTimeFlags(flags)
}

// Checks view-specific error report flags
func validateReportErrorViewFlags(view nvfleetint.ErrorReportView, groupBy nvfleetint.ErrorReportGroupBy, format nvfleetint.ReportFormat, flags reportErrorFlags, common resolvedCommonFlags) error {
	if format == nvfleetint.ReportFormatCSV {
		if common.outputSet {
			return errors.New("--output cannot be used with --format csv")
		}
		if common.allSet || common.pageSet || common.pageSizeSet {
			return errors.New("pagination flags cannot be used with --format csv")
		}
		if view != nvfleetint.ErrorReportViewList {
			return errors.New("--format csv is only supported with --view list")
		}
	}

	switch view {
	case nvfleetint.ErrorReportViewList:
		if strings.TrimSpace(flags.groupBy) == "" {
			return errors.New("--group-by is required for --view list")
		}
	case nvfleetint.ErrorReportViewGraph:
		if common.allSet || common.pageSet || common.pageSizeSet {
			return errors.New("pagination flags cannot be used with --view graph")
		}
		if strings.TrimSpace(flags.groupBy) != "" && groupBy != nvfleetint.ErrorReportGroupByError {
			return errors.New("--view graph only supports --group-by error")
		}
	case nvfleetint.ErrorReportViewOverview:
		if common.allSet || common.pageSet || common.pageSizeSet {
			return errors.New("pagination flags cannot be used with --view overview")
		}
		if strings.TrimSpace(flags.groupBy) != "" {
			return errors.New("--group-by cannot be used with --view overview")
		}
	}

	return nil
}

// Checks relative and absolute error report time flags
func validateReportTimeFlags(flags reportErrorFlags) error {
	hasWindow := strings.TrimSpace(flags.window) != ""
	hasStart := strings.TrimSpace(flags.start) != ""
	hasEnd := strings.TrimSpace(flags.end) != ""

	if !hasWindow && !hasStart && !hasEnd {
		return errors.New("a time range is required: use --window for a relative range, or --start and --end for an absolute range")
	}
	if hasWindow && (hasStart || hasEnd) {
		return errors.New("--window cannot be used with --start or --end")
	}
	if hasWindow {
		if _, err := reportWindowBackendValue(flags.window); err != nil {
			return err
		}
	}
	if hasStart != hasEnd {
		return errors.New("--start and --end must be used together")
	}
	if hasStart {
		if err := validateRFC3339Flag("--start", flags.start); err != nil {
			return err
		}
		if err := validateRFC3339Flag("--end", flags.end); err != nil {
			return err
		}
	}
	return nil
}

// Builds SDK options from validated error report flags
func errorReportOptions(flags reportErrorFlags) nvfleetint.ErrorReportOptions {
	opts := nvfleetint.ErrorReportOptions{
		View:    nvfleetint.ErrorReportView(flags.view),
		GroupBy: nvfleetint.ErrorReportGroupBy(flags.groupBy),
		Format:  nvfleetint.ReportFormat(flags.format),
	}
	opts.ComputeZoneIDs, _ = clihelpers.ParseCommaList(flags.computeZoneIDs)
	opts.NodeGroupIDs, _ = clihelpers.ParseCommaList(flags.nodeGroupIDs)
	opts.Tags, _ = clihelpers.ParseCommaList(flags.tags)
	opts.Errors, _ = clihelpers.ParseCommaList(flags.errors)
	opts.Severities, _ = parseErrorSeverityList(flags.severities)
	opts.Step = strings.TrimSpace(flags.step)
	if opts.View == nvfleetint.ErrorReportViewGraph && opts.GroupBy == "" {
		opts.GroupBy = nvfleetint.ErrorReportGroupByError
	}
	if strings.TrimSpace(flags.window) != "" {
		opts.TimeMode = nvfleetint.ErrorReportTimeModeRelative
		window, _ := reportWindowBackendValue(flags.window)
		opts.Window = window
	}
	if strings.TrimSpace(flags.start) != "" {
		opts.TimeMode = nvfleetint.ErrorReportTimeModeAbsolute
		opts.StartTime = strings.TrimSpace(flags.start)
		opts.EndTime = strings.TrimSpace(flags.end)
	}
	return opts
}

// Validates and normalizes a backend duration value
func reportWindowBackendValue(window string) (string, error) {
	window = strings.TrimSpace(window)
	if !reportDurationPattern.MatchString(window) {
		return "", fmt.Errorf("invalid window %q: %s", window, reportDurationUnitsMessage)
	}

	duration, err := time.ParseDuration(window)
	if err != nil {
		return "", fmt.Errorf("invalid window %q: duration is too large; maximum is %s", window, maxReportWindow)
	}
	if duration <= 0 {
		return "", fmt.Errorf("invalid window %q: %s", window, reportDurationUnitsMessage)
	}
	return window, nil
}

// Validates a graph step width flag
func validateReportStepFlag(step string) error {
	step = strings.TrimSpace(step)
	if !reportDurationPattern.MatchString(step) {
		return fmt.Errorf("invalid step %q: %s", step, reportDurationUnitsMessage)
	}

	duration, err := time.ParseDuration(step)
	if err != nil {
		return fmt.Errorf("invalid step %q: duration is too large; maximum is %s", step, maxReportWindow)
	}
	if duration < time.Minute {
		return fmt.Errorf("invalid step %q: expected at least 1m", step)
	}
	return nil
}

// Parses comma-separated severities for error reports
func parseErrorSeverityList(raw string) ([]nvfleetint.ErrorSeverity, error) {
	return clihelpers.ParseEnumList[nvfleetint.ErrorSeverity]("severity", raw, "Critical, Fatal, Info, or Warning")
}

// Validates a timestamp flag as RFC3339
func validateRFC3339Flag(name, value string) error {
	if _, err := time.Parse(time.RFC3339, value); err != nil {
		return fmt.Errorf("%s must be RFC3339", name)
	}
	return nil
}

// Selects the paginated item key for all-page error list reports
func errorReportItemKey(groupBy nvfleetint.ErrorReportGroupBy) string {
	if groupBy == nvfleetint.ErrorReportGroupByNode {
		return "nodes"
	}
	return "errors"
}

// Extracts pagination for list-style error reports
func errorReportPagination(report nvfleetint.ErrorReport) *clioutput.Pagination {
	if report.View != nvfleetint.ErrorReportViewList {
		return nil
	}
	return &clioutput.Pagination{
		Page:     report.Page,
		PageSize: report.PageSize,
		Total:    report.Total,
	}
}

// Writes JSON or table output for inventory report results
func writeReportInventoryOutput(w io.Writer, common resolvedCommonFlags, result reportInventoryOutput) error {
	if common.output == clioutput.FormatJSON {
		return writePaginatedListJSON(w, result.RawJSON, result.JSONValue)
	}

	if err := writeReportInventoryTable(w, result.Report.Nodes); err != nil {
		return err
	}
	if result.Page == nil {
		return nil
	}
	return clioutput.WritePaginationFooter(w, *result.Page)
}

// Writes JSON or table output for error report results
func writeReportErrorOutput(w io.Writer, common resolvedCommonFlags, result reportErrorOutput) error {
	if common.output == clioutput.FormatJSON {
		return writePaginatedListJSON(w, result.RawJSON, result.JSONValue)
	}

	if err := writeReportErrorTable(w, result.Report); err != nil {
		return err
	}
	if result.Page == nil {
		return nil
	}
	return clioutput.WritePaginationFooter(w, *result.Page)
}

// Renders inventory report nodes as a table
func writeReportInventoryTable(w io.Writer, nodes []nvfleetint.InventoryNode) error {
	// "VERIFICATION CHECK" is the user-facing label for the backend integrityCheck field.
	return clioutput.WriteTable(w, []string{"UUID", "HOSTNAME", "COMPUTE ZONE", "NODE GROUP", "GPU TYPE", "GPU COUNT", "VERIFICATION CHECK", "FIRMWARE CHECK", "PUBLIC IP", "PRIVATE IP"}, reportInventoryRows(nodes))
}

// Renders an error report using its selected view
func writeReportErrorTable(w io.Writer, report nvfleetint.ErrorReport) error {
	switch report.View {
	case nvfleetint.ErrorReportViewList:
		if report.GroupBy == nvfleetint.ErrorReportGroupByNode {
			return clioutput.WriteTable(w, []string{"NODE UUID", "HOSTNAME", "ERRORS"}, reportErrorNodeRows(report.Nodes))
		}
		return clioutput.WriteTable(w, []string{"ERROR", "COUNT", "NODE COUNT", "SUGGESTED ACTION"}, reportErrorRows(report.Errors))
	case nvfleetint.ErrorReportViewGraph:
		return clioutput.WriteTable(w, []string{"ERROR", "VALUES", "START", "END"}, reportGraphRows(report.Graph))
	case nvfleetint.ErrorReportViewOverview:
		return clioutput.WriteTable(w, []string{"FIELD", "VALUE"}, reportOverviewRows(report.Overview))
	default:
		return fmt.Errorf("invalid report view %q", report.View)
	}
}

// Converts inventory nodes into table rows
func reportInventoryRows(nodes []nvfleetint.InventoryNode) [][]string {
	rows := make([][]string, 0, len(nodes))
	for _, node := range nodes {
		rows = append(rows, []string{
			clioutput.DisplayString(node.NodeUUID),
			clioutput.DisplayString(node.Hostname),
			clioutput.DisplayString(node.ComputeZone),
			clioutput.DisplayString(node.NodeGroup),
			clioutput.DisplayString(node.GPUType),
			clioutput.FormatOptionalInt(node.GPUCount),
			clioutput.DisplayString(node.IntegrityCheck),
			clioutput.DisplayString(node.FirmwareCheck),
			clioutput.DisplayString(node.PublicIP),
			clioutput.DisplayString(node.PrivateIP),
		})
	}
	return rows
}

// Converts error groupings into table rows
func reportErrorRows(errorsByType []nvfleetint.ErrorReportError) [][]string {
	rows := make([][]string, 0, len(errorsByType))
	for _, item := range errorsByType {
		rows = append(rows, []string{
			clioutput.DisplayString(item.Name),
			clioutput.FormatOptionalInt(item.Count),
			clioutput.FormatOptionalInt(item.NodeCount),
			formatSuggestedAction(item.SuggestedAction),
		})
	}
	return rows
}

// Converts node groupings into table rows
func reportErrorNodeRows(nodes []nvfleetint.ErrorReportNode) [][]string {
	rows := make([][]string, 0, len(nodes))
	for _, node := range nodes {
		rows = append(rows, []string{
			clioutput.DisplayString(node.NodeUUID),
			clioutput.DisplayString(node.Hostname),
			clioutput.FormatStringList(node.Errors),
		})
	}
	return rows
}

// Converts graph report series into table rows
func reportGraphRows(graph *nvfleetint.ErrorReportGraph) [][]string {
	if graph == nil {
		return nil
	}
	start, end := reportGraphTimeRange(graph.TimeRange)
	rows := make([][]string, 0, len(graph.Result))
	for _, series := range graph.Result {
		rows = append(rows, []string{
			clioutput.DisplayString(series.Error),
			clioutput.DisplayString(series.Values),
			clioutput.DisplayString(start),
			clioutput.DisplayString(end),
		})
	}
	return rows
}

// Converts overview report totals into table rows
func reportOverviewRows(overview *nvfleetint.ErrorReportOverview) [][]string {
	if overview == nil {
		return nil
	}
	return [][]string{
		{"TOTAL ERRORS", clioutput.FormatOptionalInt(overview.TotalErrors)},
		{"TOTAL ERROR NODES", clioutput.FormatOptionalInt(overview.TotalErrorNodes)},
		{"TOTAL ERROR TYPES", clioutput.FormatOptionalInt(overview.TotalErrorTypes)},
	}
}

// Formats a suggested action for table output
func formatSuggestedAction(action *nvfleetint.SuggestedAction) string {
	if action == nil {
		return "-"
	}

	actionText := strings.TrimSpace(action.Action)
	code := strings.TrimSpace(action.Code)
	switch {
	case actionText != "" && code != "":
		return fmt.Sprintf("%s (%s)", actionText, code)
	case actionText != "":
		return actionText
	case code != "":
		return code
	default:
		return "-"
	}
}

// Returns a graph report time range for table output
func reportGraphTimeRange(timeRange *nvfleetint.TimeRange) (string, string) {
	if timeRange == nil {
		return "", ""
	}
	return timeRange.Start, timeRange.End
}

// Writes a raw report payload to stdout
func writeRawReportBytes(w io.Writer, data []byte) error {
	_, err := w.Write(data)
	return err
}

// Writes a signed report bundle to disk and returns the path it was written to.
// The bundle lands in the current directory unless outputPath is supplied; an
// outputPath pointing at an existing directory keeps the suggested filename,
// otherwise it is treated as the destination file path.
func writeSignedReport(outputPath, filename string, data []byte) (string, error) {
	if filename == "" {
		filename = "inventory-report.zip"
	}

	target := filename
	if outputPath != "" {
		target = outputPath
		if info, err := os.Stat(outputPath); err == nil && info.IsDir() {
			target = filepath.Join(outputPath, filename)
		}
	}

	if err := os.WriteFile(target, data, 0o644); err != nil {
		return "", fmt.Errorf("write signed report: %w", err)
	}
	return target, nil
}
