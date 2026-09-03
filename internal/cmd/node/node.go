// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package node

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdutil"
	clioutput "github.com/NVIDIA/fleet-intelligence-client/internal/output"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"

	"github.com/spf13/cobra"
)

// Lists the sort fields accepted by node list
const nodeSortByList = "hostname, nodeUUID, healthStatus, nodegroup, computezone, gpuType, gpuCount, " +
	"verificationCheck, agentStatus, agentVersion, kernelVersion, gpuDriverVersion, gpuFirmwareVersions, " +
	"nodeName, or bmcHostname"

// Stores local flag values for node list
type nodeListFlags struct {
	view              string
	agentType         string
	nodeUUIDs         string
	health            string
	computeZoneIDs    string
	computeZoneNames  string
	nodeGroupIDs      string
	nodeGroupNames    string
	gpuType           string
	gpuCount          string
	publicIP          string
	privateIP         string
	hostname          string
	nodeName          string
	bmcHostname       string
	agentStatus       string
	verificationCheck string
	firmwareCheck     string
	sortBy            string
	order             string
}

// Stores data ready for node list rendering
type nodeListOutput struct {
	Nodes     []nvfleetint.Node
	View      string
	AgentType nvfleetint.NodeAgentType
	JSONValue any
	RawJSON   []byte
	Page      *clioutput.Pagination
}

type nodeListResult struct {
	Inband *nodeListOutput
	OOB    *nodeListOutput
	// Set instead of the corresponding *nodeListOutput above when that leg's
	// filters/sort-by aren't valid for its agent type (backend 400): the
	// other, compatible leg still renders rather than failing the whole
	// command.
	InbandSkipReason string
	OOBSkipReason    string
}

type combinedNodeListJSON struct {
	Inband      any    `json:"inband"`
	OOB         any    `json:"oob"`
	InbandError string `json:"inbandError,omitempty"`
	OOBError    string `json:"oobError,omitempty"`
}

type nodeDescribeFlags struct {
	agentType string
	sections  string
}

type nodeDescribeRenderOptions struct {
	showSummary bool
	sections    map[oobInventorySection]bool
}

type nodeDescribeResult struct {
	Inband *nvfleetint.NodeDetails
	OOB    *nvfleetint.NodeDetails
}

type combinedNodeDescribeJSON struct {
	Inband json.RawMessage `json:"inband,omitempty"`
	OOB    json.RawMessage `json:"oob,omitempty"`
}

type oobInventorySection string

const (
	oobInventorySectionManagers oobInventorySection = "managers"
	oobInventorySectionSystems  oobInventorySection = "systems"
	oobInventorySectionChassis  oobInventorySection = "chassis"
	oobInventorySectionFirmware oobInventorySection = "firmware"
)

var oobInventorySectionOrder = []oobInventorySection{
	oobInventorySectionManagers,
	oobInventorySectionSystems,
	oobInventorySectionChassis,
	oobInventorySectionFirmware,
}

// Creates the top-level node command group
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "node",
		Short: "Inspect nodes",
	}

	cmd.AddCommand(newNodeListCmd())
	cmd.AddCommand(newNodeDescribeCmd())
	cmd.AddCommand(newNodeHealthCmd())
	cmd.AddCommand(newNodeOptionsCmd())
	cmdutil.RejectUnknownSubcommands(cmd)

	return cmd
}

// Creates the node list command
func newNodeListCmd() *cobra.Command {
	flags := nodeListFlags{
		view: string(nvfleetint.NodeViewDetail),
	}
	common := cmdutil.NewCommon()

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List nodes",
		Long: "List nodes.\n\n" +
			cmdutil.OptionsHelpNote("nvfleetint node options",
				"--health", "--compute-zone-ids", "--nodegroup-ids",
				"--gpu-type", "--agent-status", "--sort-by", "--order"),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runNodeList(cmd, flags, cmdutil.ResolveCommon(cmd, common))
		},
	}

	cmd.Flags().StringVar(&flags.view, "view", flags.view, "View mode: detail or basic")
	cmd.Flags().StringVar(
		&flags.agentType,
		"agent-type",
		flags.agentType,
		"Agent type view: inband or oob (detail view defaults to both)",
	)
	cmd.Flags().StringVar(&flags.nodeUUIDs, "node-uuids", "", "Comma-separated node UUIDs to filter")
	cmd.Flags().StringVar(&flags.health, "health", "", "Comma-separated health states to filter")
	cmd.Flags().StringVar(&flags.computeZoneIDs, "compute-zone-ids", "", "Comma-separated compute zone IDs to filter")
	cmd.Flags().StringVar(&flags.computeZoneNames, "compute-zone-names", "", "Comma-separated compute zone names to filter (partial match)")
	cmd.Flags().StringVar(&flags.nodeGroupIDs, "nodegroup-ids", "", "Comma-separated node group IDs to filter")
	cmd.Flags().StringVar(&flags.nodeGroupNames, "nodegroup-names", "", "Comma-separated node group names to filter (partial match)")
	cmd.Flags().StringVar(&flags.gpuType, "gpu-type", "", "Comma-separated GPU types to filter")
	cmd.Flags().StringVar(&flags.gpuCount, "gpu-count", "", "Comma-separated GPU counts to filter")
	cmd.Flags().StringVar(&flags.publicIP, "public-ip", "", "Comma-separated public IP addresses to filter")
	cmd.Flags().StringVar(&flags.privateIP, "private-ip", "", "Comma-separated private IP addresses to filter")
	cmd.Flags().StringVar(&flags.hostname, "hostname", "", "Hostname partial match")
	cmd.Flags().StringVar(&flags.nodeName, "node-name", "", "Node name partial match (OOB view)")
	cmd.Flags().StringVar(&flags.bmcHostname, "bmc-hostname", "", "BMC hostname partial match (OOB view)")
	cmd.Flags().StringVar(&flags.agentStatus, "agent-status", "", "Comma-separated agent statuses to filter")
	cmd.Flags().StringVar(&flags.verificationCheck, "verification-check", "", "Comma-separated verification check statuses to filter: Verified, Unverified, Degraded, Pending, Unsupported, or Unknown")
	cmd.Flags().StringVar(&flags.firmwareCheck, "firmware-check", "", "Comma-separated firmware check statuses to filter: Passed, Failed, or Unknown")
	cmd.Flags().StringVar(&flags.sortBy, "sort-by", "", "Sort field (including verificationCheck)")
	cmd.Flags().StringVar(&flags.order, "order", "", "Sort order")
	cmdutil.RegisterListFlags(cmd, common)

	return cmd
}

