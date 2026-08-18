// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"
	"strings"

	clioutput "github.com/NVIDIA/fleet-intelligence-client/internal/output"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
)

// Describes how one backend filter field is spelled on the command line.
type optionFlag struct {
	name string
	// promote names the flag that accepts this field's child options, if the
	// backend nests any (a compute zone nests its node groups). Those children
	// are lifted into a section of their own so that every section describes
	// exactly one flag; `/v1/alert_timeline/filter_options` already returns the
	// two lists flat, so this makes all four commands render alike.
	promote string
}

// States the two conventions once, so each section can be a bare flag name.
// Whether a flag matches an option's ID or its display value is load-bearing —
// the backend silently returns no rows for the wrong one — and an ID is always
// rendered as the left of two columns.
const optionsPreamble = "Values are comma-separated; where two columns are shown, pass the left one."

// Builds the command-level help note for flags whose values are discoverable
// from an options endpoint. Keeping this out of every individual flag avoids
// noisy repeated help text while still surfacing the discovery path.
func optionsHelpNote(command string, flags ...string) string {
	return fmt.Sprintf("Run '%s' to list accepted values for %s.", command, formatFlagList(flags))
}

func formatFlagList(flags []string) string {
	switch len(flags) {
	case 0:
		return "available filters and sorting options"
	case 1:
		return flags[0]
	case 2:
		return flags[0] + " and " + flags[1]
	default:
		return strings.Join(flags[:len(flags)-1], ", ") + ", and " + flags[len(flags)-1]
	}
}

// Renders a shared options envelope grouped by the flag that consumes each
// filter, so the output reads as a list of things the user can type rather
// than a dump of backend field names.
type optionsRenderer struct {
	// consumers are the commands whose flags these options describe.
	consumers []string
	// filters maps a backend filter field name to its flag. Fields missing from
	// the map render under their backend name with no flag hint, so an endpoint
	// that grows a new filter degrades to showing it rather than hiding it.
	filters map[string]optionFlag
	// sortFields renames backend sort fields the CLI spells differently.
	sortFields map[string]string
	// sortAccepted reports whether the CLI's --sort-by allowlist takes a field.
	// Backends return sort fields for views the CLI cannot request, and those
	// are called out rather than presented as usable.
	sortAccepted func(field string) bool
	// sortConsumers names the commands the endpoint's sorting block describes,
	// when that is narrower than consumers. An endpoint can advertise the
	// columns of one view while its filters apply to several.
	sortConsumers []string
	// staticSorting documents a consumer whose sort fields the endpoint does
	// not advertise at all, so the CLI prints its own allowlist rather than
	// leaving the user with another command's columns.
	staticSorting []staticSortSection
}

// Describes the --sort-by/--order a command accepts when the options endpoint
// does not advertise them. Values come from the CLI's own allowlist, which is
// generated from the same OpenAPI enums the command validates against.
type staticSortSection struct {
	consumer     string
	fields       []string
	defaultField string
	orders       []string
	defaultOrder string
}

// Groups the values one flag accepts under a heading that names it.
type optionSection struct {
	heading string
	rows    [][]string
}

// Writes the filter sections followed by the sorting flags.
func (renderer optionsRenderer) write(w io.Writer, options nvfleetint.FilterOptions) error {
	if _, err := fmt.Fprintf(w, "Filters for %s\n%s\n", renderer.consumerList(), optionsPreamble); err != nil {
		return err
	}
	// A field that owns a flag outright wins over promoting children into it, so
	// a backend that starts returning the child list as its own field does not
	// produce two sections for the same flag.
	claimed := make(map[string]bool, len(options.Filters.Fields))
	for _, field := range options.Filters.Fields {
		if flag, ok := renderer.filters[field.Name]; ok && flag.name != "" {
			claimed[flag.name] = true
		}
	}

	for _, field := range options.Filters.Fields {
		flag := renderer.filters[field.Name]
		nested := flag.promote != "" && hasNestedOptions(field.Options)
		promoting := nested && !claimed[flag.promote]

		// Nested children belong to another flag either way, so they are never
		// left indented under this field's heading, where they would read as
		// values this flag accepts.
		rows := optionRows(field.Options, 0)
		if nested {
			rows = flatOptionRows(field.Options)
		}
		if err := writeOptionSection(w, renderer.heading(field), rows); err != nil {
			return err
		}
		switch {
		case promoting:
			if err := writeOptionSection(w, flag.promote, promotedRows(field.Options)); err != nil {
				return err
			}
		case nested:
			// The claiming field already renders a section for that flag, so
			// point at it instead of printing the values twice.
			if _, err := fmt.Fprintf(w, "  Values nested under these are listed under %s.\n", flag.promote); err != nil {
				return err
			}
		}
	}
	return renderer.writeSorting(w, options.Sorting)
}

// Writes one filter section: a blank line, a heading naming the flag, then the
// values indented beneath it.
func writeOptionSection(w io.Writer, heading string, rows [][]string) error {
	if _, err := fmt.Fprintf(w, "\n%s\n", heading); err != nil {
		return err
	}
	if len(rows) == 0 {
		rows = [][]string{{"(none)"}}
	}
	return clioutput.WriteIndentedRows(w, "  ", rows)
}

