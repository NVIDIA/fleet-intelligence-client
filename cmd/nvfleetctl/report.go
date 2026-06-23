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

	clihelpers "github.com/NVIDIA/fleet-intelligence-client/cmd/nvfleetctl/helpers"
	clioutput "github.com/NVIDIA/fleet-intelligence-client/internal/output"
	"github.com/NVIDIA/fleet-intelligence-client/pkg/fleetintelligence"

	"github.com/spf13/cobra"
)

const reportDurationUnitsMessage = "expected a positive duration using units ns, us, µs, ms, s, m, or h"

var (
	maxReportWindow       = time.Duration(1<<63 - 1)
	reportDurationPattern = regexp.MustCompile(`^\+?(?:(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:ns|us|µs|ms|s|m|h))+$`)
)

// Stores local flag values for inventory reports
type reportInventoryFlags struct {
	format     string
	signed     bool
	outputPath string
}

// Stores local flag values for error reports
type reportErrorFlags struct {
	view    string
	groupBy string
	format  string
	window  string
	start   string
	end     string
}

// Stores local flag values for report verification
type reportVerifyFlags struct {
	csv    string
	bundle string
	key    string
}

// Stores data ready for inventory report rendering
type reportInventoryOutput struct {
	Report    fleetintelligence.InventoryReport
	JSONValue any
	RawJSON   []byte
	Page      *clioutput.Pagination
}

// Stores data ready for error report rendering
type reportErrorOutput struct {
	Report    fleetintelligence.ErrorReport
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

	return cmd
}

// Creates the inventory report command
func newReportInventoryCmd() *cobra.Command {
	flags := reportInventoryFlags{
		format: string(fleetintelligence.ReportFormatJSON),
	}
	common := newCommonFlags()

	cmd := &cobra.Command{
		Use:   "inventory",
		Short: "Generate an inventory report",
		Long: `Generate an inventory report for the fleet.

Use --signed with --format csv to download a signed CSV bundle (a zip
containing the CSV plus a cosign-verifiable signature). The bundle is
written to the current directory unless --output-path is provided.`,
		Example: `  # Inventory report as a table (default)
  nvfleetctl report inventory

  # Download a signed CSV bundle into the current directory
  nvfleetctl report inventory --format csv --signed

  # Download a signed CSV bundle to a specific path
  nvfleetctl report inventory --format csv --signed --output-path ./reports/inventory.zip`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runReportInventory(cmd, flags, resolveCommonFlags(cmd, common))
		},
	}

	cmd.Flags().StringVar(&flags.format, "format", flags.format, "Report format: json or csv")
	cmd.Flags().BoolVar(&flags.signed, "signed", false, "Download a signed CSV bundle (zip of CSV plus signature); requires --format csv")
	cmd.Flags().StringVar(&flags.outputPath, "output-path", "", "Destination file or directory for the signed bundle; defaults to the current directory")
	registerListCommonFlags(cmd, common)

	return cmd
}