// Creates the node describe command
func newNodeDescribeCmd() *cobra.Command {
	common := cmdutil.NewCommon()
	flags := nodeDescribeFlags{}
	cmd := &cobra.Command{
		Use:   "describe <uuid>",
		Short: "Describe a node",
		Args:  cmdutil.RequireSingleArg("node UUID"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runNodeDescribe(cmd, args[0], flags, cmdutil.ResolveCommon(cmd, common))
		},
	}

	cmd.Flags().StringVar(
		&flags.agentType,
		"agent-type",
		flags.agentType,
		"Agent type view: inband or oob (default: both)",
	)
	cmd.Flags().StringVar(
		&flags.sections,
		"section",
		"",
		"Comma-separated OOB inventory sections: managers, systems, chassis, firmware, or all",
	)
	cmdutil.RegisterReadFlags(cmd, common)

	return cmd
}

// Validates flags, calls the SDK, and writes output
func runNodeList(cmd *cobra.Command, flags nodeListFlags, common cmdutil.Resolved) error {
	if err := cmdutil.ValidateListFlags(common); err != nil {
		return err
	}
	opts, err := nodeListOptions(flags)
	if err != nil {
		return err
	}

	client, err := cmdutil.New(common)
	if err != nil {
		return err
	}

	cmdutil.ApplyPagination(common, func(page *int) { opts.Page = page }, func(pageSize *int) { opts.PageSize = pageSize })
	agentType := nvfleetint.NodeAgentType(flags.agentType)
	if nvfleetint.NodeView(flags.view) == nvfleetint.NodeViewDetail && agentType == "" {
		result, err := listNodeViews(cmd.Context(), client, opts, common)
		if err != nil {
			return err
		}
		return writeNodeListResult(cmd.OutOrStdout(), common, result)
	}

	result, err := fetchNodeList(cmd.Context(), client, opts, common)
	if err != nil {
		return err
	}
	return writeNodeListOutput(cmd.OutOrStdout(), common, result)
}

func listNodeViews(
	ctx context.Context,
	client *nvfleetint.Client,
	opts nvfleetint.ListNodesOptions,
	common cmdutil.Resolved,
) (nodeListResult, error) {
	var result nodeListResult
	var inbandOutput nodeListOutput
	var oobOutput nodeListOutput
	var inbandErr error
	var oobErr error
	var waitGroup sync.WaitGroup
	waitGroup.Add(2)

	go func() {
		defer waitGroup.Done()
		inbandOptions := opts
		inbandOptions.AgentType = nvfleetint.NodeAgentTypeInband
		inbandOutput, inbandErr = fetchNodeList(ctx, client, inbandOptions, common)
	}()
	go func() {
		defer waitGroup.Done()
		oobOptions := opts
		oobOptions.AgentType = nvfleetint.NodeAgentTypeOOB
		oobOutput, oobErr = fetchNodeList(ctx, client, oobOptions, common)
	}()
	waitGroup.Wait()

	// A 400 here means this leg's filters/sort-by don't apply to its agent
	// type (e.g. --gpu-type against the OOB view) rather than a real
	// failure, so it doesn't need to take down the whole command: the other,
	// compatible leg still has something useful to show.
	inbandBadFilter := apiErrorHasStatus(inbandErr, http.StatusBadRequest)
	oobBadFilter := apiErrorHasStatus(oobErr, http.StatusBadRequest)

	switch {
	case inbandErr != nil && !inbandBadFilter:
		return nodeListResult{}, fmt.Errorf("fetch in-band node list: %w", inbandErr)
	case oobErr != nil && !oobBadFilter:
		return nodeListResult{}, fmt.Errorf("fetch OOB node list: %w", oobErr)
	case inbandBadFilter && oobBadFilter:
		return nodeListResult{}, fmt.Errorf(
			"these filters are not supported for either the in-band or OOB view: %w", inbandErr,
		)
	}

	if inbandBadFilter {
		result.InbandSkipReason = inbandErr.Error()
	} else {
		result.Inband = &inbandOutput
	}
	if oobBadFilter {
		result.OOBSkipReason = oobErr.Error()
	} else {
		result.OOB = &oobOutput
	}
	return result, nil
}

func fetchNodeList(
	ctx context.Context,
	client *nvfleetint.Client,
	opts nvfleetint.ListNodesOptions,
	common cmdutil.Resolved,
) (nodeListOutput, error) {
	if common.All {
		var nodes []nvfleetint.Node
		result, err := cmdutil.FetchAllPages("nodes",
			func(pageNumber int) (nvfleetint.NodesPage, error) {
				opts.Page = &pageNumber
				return client.ListNodes(ctx, opts)
			},
			func(page nvfleetint.NodesPage) { nodes = append(nodes, page.Nodes...) },
		)
		if err != nil {
			return nodeListOutput{}, friendlyNodeListError(err, opts)
		}
		return nodeListOutput{
			Nodes:     nodes,
			View:      string(opts.View),
			AgentType: opts.AgentType,
			JSONValue: result,
		}, nil
	}

	page, err := client.ListNodes(ctx, opts)
	if err != nil {
		return nodeListOutput{}, friendlyNodeListError(err, opts)
	}
	return nodeListOutput{
		Nodes:     page.Nodes,
		View:      string(opts.View),
		AgentType: opts.AgentType,
		RawJSON:   page.RawJSON,
		Page: &clioutput.Pagination{
			Page:     page.Page,
			PageSize: page.PageSize,
			Total:    page.Total,
		},
	}, nil
}

// Validates args, calls the SDK, and writes output
func runNodeDescribe(
	cmd *cobra.Command,
	nodeUUID string,
	flags nodeDescribeFlags,
	common cmdutil.Resolved,
) error {
	if err := cmdutil.ValidateReadFlags(common); err != nil {
		return err
	}

	nodeUUID = strings.TrimSpace(nodeUUID)
	if nodeUUID == "" {
		return errors.New("node UUID is required")
	}
	agentType := nvfleetint.NodeAgentType(flags.agentType)
	if agentType != "" && !agentType.Valid() {
		return fmt.Errorf("invalid agent-type %q: expected inband or oob", agentType)
	}
	renderOptions, err := parseOOBInventorySections(flags, agentType, common.Output)
	if err != nil {
		return err
	}

	client, err := cmdutil.New(common)
	if err != nil {
		return err
	}

	result, err := describeNodeResult(cmd.Context(), client, nodeUUID, agentType)
	if err != nil {
		return err
	}

	if common.Output == clioutput.FormatJSON {
		return writeNodeDescribeJSON(cmd.OutOrStdout(), result, agentType)
	}
	if !renderOptions.showSummary && (result.OOB == nil || result.OOB.OOBInventory == nil) {
		return fmt.Errorf("node %q does not have an OOB view for the requested section", nodeUUID)
	}
	return writeNodeDescribeResultTable(cmd.OutOrStdout(), result, agentType, renderOptions)
}