// Builds a filter section heading naming the flag that accepts the field.
func (renderer optionsRenderer) heading(field nvfleetint.FilterField) string {
	flag, ok := renderer.filters[field.Name]
	if !ok || flag.name == "" {
		return fmt.Sprintf("%s  (no flag on %s)", field.Name, renderer.consumerList())
	}
	return flag.name
}

// Builds the rows of a promoted section: every descendant of the field's
// options, each tagged with the top-level option it belongs to.
func promotedRows(parents []nvfleetint.FilterOption) [][]string {
	var rows [][]string
	for _, parent := range parents {
		label := parent.Value
		if label == "" {
			label = parent.ID
		}
		for _, row := range optionRows(parent.Options, 0) {
			// An option without an ID yields one column; pad it so the
			// membership column lines up with the rows that have one.
			for len(row) < 2 {
				row = append(row, "")
			}
			rows = append(rows, append(row, fmt.Sprintf("(in %s)", label)))
		}
	}
	return rows
}

// Writes the sorting flags, separating fields the CLI accepts from those it
// does not.
func (renderer optionsRenderer) writeSorting(w io.Writer, sorting nvfleetint.SortingOptions) error {
	accepted := make([]string, 0, len(sorting.Fields))
	var rejected []string
	for _, field := range sorting.Fields {
		translated := renderer.translateSortField(field)
		if renderer.sortAccepted == nil || renderer.sortAccepted(translated) {
			accepted = append(accepted, translated)
			continue
		}
		rejected = append(rejected, translated)
	}

	sortConsumers := quotedList(renderer.sortConsumers, renderer.consumerList())
	if _, err := fmt.Fprintf(w, "\nSorting for %s\n", sortConsumers); err != nil {
		return err
	}
	sortByHeading := sortHeading("--sort-by", renderer.translateSortField(sorting.Defaults.Field))
	if err := writeOptionSection(w, sortByHeading, valueRows(accepted)); err != nil {
		return err
	}
	if err := writeOptionSection(w, sortHeading("--order", sorting.Defaults.Order), valueRows(sorting.Orders)); err != nil {
		return err
	}
	if len(rejected) > 0 {
		if _, err := fmt.Fprintf(w, "\n  Returned by the API but not accepted by %s --sort-by: %s\n",
			sortConsumers, strings.Join(rejected, ", ")); err != nil {
			return err
		}
	}
	for _, section := range renderer.staticSorting {
		if err := section.write(w); err != nil {
			return err
		}
	}
	return nil
}

// Writes the sorting flags of a consumer the endpoint says nothing about.
func (section staticSortSection) write(w io.Writer) error {
	if _, err := fmt.Fprintf(w, "\nSorting for '%s'\n", section.consumer); err != nil {
		return err
	}
	if err := writeOptionSection(w, sortHeading("--sort-by", section.defaultField), valueRows(section.fields)); err != nil {
		return err
	}
	return writeOptionSection(w, sortHeading("--order", section.defaultOrder), valueRows(section.orders))
}

// Builds a sorting section heading, noting the default the backend applies.
func sortHeading(flag, defaultValue string) string {
	if strings.TrimSpace(defaultValue) == "" {
		return flag
	}
	return fmt.Sprintf("%s  (default: %s)", flag, defaultValue)
}

// Applies the CLI's spelling for a backend sort field.
func (renderer optionsRenderer) translateSortField(field string) string {
	if translated, ok := renderer.sortFields[field]; ok {
		return translated
	}
	return field
}

// Renders the consuming commands as a quoted list.
func (renderer optionsRenderer) consumerList() string {
	return quotedList(renderer.consumers, "this command")
}

// Renders command names as a quoted list, falling back when there are none.
func quotedList(commands []string, fallback string) string {
	quoted := make([]string, 0, len(commands))
	for _, command := range commands {
		quoted = append(quoted, "'"+command+"'")
	}
	switch len(quoted) {
	case 0:
		return fallback
	case 1:
		return quoted[0]
	default:
		return strings.Join(quoted[:len(quoted)-1], ", ") + " and " + quoted[len(quoted)-1]
	}
}

// Converts only the top-level options into rows, for a field whose children are
// promoted into a section of their own.
func flatOptionRows(options []nvfleetint.FilterOption) [][]string {
	rows := make([][]string, 0, len(options))
	for _, option := range options {
		if option.ID == "" {
			rows = append(rows, []string{option.Value})
			continue
		}
		rows = append(rows, []string{option.ID, option.Value})
	}
	return rows
}

// Flattens options into rows, indenting each level of nesting. Used when a
// field nests children that no flag claims, so they are shown rather than lost.
func optionRows(options []nvfleetint.FilterOption, depth int) [][]string {
	rows := make([][]string, 0, len(options))
	indent := strings.Repeat("  ", depth)
	for _, option := range options {
		if option.ID == "" {
			rows = append(rows, []string{indent + option.Value})
		} else {
			rows = append(rows, []string{indent + option.ID, option.Value})
		}
		rows = append(rows, optionRows(option.Options, depth+1)...)
	}
	return rows
}

// Reports whether any option carries children.
func hasNestedOptions(options []nvfleetint.FilterOption) bool {
	for _, option := range options {
		if len(option.Options) > 0 {
			return true
		}
	}
	return false
}

// Converts strings into single-column rows.
func valueRows(values []string) [][]string {
	rows := make([][]string, 0, len(values))
	for _, value := range values {
		rows = append(rows, []string{value})
	}
	return rows
}