// Creates the error report command
func newReportErrorCmd() *cobra.Command {
	flags := reportErrorFlags{
		view:   string(fleetintelligence.ErrorReportViewOverview),
		format: string(fleetintelligence.ReportFormatJSON),
	}
	common := newCommonFlags()

	cmd := &cobra.Command{
		Use:   "error",
		Short: "Generate an error report",
		Long: `Generate an error report for the fleet.

Views:
  overview  Summary totals for the time range (default).
  list      Per-error or per-node breakdown; requires --group-by.
  graph     Time series of error counts.

A time range is always required: use --window for a relative range, or
--start and --end for an absolute range.`,
		Example: `  # Summary totals over the last 24 hours (default view)
  nvfleetctl report error --window 24h

  # Errors grouped by type over the last 7 days
  nvfleetctl report error --view list --group-by error --window 168h

  # Errors grouped by node for an absolute range
  nvfleetctl report error --view list --group-by node --start 2026-05-01T00:00:00Z --end 2026-05-08T00:00:00Z

  # Error count time series
  nvfleetctl report error --view graph --window 24h`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runReportError(cmd, flags, resolveCommonFlags(cmd, common))
		},
	}

	cmd.Flags().StringVar(&flags.view, "view", flags.view, "Report view: list, graph, or overview")
	cmd.Flags().StringVar(&flags.groupBy, "group-by", "", "Group list view by: error or node")
	cmd.Flags().StringVar(&flags.format, "format", flags.format, "Report format: json or csv")
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
  nvfleetctl report verify \
    --csv <report>.csv \
    --bundle <report>.sig.bundle

  # Verify offline with a previously downloaded public key
  nvfleetctl report verify \
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
	registerTimeoutFlag(cmd, common)

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

	if err := fleetintelligence.VerifySignedReport(csv, bundle, key); err != nil {
		return reportVerifyError(flags, err)
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
	case errors.Is(err, fleetintelligence.ErrInvalidBundle):
		return fmt.Errorf("%q is not a valid signature bundle; pass the report's .sig.bundle file to --bundle", flags.bundle)
	case errors.Is(err, fleetintelligence.ErrInvalidKey):
		if flags.key == "" {
			return errors.New("the signing key fetched from the API is not a valid PEM public key")
		}
		return fmt.Errorf("%q is not a valid PEM public key; pass the signing key (e.g. signing-key.pub) to --key", flags.key)
	case errors.Is(err, fleetintelligence.ErrVerificationFailed):
		return fmt.Errorf("verification failed: %q does not match the signature in %q.\n"+
			"The report may have been modified, or --csv and --bundle may point to the wrong files.", flags.csv, flags.bundle)
	default:
		return err
	}
}