func describeNodeResult(
	ctx context.Context,
	client *nvfleetint.Client,
	nodeUUID string,
	agentType nvfleetint.NodeAgentType,
) (nodeDescribeResult, error) {
	if agentType != "" {
		node, err := client.DescribeNodeWithOptions(ctx, nodeUUID, nvfleetint.DescribeNodeOptions{
			AgentType: agentType,
		})
		if err != nil {
			return nodeDescribeResult{}, err
		}
		if agentType == nvfleetint.NodeAgentTypeOOB {
			return nodeDescribeResult{OOB: &node}, nil
		}
		return nodeDescribeResult{Inband: &node}, nil
	}

	var result nodeDescribeResult
	var inbandErr error
	var oobErr error
	var waitGroup sync.WaitGroup
	waitGroup.Add(2)
	go func() {
		defer waitGroup.Done()
		node, err := client.DescribeNodeWithOptions(ctx, nodeUUID, nvfleetint.DescribeNodeOptions{
			AgentType: nvfleetint.NodeAgentTypeInband,
		})
		if err != nil {
			inbandErr = err
			return
		}
		result.Inband = &node
	}()
	go func() {
		defer waitGroup.Done()
		node, err := client.DescribeNodeWithOptions(ctx, nodeUUID, nvfleetint.DescribeNodeOptions{
			AgentType: nvfleetint.NodeAgentTypeOOB,
		})
		if err != nil {
			oobErr = err
			return
		}
		result.OOB = &node
	}()
	waitGroup.Wait()

	inbandMissing := apiErrorHasStatus(inbandErr, http.StatusNotFound)
	oobMissing := apiErrorHasStatus(oobErr, http.StatusNotFound)
	switch {
	case inbandErr != nil && !inbandMissing:
		return nodeDescribeResult{}, fmt.Errorf("fetch in-band node view: %w", inbandErr)
	case oobErr != nil && !oobMissing:
		return nodeDescribeResult{}, fmt.Errorf("fetch OOB node view: %w", oobErr)
	case inbandMissing && oobMissing:
		return nodeDescribeResult{}, fmt.Errorf("node %q was not found in either the in-band or OOB view", nodeUUID)
	default:
		return result, nil
	}
}

func apiErrorHasStatus(err error, statusCode int) bool {
	var apiErr *nvfleetint.APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == statusCode
}

// friendlyNodeListError rewrites a backend 400 from GET /v1/nodes into a
// message that names the flag combination at fault. The API rejects a
// --sort-by field or filter that isn't valid for the requested agent type or
// view (e.g. --sort-by gpuType with --agent-type oob, since OOB nodes carry
// no GPU inventory) but only reports it as a generic "Bad Request", so the
// CLI adds that context here. The original *nvfleetint.APIError stays
// wrapped via %w so exit codes and -o json error output are unaffected.
func friendlyNodeListError(err error, opts nvfleetint.ListNodesOptions) error {
	var apiErr *nvfleetint.APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusBadRequest {
		return err
	}

	scope := "the basic view"
	optionsCmd := "nvfleetint node options"
	if opts.AgentType != "" {
		scope = fmt.Sprintf("--agent-type %s", opts.AgentType)
		optionsCmd = fmt.Sprintf("%s --agent-type %s", optionsCmd, opts.AgentType)
	}

	return fmt.Errorf(
		"a filter or --sort-by value is not supported for %s; run %q to see what's accepted: %w",
		scope, optionsCmd, err,
	)
}

func writeNodeDescribeJSON(
	w io.Writer,
	result nodeDescribeResult,
	agentType nvfleetint.NodeAgentType,
) error {
	if agentType == nvfleetint.NodeAgentTypeInband && result.Inband != nil {
		return clioutput.WriteRawJSON(w, result.Inband.RawJSON)
	}
	if agentType == nvfleetint.NodeAgentTypeOOB && result.OOB != nil {
		return clioutput.WriteRawJSON(w, result.OOB.RawJSON)
	}

	output := combinedNodeDescribeJSON{}
	if result.Inband != nil {
		output.Inband = json.RawMessage(result.Inband.RawJSON)
	}
	if result.OOB != nil {
		output.OOB = json.RawMessage(result.OOB.RawJSON)
	}
	return clioutput.WriteJSON(w, output)
}

func parseOOBInventorySections(
	flags nodeDescribeFlags,
	agentType nvfleetint.NodeAgentType,
	output string,
) (nodeDescribeRenderOptions, error) {
	values, err := cmdutil.ParseCommaList(flags.sections)
	if err != nil {
		return nodeDescribeRenderOptions{}, err
	}
	showAll := false
	for _, value := range values {
		if strings.EqualFold(value, "all") {
			showAll = true
		}
	}
	if showAll && len(values) != 1 {
		return nodeDescribeRenderOptions{}, errors.New("section all cannot be combined with other sections")
	}
	if len(values) > 0 &&
		agentType != "" &&
		agentType != nvfleetint.NodeAgentTypeOOB {
		return nodeDescribeRenderOptions{}, errors.New("--section requires the OOB view")
	}
	if len(values) > 0 && output == clioutput.FormatJSON {
		return nodeDescribeRenderOptions{}, errors.New(
			"--section cannot be used with --output json; JSON already includes the full inventory",
		)
	}

	options := nodeDescribeRenderOptions{
		showSummary: len(values) == 0 || showAll,
		sections:    make(map[oobInventorySection]bool, len(oobInventorySectionOrder)),
	}
	if showAll {
		for _, section := range oobInventorySectionOrder {
			options.sections[section] = true
		}
		return options, nil
	}
	for _, value := range values {
		section := oobInventorySection(strings.ToLower(value))
		switch section {
		case oobInventorySectionManagers,
			oobInventorySectionSystems,
			oobInventorySectionChassis,
			oobInventorySectionFirmware:
			options.sections[section] = true
		default:
			return nodeDescribeRenderOptions{}, fmt.Errorf(
				"invalid OOB inventory section %q: expected managers, systems, chassis, or firmware",
				value,
			)
		}
	}
	return options, nil
}

// Names the flag carrying each node list option, for rendering SDK validation
// errors against what the user typed.
var nodeListFlagNames = map[string]cmdutil.OptionFlagName{
	"view":              {Flag: "view"},
	"agentType":         {Flag: "agent-type"},
	"health":            {Flag: "health"},
	"agentStatus":       {Flag: "agent-status"},
	"verificationCheck": {Flag: "verification-check"},
	"firmwareCheck":     {Flag: "firmware-check"},
	"gpuCount":          {Flag: "gpu-count"},
	"order":             {Flag: "order"},
	// The CLI accepts a subset of the backend's sort fields, so it lists them
	// itself rather than echoing the backend's.
	"sortBy": {Flag: "sort-by", Expected: nodeSortByList},
}

// nodeListOptions reads every node list flag exactly once and hands the result
// to the SDK to validate, so the accepted values and the view compatibility
// rules are stated in one place rather than in both layers.
func nodeListOptions(flags nodeListFlags) (nvfleetint.ListNodesOptions, error) {
	var err error

	opts := nvfleetint.ListNodesOptions{
		View:        nvfleetint.NodeView(flags.view),
		AgentType:   nvfleetint.NodeAgentType(flags.agentType),
		Hostname:    strings.TrimSpace(flags.hostname),
		NodeName:    strings.TrimSpace(flags.nodeName),
		BMCHostname: strings.TrimSpace(flags.bmcHostname),
		SortBy:      nvfleetint.NodeSortBy(strings.TrimSpace(flags.sortBy)),
		Order:       nvfleetint.NodeSortOrder(flags.order),
	}

	if opts.HealthStatuses, err = cmdutil.ParseTypedList[nvfleetint.NodeHealthStatus](
		"health", flags.health); err != nil {
		return opts, err
	}
	if opts.AgentStatuses, err = cmdutil.ParseTypedList[nvfleetint.NodeAgentStatus](
		"agent-status", flags.agentStatus); err != nil {
		return opts, err
	}
	if opts.VerificationChecks, err = cmdutil.ParseTypedList[nvfleetint.NodeVerificationCheck](
		"verification-check", flags.verificationCheck); err != nil {
		return opts, err
	}
	if opts.FirmwareChecks, err = cmdutil.ParseTypedList[nvfleetint.NodeFirmwareCheck](
		"firmware-check", flags.firmwareCheck); err != nil {
		return opts, err
	}

	for _, list := range []struct {
		name string
		raw  string
		dest *[]string
	}{
		{name: "node-uuids", raw: flags.nodeUUIDs, dest: &opts.NodeUUIDs},
		{name: "compute-zone-ids", raw: flags.computeZoneIDs, dest: &opts.ComputeZoneIDs},
		{name: "compute-zone-names", raw: flags.computeZoneNames, dest: &opts.ComputeZoneNames},
		{name: "nodegroup-ids", raw: flags.nodeGroupIDs, dest: &opts.NodeGroupIDs},
		{name: "nodegroup-names", raw: flags.nodeGroupNames, dest: &opts.NodeGroupNames},
		{name: "gpu-type", raw: flags.gpuType, dest: &opts.GPUTypes},
		{name: "public-ip", raw: flags.publicIP, dest: &opts.PublicIPs},
		{name: "private-ip", raw: flags.privateIP, dest: &opts.PrivateIPs},
	} {
		values, err := cmdutil.ParseCommaList(list.raw)
		if err != nil {
			return opts, fmt.Errorf("invalid %s: %w", list.name, err)
		}
		*list.dest = values
	}

	// GPU counts are the one filter the SDK cannot check on its own: it takes
	// integers, so a non-numeric value has to be rejected here.
	gpuCounts, err := cmdutil.ParseIntList("gpu-count", flags.gpuCount)
	if err != nil {
		return opts, err
	}
	opts.GPUCounts = gpuCounts

	if err := opts.Validate(); err != nil {
		return opts, cmdutil.RenderOptionError(err, nodeListFlagNames)
	}

	return opts, nil
}

// Writes JSON or table output for node list results
func writeNodeListOutput(w io.Writer, common cmdutil.Resolved, result nodeListOutput) error {
	if common.Output == clioutput.FormatJSON {
		return cmdutil.WritePaginatedListJSON(w, result.RawJSON, result.JSONValue)
	}

	if err := writeNodeTable(w, result.View, result.AgentType, result.Nodes); err != nil {
		return err
	}
	if result.Page == nil {
		return nil
	}
	return clioutput.WritePaginationFooter(w, *result.Page)
}

func writeNodeListResult(w io.Writer, common cmdutil.Resolved, result nodeListResult) error {
	if common.Output == clioutput.FormatJSON {
		return clioutput.WriteJSON(w, combinedNodeListJSON{
			Inband:      nodeListJSONValue(result.Inband),
			OOB:         nodeListJSONValue(result.OOB),
			InbandError: result.InbandSkipReason,
			OOBError:    result.OOBSkipReason,
		})
	}

	wrote := false
	switch {
	case result.Inband != nil:
		if err := writeNodeListSection(w, "In-band", *result.Inband, wrote); err != nil {
			return err
		}
		wrote = true
	case result.InbandSkipReason != "":
		if err := writeNodeListSkipNote(w, "In-band", result.InbandSkipReason, wrote); err != nil {
			return err
		}
		wrote = true
	}
	switch {
	case result.OOB != nil:
		if err := writeNodeListSection(w, "Out-of-band", *result.OOB, wrote); err != nil {
			return err
		}
	case result.OOBSkipReason != "":
		if err := writeNodeListSkipNote(w, "Out-of-band", result.OOBSkipReason, wrote); err != nil {
			return err
		}
	}
	return nil
}

func writeNodeListSection(w io.Writer, title string, result nodeListOutput, leadingNewline bool) error {
	if leadingNewline {
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w, title); err != nil {
		return err
	}
	if err := writeNodeTable(w, result.View, result.AgentType, result.Nodes); err != nil {
		return err
	}
	if result.Page != nil {
		return clioutput.WritePaginationFooter(w, *result.Page)
	}
	return nil
}