// Loads the verification key from --key or fetches it from the configured API
func reportVerifyKey(cmd *cobra.Command, flags reportVerifyFlags, common resolvedCommonFlags) ([]byte, error) {
	if flags.key != "" {
		return readVerifyFile("--key", flags.key)
	}

	client, err := newConfiguredClient(commonClientOptions(common)...)
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

	client, err := newConfiguredClient(commonClientOptions(common)...)
	if err != nil {
		return err
	}

	opts := fleetintelligence.InventoryReportOptions{
		Format: fleetintelligence.ReportFormat(flags.format),
		Signed: flags.signed,
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
		fmt.Fprintf(cmd.OutOrStdout(), "Signed report written to %s\n", path)
		return nil
	}

	if fleetintelligence.ReportFormat(flags.format) == fleetintelligence.ReportFormatCSV {
		report, err := client.GetInventoryReport(cmd.Context(), opts)
		if err != nil {
			return err
		}
		return writeRawReportBytes(cmd.OutOrStdout(), report.RawCSV)
	}

	if common.all {
		var nodes []fleetintelligence.InventoryNode
		result, err := clihelpers.FetchAllRawPages("nodes", 0, func(pageNumber int) (clihelpers.RawPage, error) {
			page := pageNumber
			opts.Page = &page
			currentPage, err := client.GetInventoryReport(cmd.Context(), opts)
			if err != nil {
				return clihelpers.RawPage{}, err
			}
			nodes = append(nodes, currentPage.Nodes...)
			hasMore := currentPage.HasMore
			return clihelpers.RawPage{
				RawJSON:  currentPage.RawJSON,
				Page:     currentPage.Page,
				PageSize: currentPage.PageSize,
				Total:    currentPage.Total,
				HasMore:  &hasMore,
			}, nil
		})
		if err != nil {
			return err
		}
		return writeReportInventoryOutput(cmd.OutOrStdout(), common, reportInventoryOutput{
			Report: fleetintelligence.InventoryReport{
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
			HasMore:  report.HasMore,
		},
	})
}

// Validates flags, calls the SDK, and writes error report output
func runReportError(cmd *cobra.Command, flags reportErrorFlags, common resolvedCommonFlags) error {
	if err := validateReportErrorFlags(flags, common); err != nil {
		return err
	}

	client, err := newConfiguredClient(commonClientOptions(common)...)
	if err != nil {
		return err
	}

	opts := errorReportOptions(flags)
	applyPagination(common, func(page *int) { opts.Page = page }, func(pageSize *int) { opts.PageSize = pageSize })

	if fleetintelligence.ReportFormat(flags.format) == fleetintelligence.ReportFormatCSV {
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
func runReportErrorAll(cmd *cobra.Command, client *fleetintelligence.Client, opts fleetintelligence.ErrorReportOptions, flags reportErrorFlags, common resolvedCommonFlags) error {
	var errorsByType []fleetintelligence.ErrorReportError
	var nodes []fleetintelligence.ErrorReportNode
	itemKey := errorReportItemKey(fleetintelligence.ErrorReportGroupBy(flags.groupBy))

	result, err := clihelpers.FetchAllRawPages(itemKey, 0, func(pageNumber int) (clihelpers.RawPage, error) {
		page := pageNumber
		opts.Page = &page
		currentPage, err := client.GetErrorReport(cmd.Context(), opts)
		if err != nil {
			return clihelpers.RawPage{}, err
		}
		errorsByType = append(errorsByType, currentPage.Errors...)
		nodes = append(nodes, currentPage.Nodes...)
		hasMore := currentPage.HasMore
		return clihelpers.RawPage{
			RawJSON:  currentPage.RawJSON,
			Page:     currentPage.Page,
			PageSize: currentPage.PageSize,
			Total:    currentPage.Total,
			HasMore:  &hasMore,
		}, nil
	})
	if err != nil {
		return err
	}

	return writeReportErrorOutput(cmd.OutOrStdout(), common, reportErrorOutput{
		Report: fleetintelligence.ErrorReport{
			View:    fleetintelligence.ErrorReportView(flags.view),
			GroupBy: fleetintelligence.ErrorReportGroupBy(flags.groupBy),
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
	if !fleetintelligence.ReportFormat(flags.format).Valid() {
		return fmt.Errorf("invalid format %q: expected json or csv", flags.format)
	}
	if flags.signed && fleetintelligence.ReportFormat(flags.format) != fleetintelligence.ReportFormatCSV {
		return errors.New("--signed requires --format csv")
	}
	if flags.outputPath != "" && !flags.signed {
		return errors.New("--output-path can only be used with --signed")
	}
	if fleetintelligence.ReportFormat(flags.format) == fleetintelligence.ReportFormatCSV {
		if common.outputSet {
			return errors.New("--output cannot be used with --format csv")
		}
		if common.allSet || common.pageSet || common.pageSizeSet {
			return errors.New("pagination flags cannot be used with --format csv")
		}
	}
	return nil
}

// Checks error report flags
func validateReportErrorFlags(flags reportErrorFlags, common resolvedCommonFlags) error {
	if err := validateListCommonFlags(common); err != nil {
		return err
	}

	view := fleetintelligence.ErrorReportView(flags.view)
	groupBy := fleetintelligence.ErrorReportGroupBy(flags.groupBy)
	format := fleetintelligence.ReportFormat(flags.format)

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

	if err := validateReportErrorViewFlags(view, groupBy, format, flags, common); err != nil {
		return err
	}
	return validateReportTimeFlags(flags)
}

// Checks view-specific error report flags
func validateReportErrorViewFlags(view fleetintelligence.ErrorReportView, groupBy fleetintelligence.ErrorReportGroupBy, format fleetintelligence.ReportFormat, flags reportErrorFlags, common resolvedCommonFlags) error {
	if format == fleetintelligence.ReportFormatCSV {
		if common.outputSet {
			return errors.New("--output cannot be used with --format csv")
		}
		if common.allSet || common.pageSet || common.pageSizeSet {
			return errors.New("pagination flags cannot be used with --format csv")
		}
		if view != fleetintelligence.ErrorReportViewList {
			return errors.New("--format csv is only supported with --view list")
		}
	}

	switch view {
	case fleetintelligence.ErrorReportViewList:
		if strings.TrimSpace(flags.groupBy) == "" {
			return errors.New("--group-by is required for --view list")
		}
	case fleetintelligence.ErrorReportViewGraph:
		if common.allSet || common.pageSet || common.pageSizeSet {
			return errors.New("pagination flags cannot be used with --view graph")
		}
		if strings.TrimSpace(flags.groupBy) != "" && groupBy != fleetintelligence.ErrorReportGroupByError {
			return errors.New("--view graph only supports --group-by error")
		}
	case fleetintelligence.ErrorReportViewOverview:
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
func errorReportOptions(flags reportErrorFlags) fleetintelligence.ErrorReportOptions {
	opts := fleetintelligence.ErrorReportOptions{
		View:    fleetintelligence.ErrorReportView(flags.view),
		GroupBy: fleetintelligence.ErrorReportGroupBy(flags.groupBy),
		Format:  fleetintelligence.ReportFormat(flags.format),
	}
	if opts.View == fleetintelligence.ErrorReportViewGraph && opts.GroupBy == "" {
		opts.GroupBy = fleetintelligence.ErrorReportGroupByError
	}
	if strings.TrimSpace(flags.window) != "" {
		opts.TimeMode = fleetintelligence.ErrorReportTimeModeRelative
		window, _ := reportWindowBackendValue(flags.window)
		opts.Window = window
	}
	if strings.TrimSpace(flags.start) != "" {
		opts.TimeMode = fleetintelligence.ErrorReportTimeModeAbsolute
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

// Validates a timestamp flag as RFC3339
func validateRFC3339Flag(name, value string) error {
	if _, err := time.Parse(time.RFC3339, value); err != nil {
		return fmt.Errorf("%s must be RFC3339", name)
	}
	return nil
}

// Selects the paginated item key for all-page error list reports
func errorReportItemKey(groupBy fleetintelligence.ErrorReportGroupBy) string {
	if groupBy == fleetintelligence.ErrorReportGroupByNode {
		return "nodes"
	}
	return "errors"
}

// Extracts pagination for list-style error reports
func errorReportPagination(report fleetintelligence.ErrorReport) *clioutput.Pagination {
	if report.View != fleetintelligence.ErrorReportViewList {
		return nil
	}
	return &clioutput.Pagination{
		Page:     report.Page,
		PageSize: report.PageSize,
		Total:    report.Total,
		HasMore:  report.HasMore,
	}
}

// Writes JSON or table output for inventory report results
func writeReportInventoryOutput(w io.Writer, common resolvedCommonFlags, result reportInventoryOutput) error {
	if common.output == clioutput.FormatJSON {
		if result.RawJSON != nil {
			return clioutput.WriteRawJSON(w, result.RawJSON)
		}
		return clioutput.WriteJSON(w, result.JSONValue)
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
		if result.RawJSON != nil {
			return clioutput.WriteRawJSON(w, result.RawJSON)
		}
		return clioutput.WriteJSON(w, result.JSONValue)
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
func writeReportInventoryTable(w io.Writer, nodes []fleetintelligence.InventoryNode) error {
	return clioutput.WriteTable(w, []string{"UUID", "HOSTNAME", "COMPUTE ZONE", "NODE GROUP", "GPU TYPE", "GPU COUNT", "INTEGRITY CHECK", "FIRMWARE CHECK", "PUBLIC IP", "PRIVATE IP"}, reportInventoryRows(nodes))
}

// Renders an error report using its selected view
func writeReportErrorTable(w io.Writer, report fleetintelligence.ErrorReport) error {
	switch report.View {
	case fleetintelligence.ErrorReportViewList:
		if report.GroupBy == fleetintelligence.ErrorReportGroupByNode {
			return clioutput.WriteTable(w, []string{"NODE UUID", "HOSTNAME", "ERRORS"}, reportErrorNodeRows(report.Nodes))
		}
		return clioutput.WriteTable(w, []string{"ERROR", "COUNT", "NODE COUNT", "SUGGESTED ACTION"}, reportErrorRows(report.Errors))
	case fleetintelligence.ErrorReportViewGraph:
		return clioutput.WriteTable(w, []string{"ERROR", "VALUES", "START", "END"}, reportGraphRows(report.Graph))
	case fleetintelligence.ErrorReportViewOverview:
		return clioutput.WriteTable(w, []string{"FIELD", "VALUE"}, reportOverviewRows(report.Overview))
	default:
		return fmt.Errorf("invalid report view %q", report.View)
	}
}

// Converts inventory nodes into table rows
func reportInventoryRows(nodes []fleetintelligence.InventoryNode) [][]string {
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
func reportErrorRows(errorsByType []fleetintelligence.ErrorReportError) [][]string {
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
func reportErrorNodeRows(nodes []fleetintelligence.ErrorReportNode) [][]string {
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
func reportGraphRows(graph *fleetintelligence.ErrorReportGraph) [][]string {
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
func reportOverviewRows(overview *fleetintelligence.ErrorReportOverview) [][]string {
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
func formatSuggestedAction(action *fleetintelligence.SuggestedAction) string {
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
func reportGraphTimeRange(timeRange *fleetintelligence.TimeRange) (string, string) {
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