// writeNodeListSkipNote reports why a view was left out of a combined node
// list result instead of silently omitting it, so a filter/sort-by mismatch
// with one agent type doesn't look like that view simply had no nodes.
func writeNodeListSkipNote(w io.Writer, title, reason string, leadingNewline bool) error {
	if leadingNewline {
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(w, "%s: skipped (%s)\n", title, reason)
	return err
}

func nodeListJSONValue(result *nodeListOutput) any {
	if result == nil {
		return nil
	}
	if result.RawJSON != nil {
		return json.RawMessage(cmdutil.OneIndexRawPage(result.RawJSON))
	}
	if merged, ok := result.JSONValue.(cmdutil.MergedJSONResult); ok {
		merged.Pagination.Page++
		return merged
	}
	return result.JSONValue
}

// Renders nodes using the selected view columns
func writeNodeTable(
	w io.Writer,
	view string,
	agentType nvfleetint.NodeAgentType,
	nodes []nvfleetint.Node,
) error {
	if nvfleetint.NodeView(view) == nvfleetint.NodeViewBasic {
		return clioutput.WriteTable(
			w,
			[]string{"UUID", "HOSTNAME", "NODE NAME", "BMC HOSTNAME", "BMC IP"},
			basicNodeRows(nodes),
		)
	}
	if agentType == nvfleetint.NodeAgentTypeOOB || nodeListIsOOB(nodes) {
		return clioutput.WriteTable(
			w,
			[]string{"UUID", "NODE NAME", "BMC HOSTNAME", "BMC IP", "COMPUTE ZONE", "NODE GROUP", "HEALTH", "VERIFICATION CHECK", "AGENT STATUS"},
			oobDetailNodeRows(nodes),
		)
	}
	return clioutput.WriteTable(w, []string{"UUID", "HOSTNAME", "COMPUTE ZONE", "NODE GROUP", "HEALTH", "GPU TYPE", "GPU COUNT", "VERIFICATION CHECK", "FIRMWARE CHECK", "AGENT STATUS"}, detailNodeRows(nodes))
}

// Reports whether a node list contains OOB-view records
func nodeListIsOOB(nodes []nvfleetint.Node) bool {
	for _, node := range nodes {
		if node.AgentType == string(nvfleetint.NodeAgentTypeOOB) || node.NodeName != "" || node.BMCHostname != "" || node.BMCIP != "" {
			return true
		}
	}
	return false
}

// Converts nodes into basic table rows
func basicNodeRows(nodes []nvfleetint.Node) [][]string {
	rows := make([][]string, 0, len(nodes))
	for _, node := range nodes {
		rows = append(rows, []string{
			clioutput.DisplayString(node.UUID),
			clioutput.DisplayString(node.Hostname),
			clioutput.DisplayString(node.NodeName),
			clioutput.DisplayString(node.BMCHostname),
			clioutput.DisplayString(node.BMCIP),
		})
	}
	return rows
}

// Converts nodes into detail table rows
func detailNodeRows(nodes []nvfleetint.Node) [][]string {
	rows := make([][]string, 0, len(nodes))
	for _, node := range nodes {
		rows = append(rows, []string{
			clioutput.DisplayString(node.UUID),
			clioutput.DisplayString(node.Hostname),
			clioutput.DisplayString(node.ComputeZone),
			clioutput.DisplayString(node.NodeGroup),
			clioutput.DisplayString(node.Health),
			clioutput.DisplayString(node.GPUType),
			clioutput.FormatOptionalInt(node.GPUCount),
			clioutput.DisplayString(node.VerificationCheck),
			clioutput.DisplayString(node.FirmwareCheck),
			clioutput.DisplayString(node.AgentStatus),
		})
	}
	return rows
}

// Converts OOB nodes into detail table rows
func oobDetailNodeRows(nodes []nvfleetint.Node) [][]string {
	rows := make([][]string, 0, len(nodes))
	for _, node := range nodes {
		rows = append(rows, []string{
			clioutput.DisplayString(node.UUID),
			clioutput.DisplayString(node.NodeName),
			clioutput.DisplayString(node.BMCHostname),
			clioutput.DisplayString(node.BMCIP),
			clioutput.DisplayString(node.ComputeZone),
			clioutput.DisplayString(node.NodeGroup),
			clioutput.DisplayString(node.Health),
			clioutput.DisplayString(node.VerificationCheck),
			clioutput.DisplayString(node.AgentStatus),
		})
	}
	return rows
}

// Renders node detail fields as a table
func writeNodeDescribeResultTable(
	w io.Writer,
	result nodeDescribeResult,
	agentType nvfleetint.NodeAgentType,
	options nodeDescribeRenderOptions,
) error {
	if agentType == nvfleetint.NodeAgentTypeInband && result.Inband != nil {
		return writeNodeDescribeTable(w, *result.Inband, options)
	}
	if agentType == nvfleetint.NodeAgentTypeOOB && result.OOB != nil {
		return writeNodeDescribeTable(w, *result.OOB, options)
	}

	if !options.showSummary {
		return writeOOBInventoryTables(w, result.OOB.OOBInventory, options)
	}

	wroteSection := false
	if result.Inband != nil {
		if err := writeNodeViewSection(w, "In-band", *result.Inband, wroteSection); err != nil {
			return err
		}
		wroteSection = true
	}
	if result.OOB != nil {
		if err := writeNodeViewSection(w, "Out-of-band", *result.OOB, wroteSection); err != nil {
			return err
		}
		if result.OOB.OOBInventory != nil {
			return writeOOBInventoryTables(w, result.OOB.OOBInventory, options)
		}
	}
	return nil
}

func writeNodeViewSection(
	w io.Writer,
	title string,
	node nvfleetint.NodeDetails,
	leadingNewline bool,
) error {
	if leadingNewline {
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w, title); err != nil {
		return err
	}
	return clioutput.WriteTable(w, []string{"FIELD", "VALUE"}, nodeDescribeSummaryRows(node))
}

func writeNodeDescribeTable(
	w io.Writer,
	node nvfleetint.NodeDetails,
	options nodeDescribeRenderOptions,
) error {
	if options.showSummary {
		if err := clioutput.WriteTable(w, []string{"FIELD", "VALUE"}, nodeDescribeSummaryRows(node)); err != nil {
			return err
		}
	}
	if node.OOBInventory == nil {
		return nil
	}
	return writeOOBInventoryTables(w, node.OOBInventory, options)
}

func nodeDescribeSummaryRows(node nvfleetint.NodeDetails) [][]string {
	rows := nodeDescribeRows(node)
	if node.AgentType == string(nvfleetint.NodeAgentTypeOOB) || node.OOBInventory != nil {
		rows = oobNodeDescribeRows(node)
	}
	if node.OOBInventory == nil {
		return rows
	}

	rows = append(rows, oobInventorySummaryRows(node.OOBInventory)...)
	if node.OOBInventory.Source != nil {
		rows = append(rows, oobSourceRows(node.OOBInventory.Source)...)
	}
	for index, domainError := range node.OOBInventory.DomainErrors {
		parts := make([]string, 0, 3)
		for _, value := range []string{domainError.Domain, domainError.Resource, domainError.Message} {
			if value = strings.TrimSpace(value); value != "" {
				parts = append(parts, value)
			}
		}
		rows = append(rows, []string{
			fmt.Sprintf("INVENTORY DOMAIN ERROR %d", index+1),
			clioutput.DisplayString(strings.Join(parts, ": ")),
		})
	}
	return rows
}

func oobNodeDescribeRows(node nvfleetint.NodeDetails) [][]string {
	return [][]string{
		{"UUID", clioutput.DisplayString(node.UUID)},
		{"HEALTH", clioutput.DisplayString(node.Health)},
		{"COMPUTE ZONE", clioutput.FormatNameAndID(node.ComputeZone, node.ComputeZoneID)},
		{"NODE GROUP", clioutput.FormatNameAndID(node.NodeGroup, node.NodeGroupID)},
		{"AGENT STATUS", clioutput.DisplayString(node.AgentStatus)},
		{"VERIFICATION CHECK", clioutput.DisplayString(node.VerificationCheck)},
		{"VERIFICATION REASON", clioutput.DisplayString(node.VerificationCheckReason)},
		{"TAGS", clioutput.FormatStringList(node.Tags)},
		{"ENROLLED AT", clioutput.DisplayString(node.EnrolledAt)},
		{"LAST UPDATED", clioutput.DisplayString(node.LastUpdatedTime)},
		{"LAST VERIFICATION CHECK", clioutput.DisplayString(node.LastVerificationCheckTime)},
		{"HEALTHY COMPONENTS", clioutput.FormatOptionalInt(node.HealthyComponentCount)},
		{"DEGRADED COMPONENTS", clioutput.FormatOptionalInt(node.DegradedComponentCount)},
		{"UNHEALTHY COMPONENTS", clioutput.FormatOptionalInt(node.UnhealthyComponentCount)},
		{"LOCATION", cmdutil.FormatLocation(node.Location)},
		{"NODE NAME", clioutput.DisplayString(node.NodeName)},
		{"BMC HOSTNAME", clioutput.DisplayString(node.BMCHostname)},
		{"BMC IP", clioutput.DisplayString(node.BMCIP)},
	}
}

// Converts node details into describe table rows
func nodeDescribeRows(node nvfleetint.NodeDetails) [][]string {
	rows := [][]string{
		{"UUID", clioutput.DisplayString(node.UUID)},
		{"HOSTNAME", clioutput.DisplayString(node.Hostname)},
		{"AGENT TYPE", clioutput.DisplayString(node.AgentType)},
		{"HEALTH", clioutput.DisplayString(node.Health)},
		{"COMPUTE ZONE", clioutput.FormatNameAndID(node.ComputeZone, node.ComputeZoneID)},
		{"NODE GROUP", clioutput.FormatNameAndID(node.NodeGroup, node.NodeGroupID)},
		{"GPU TYPE", clioutput.DisplayString(node.GPUType)},
		{"GPU COUNT", clioutput.FormatOptionalInt(node.GPUCount)},
		{"AGENT STATUS", clioutput.DisplayString(node.AgentStatus)},
		{"VERIFICATION CHECK", clioutput.DisplayString(node.VerificationCheck)},
		{"VERIFICATION REASON", clioutput.DisplayString(node.VerificationCheckReason)},
		{"FIRMWARE CHECK", clioutput.DisplayString(node.FirmwareCheck)},
		{"PUBLIC IP", clioutput.DisplayString(node.PublicIP)},
		{"PRIVATE IP", clioutput.DisplayString(node.PrivateIP)},
		{"TAGS", clioutput.FormatStringList(node.Tags)},
		{"ENROLLED AT", clioutput.DisplayString(node.EnrolledAt)},
		{"LAST UPDATED", clioutput.DisplayString(node.LastUpdatedTime)},
		{"LAST VERIFICATION CHECK", clioutput.DisplayString(node.LastVerificationCheckTime)},
		{"HEALTHY COMPONENTS", clioutput.FormatOptionalInt(node.HealthyComponentCount)},
		{"DEGRADED COMPONENTS", clioutput.FormatOptionalInt(node.DegradedComponentCount)},
		{"UNHEALTHY COMPONENTS", clioutput.FormatOptionalInt(node.UnhealthyComponentCount)},
		{"LOCATION", cmdutil.FormatLocation(node.Location)},
	}

	if node.BMCHostname != "" || node.BMCIP != "" {
		rows = append(rows,
			[]string{"BMC HOSTNAME", clioutput.DisplayString(node.BMCHostname)},
			[]string{"BMC IP", clioutput.DisplayString(node.BMCIP)},
		)
	}
	if node.Resources != nil {
		rows = append(rows, nodeResourceRows(node.Resources)...)
	}
	if node.SystemInfo != nil {
		rows = append(rows, systemInfoRows(node.SystemInfo)...)
	}
	return rows
}

// Renders OOB inventory using the same domain sections as the collector detail view
func writeOOBInventoryTables(
	w io.Writer,
	inventory *nvfleetint.OOBInventory,
	options nodeDescribeRenderOptions,
) error {
	if options.sections[oobInventorySectionManagers] {
		if err := writeOOBSection(w, "Managers",
			[]string{"ID", "UUID", "MODEL", "TYPE", "FIRMWARE", "STATUS", "HEALTH", "ROLLUP"},
			oobManagerRows(inventory.Managers)); err != nil {
			return err
		}
	}
	if options.sections[oobInventorySectionSystems] {
		if err := writeOOBSection(w, "Systems",
			[]string{
				"PRIMARY", "ID", "UUID", "MANUFACTURER", "MODEL", "SKU", "SERIAL", "BIOS",
				"HOSTNAME", "ASSET", "POWER", "STATUS", "HEALTH", "ROLLUP", "MEMORY GIB", "SECURE BOOT",
			},
			oobSystemRows(inventory)); err != nil {
			return err
		}
		cpuRows, gpuRows := oobProcessorRows(inventory.Systems)
		if len(cpuRows) > 0 {
			if err := writeOOBSection(w, "CPUs",
				[]string{
					"SYSTEM", "ID", "SOCKET", "ARCH", "MANUFACTURER", "MODEL",
					"MAX MHZ", "CORES", "THREADS", "STATUS", "HEALTH", "ROLLUP",
				},
				cpuRows); err != nil {
				return err
			}
		}
		if len(gpuRows) > 0 {
			if err := writeOOBSection(w, "GPUs",
				[]string{"SYSTEM", "ID", "MANUFACTURER", "MODEL", "MAX MHZ", "STATUS", "HEALTH", "ROLLUP"},
				gpuRows); err != nil {
				return err
			}
		}
	}
	if options.sections[oobInventorySectionChassis] {
		if err := writeOOBSection(w, "Chassis",
			[]string{
				"ID", "TYPE", "MANUFACTURER", "MODEL", "SKU", "SERIAL", "PART",
				"ASSET", "POWER", "STATUS", "HEALTH", "ROLLUP", "LOCATION",
			},
			oobChassisRows(inventory.Chassis)); err != nil {
			return err
		}
		pcieRows := oobPCIeDeviceRows(inventory.Chassis)
		if len(pcieRows) > 0 {
			if err := writeOOBSection(w, "PCIe Devices",
				[]string{
					"CHASSIS", "ID", "UUID", "TYPE", "MANUFACTURER", "MODEL",
					"SKU", "SERIAL", "PART", "FIRMWARE", "STATUS", "HEALTH", "ROLLUP",
				},
				pcieRows); err != nil {
				return err
			}
		}
	}
	if options.sections[oobInventorySectionFirmware] {
		if err := writeOOBSection(w, "Firmware",
			[]string{"ID", "NAME", "VERSION", "RELEASE DATE", "STATUS", "HEALTH", "ROLLUP"},
			oobFirmwareRows(inventory.Firmware)); err != nil {
			return err
		}
	}
	if !options.showSummary && len(inventory.DomainErrors) > 0 {
		return writeOOBSection(w, "Domain Errors", []string{"DOMAIN", "RESOURCE", "MESSAGE"}, oobDomainErrorRows(inventory.DomainErrors))
	}
	return nil
}

func writeOOBSection(w io.Writer, title string, headers []string, rows [][]string) error {
	if _, err := fmt.Fprintf(w, "\n%s\n", title); err != nil {
		return err
	}
	return clioutput.WriteTable(w, headers, rows)
}

func oobInventorySummaryRows(inventory *nvfleetint.OOBInventory) [][]string {
	return [][]string{
		{"INVENTORY SCHEMA VERSION", clioutput.DisplayString(inventory.SchemaVersion)},
		{"INVENTORY COLLECTED AT", clioutput.DisplayString(inventory.CollectedAt)},
		{"INVENTORY PRIMARY SYSTEM", clioutput.DisplayString(inventory.PrimarySystemID)},
		{"INVENTORY MANAGERS", strconv.Itoa(len(inventory.Managers))},
		{"INVENTORY SYSTEMS", strconv.Itoa(len(inventory.Systems))},
		{"INVENTORY CHASSIS", strconv.Itoa(len(inventory.Chassis))},
		{"INVENTORY FIRMWARE", strconv.Itoa(len(inventory.Firmware))},
		{"INVENTORY DOMAIN ERRORS", strconv.Itoa(len(inventory.DomainErrors))},
		{"INVENTORY TARGET ERROR", clioutput.DisplayString(inventory.TargetError)},
	}
}

func oobSourceRows(source *nvfleetint.OOBSource) [][]string {
	return [][]string{
		{"SOURCE TYPE", clioutput.DisplayString(source.SourceType)},
		{"SOURCE MAC", clioutput.DisplayString(source.MAC)},
		{"SOURCE ADDRESS", clioutput.DisplayString(source.Address)},
		{"SOURCE HOSTNAME", clioutput.DisplayString(source.Hostname)},
		{"SOURCE REDFISH VERSION", clioutput.DisplayString(source.RedfishVersion)},
		{"SOURCE SERVICE UUID", clioutput.DisplayString(source.ServiceUUID)},
		{"SOURCE VENDOR", clioutput.DisplayString(source.Vendor)},
	}
}

func oobManagerRows(managers []nvfleetint.OOBManager) [][]string {
	rows := make([][]string, 0, len(managers))
	for _, manager := range managers {
		rows = append(rows, []string{
			manager.ID, manager.UUID, manager.Model, manager.ManagerType, manager.FirmwareVersion,
			manager.StatusState, manager.Health, manager.HealthRollup,
		})
	}
	return rows
}

func oobSystemRows(inventory *nvfleetint.OOBInventory) [][]string {
	rows := make([][]string, 0, len(inventory.Systems))
	for _, system := range inventory.Systems {
		rows = append(rows, []string{
			strconv.FormatBool(system.ID == inventory.PrimarySystemID),
			system.ID,
			system.UUID,
			system.Manufacturer,
			system.Model,
			system.SKU,
			system.SerialNumber,
			system.BIOSVersion,
			system.Hostname,
			system.AssetTag,
			system.PowerState,
			system.StatusState,
			system.Health,
			system.HealthRollup,
			formatOOBFloat32(system.MemoryGiB),
			formatOOBBool(system.SecureBootEnabled),
		})
	}
	return rows
}

func oobProcessorRows(systems []nvfleetint.OOBSystem) ([][]string, [][]string) {
	cpuRows := [][]string{}
	gpuRows := [][]string{}
	for _, system := range systems {
		for _, processor := range system.Processors {
			if strings.EqualFold(strings.TrimSpace(processor.ProcessorType), "gpu") {
				gpuRows = append(gpuRows, []string{
					system.ID, processor.ID, processor.Manufacturer, processor.Model,
					formatOOBInt(processor.MaxSpeedMHz), processor.StatusState,
					processor.Health, processor.HealthRollup,
				})
				continue
			}
			cpuRows = append(cpuRows, []string{
				system.ID, processor.ID, processor.Socket, processor.ProcessorArchitecture,
				processor.Manufacturer, processor.Model,
				formatOOBInt(processor.MaxSpeedMHz), formatOOBInt(processor.TotalCores),
				formatOOBInt(processor.TotalThreads), processor.StatusState,
				processor.Health, processor.HealthRollup,
			})
		}
	}
	return cpuRows, gpuRows
}

func oobChassisRows(chassis []nvfleetint.OOBChassis) [][]string {
	rows := make([][]string, 0, len(chassis))
	for _, item := range chassis {
		rows = append(rows, []string{
			item.ID, item.ChassisType, item.Manufacturer, item.Model, item.SKU,
			item.SerialNumber, item.PartNumber, item.AssetTag, item.PowerState,
			item.StatusState, item.Health, item.HealthRollup, formatOOBLocation(item.Location),
		})
	}
	return rows
}

func oobPCIeDeviceRows(chassis []nvfleetint.OOBChassis) [][]string {
	rows := [][]string{}
	for _, item := range chassis {
		for _, device := range item.PCIeDevices {
			rows = append(rows, []string{
				item.ID, device.ID, device.UUID, device.DeviceType, device.Manufacturer,
				device.Model, device.SKU, device.SerialNumber, device.PartNumber,
				device.FirmwareVersion, device.StatusState, device.Health, device.HealthRollup,
			})
		}
	}
	return rows
}

func oobFirmwareRows(firmware []nvfleetint.OOBFirmware) [][]string {
	rows := make([][]string, 0, len(firmware))
	for _, item := range firmware {
		rows = append(rows, []string{
			item.ID, item.Name, item.Version, item.ReleaseDate,
			item.StatusState, item.Health, item.HealthRollup,
		})
	}
	return rows
}

func oobDomainErrorRows(domainErrors []nvfleetint.OOBDomainError) [][]string {
	rows := make([][]string, 0, len(domainErrors))
	for _, domainError := range domainErrors {
		rows = append(rows, []string{domainError.Domain, domainError.Resource, domainError.Message})
	}
	return rows
}

func formatOOBLocation(location *nvfleetint.OOBChassisLocation) string {
	if location == nil {
		return ""
	}
	parts := []string{}
	if location.ServiceLabel != "" {
		parts = append(parts, location.ServiceLabel)
	}
	if location.Rack != "" {
		rack := "rack " + location.Rack
		if location.RackOffset != nil {
			rack += fmt.Sprintf(" U%d", *location.RackOffset)
		}
		parts = append(parts, rack)
	} else if location.RackOffset != nil {
		parts = append(parts, fmt.Sprintf("U%d", *location.RackOffset))
	}
	if location.Row != "" {
		parts = append(parts, "row "+location.Row)
	}
	if location.Room != "" {
		parts = append(parts, "room "+location.Room)
	}
	return strings.Join(parts, ", ")
}

func formatOOBInt(value *int) string {
	if value == nil {
		return ""
	}
	return strconv.Itoa(*value)
}

func formatOOBFloat32(value *float32) string {
	if value == nil {
		return ""
	}
	return strconv.FormatFloat(float64(*value), 'f', -1, 32)
}

func formatOOBBool(value *bool) string {
	if value == nil {
		return ""
	}
	return strconv.FormatBool(*value)
}

// Converts optional node resources into describe table rows
func nodeResourceRows(resources *nvfleetint.NodeResources) [][]string {
	rows := [][]string{}
	if resources.CPUInfo != nil {
		rows = append(rows,
			[]string{"CPU TYPE", clioutput.DisplayString(resources.CPUInfo.Type)},
			[]string{"CPU MANUFACTURER", clioutput.DisplayString(resources.CPUInfo.Manufacturer)},
			[]string{"CPU ARCHITECTURE", clioutput.DisplayString(resources.CPUInfo.Architecture)},
			[]string{"CPU LOGICAL CORES", clioutput.DisplayString(resources.CPUInfo.LogicalCores)},
		)
	}
	if resources.GPUInfo != nil {
		rows = append(rows,
			[]string{"GPU PRODUCT", clioutput.DisplayString(resources.GPUInfo.Product)},
			[]string{"GPU MANUFACTURER", clioutput.DisplayString(resources.GPUInfo.Manufacturer)},
			[]string{"GPU ARCHITECTURE", clioutput.DisplayString(resources.GPUInfo.Architecture)},
			[]string{"GPU MEMORY", clioutput.DisplayString(resources.GPUInfo.Memory)},
			[]string{"GPU DEVICES", fmt.Sprintf("%d", len(resources.GPUInfo.GPUs))},
		)
	}
	if resources.MemoryInfo != nil {
		rows = append(rows, []string{"MEMORY TOTAL BYTES", clioutput.DisplayString(resources.MemoryInfo.TotalBytes)})
	}
	if resources.DiskInfo != nil {
		rows = append(rows,
			[]string{"CONTAINER ROOT DISK", clioutput.DisplayString(resources.DiskInfo.ContainerRootDisk)},
			[]string{"DISK DEVICES", fmt.Sprintf("%d", len(resources.DiskInfo.BlockDevices))},
		)
	}
	if resources.NICInfo != nil {
		rows = append(rows, []string{"NIC INTERFACES", fmt.Sprintf("%d", len(resources.NICInfo.PrivateIPInterfaces))})
	}
	return rows
}

// Converts system details into describe table rows
func systemInfoRows(system *nvfleetint.SystemInfo) [][]string {
	return [][]string{
		{"OS", clioutput.DisplayString(system.OperatingSystem)},
		{"OS IMAGE", clioutput.DisplayString(system.OSImage)},
		{"KERNEL", clioutput.DisplayString(system.KernelVersion)},
		{"GPU DRIVER", clioutput.DisplayString(system.GPUDriverVersion)},
		{"CUDA", clioutput.DisplayString(system.CUDAVersion)},
		{"DCGM", clioutput.DisplayString(system.DCGMVersion)},
		{"AGENT VERSION", clioutput.DisplayString(system.AgentVersion)},
		{"CONTAINER RUNTIME", clioutput.DisplayString(system.ContainerRuntimeVersion)},
		{"SYSTEM UUID", clioutput.DisplayString(system.SystemUUID)},
		{"BOOT ID", clioutput.DisplayString(system.BootID)},
		{"STARTED AT", clioutput.DisplayString(system.StartedAt)},
	}
}

// Stores local flag values for node options.
type nodeOptionsFlags struct {
	agentType string
}

// Maps each filter field returned by the node options endpoint to the flag on
// `node list` that consumes it. The CLI spells three sort fields differently
// from the backend, and a sort field the CLI's allowlist does not take is
// reported as unusable rather than offered.
var nodeOptionsRenderer = cmdutil.OptionsRenderer{
	Consumers: []string{"node list"},
	Filters: map[string]cmdutil.OptionFlag{
		"computeZones":   {Name: "--compute-zone-ids", Promote: "--nodegroup-ids"},
		"nodeGroups":     {Name: "--nodegroup-ids"},
		"gpuTypes":       {Name: "--gpu-type"},
		"healthStatuses": {Name: "--health"},
		"agentStatuses":  {Name: "--agent-status"},
	},
	SortFields: map[string]string{
		"nodeGroup":   "nodegroup",
		"computeZone": "computezone",
	},
	SortAccepted: func(field string) bool {
		return nvfleetint.NodeSortBy(field).Valid()
	},
	SortHidden: func(field string) bool {
		return nodeDeprecatedSortFields[field]
	},
}

// Sort fields the backend still advertises but has deprecated and will drop.
// `node list --sort-by` keeps taking them, so scripts written against them do
// not break before the API breaks them, but `node options` stops offering them.
// Delete this once the API no longer returns them.
var nodeDeprecatedSortFields = map[string]bool{
	"integrityCheck": true,
}

// Creates the node options command
func newNodeOptionsCmd() *cobra.Command {
	flags := nodeOptionsFlags{}
	common := cmdutil.NewCommon()
	cmd := &cobra.Command{
		Use:   "options",
		Short: "List available node filters and sorting options",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runNodeOptions(cmd, flags, cmdutil.ResolveCommon(cmd, common))
		},
	}

	cmd.Flags().StringVar(&flags.agentType, "agent-type", "", "Agent type view: inband or oob; oob returns a reduced set")
	cmdutil.RegisterReadFlags(cmd, common)

	return cmd
}

// Gets and renders the filters and sorting choices available for node queries.
func runNodeOptions(cmd *cobra.Command, flags nodeOptionsFlags, common cmdutil.Resolved) error {
	if err := cmdutil.ValidateReadFlags(common); err != nil {
		return err
	}
	agentType := nvfleetint.NodeAgentType(strings.TrimSpace(flags.agentType))
	if agentType != "" && !agentType.Valid() {
		return fmt.Errorf("invalid agent-type %q: expected inband or oob", flags.agentType)
	}

	client, err := cmdutil.New(common)
	if err != nil {
		return err
	}
	options, err := client.GetNodeFilterOptions(cmd.Context(), agentType)
	if err != nil {
		return err
	}
	if common.Output == clioutput.FormatJSON {
		return clioutput.WriteRawJSON(cmd.OutOrStdout(), options.RawJSON)
	}
	return nodeOptionsRenderer.Write(cmd.OutOrStdout(), options)
}
