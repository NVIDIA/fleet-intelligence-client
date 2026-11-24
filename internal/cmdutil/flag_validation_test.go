// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package cmdutil

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/config"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
)

func TestRenderOptionErrorMappingsAndPassthrough(t *testing.T) {
	original := &nvfleetint.InvalidOptionError{
		Option: "futureOption", Description: "SDK option", Value: "bad", Expected: "good",
	}
	wrapped := fmt.Errorf("validation: %w", original)
	unrelated := errors.New("unrelated error")
	noExpected := &nvfleetint.InvalidOptionError{
		Option: "futureOption", Description: "SDK option", Value: "bad",
	}
	for _, tc := range []struct {
		name        string
		input       error
		names       map[string]OptionFlagName
		passthrough bool
		wantOption  *nvfleetint.InvalidOptionError
		want        string
	}{
		{name: "unmapped", input: original, names: map[string]OptionFlagName{"other": {Flag: "--other"}}, passthrough: true, wantOption: original},
		{name: "wrapped unmapped", input: wrapped, names: map[string]OptionFlagName{"other": {Flag: "--other"}}, passthrough: true, wantOption: original},
		{name: "nil map", input: original, passthrough: true, wantOption: original},
		{name: "mapped SDK expected", input: original, names: map[string]OptionFlagName{"futureOption": {Flag: "--future"}}, want: `invalid --future "bad": expected good`},
		{name: "mapped override", input: original, names: map[string]OptionFlagName{"futureOption": {Flag: "--future", Expected: "better, best"}}, want: `invalid --future "bad": expected better, best`},
		{name: "mapped without expected", input: noExpected, names: map[string]OptionFlagName{"futureOption": {Flag: "--future"}}, want: `invalid --future "bad"`},
		{name: "unrelated", input: unrelated, names: map[string]OptionFlagName{"futureOption": {Flag: "--future"}}, passthrough: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := RenderOptionError(tc.input, tc.names)
			if tc.passthrough {
				if got != tc.input {
					t.Errorf("RenderOptionError returned %v, want exact input %v", got, tc.input)
				}
				if tc.wantOption != nil {
					var optionErr *nvfleetint.InvalidOptionError
					if !errors.As(got, &optionErr) || optionErr != tc.wantOption {
						t.Errorf("errors.As = %v, want original option error %v", optionErr, tc.wantOption)
					}
				}
				return
			}
			if got == nil || got.Error() != tc.want {
				t.Errorf("RenderOptionError = %v, want %q", got, tc.want)
			}
		})
	}
}

func TestFromResolvedPreservesMissingBaseURLError(t *testing.T) {
	client, err := FromResolved(config.Resolved{APIKey: "test-api-key"}, testCommonFlags(""))
	if client != nil || !errors.Is(err, nvfleetint.ErrMissingBaseURL) {
		t.Errorf("FromResolved = (%v, %v), want nil client and ErrMissingBaseURL", client, err)
	}
}

func TestFromResolvedInsecureURLGenericRemedy(t *testing.T) {
	const key = "test-api-key-secret"
	client, err := FromResolved(config.Resolved{
		APIKey: key, APIURL: "http://example.com", APIURLSource: config.SourceDefault,
	}, testCommonFlags(""))
	if client != nil || !errors.Is(err, nvfleetint.ErrInsecureBaseURL) {
		t.Fatalf("FromResolved = (%v, %v), want nil client and ErrInsecureBaseURL", client, err)
	}
	if !strings.Contains(err.Error(), "run `nvfleetint auth add <name>` and enter an https URL") {
		t.Errorf("error missing generic HTTPS remedy: %v", err)
	}
	if strings.Contains(err.Error(), key) {
		t.Error("error exposed the API key")
	}
}

func TestOptionsRendererPromotionOptionShapes(t *testing.T) {
	renderer := OptionsRenderer{Filters: map[string]OptionFlag{
		"computeZones": {Name: "--compute-zone-ids", Promote: "--nodegroup-ids"},
		"empty":        {Name: "--empty"},
		"flat":         {Name: "--flat", Promote: "--unused-promoted"},
	}}
	options := nvfleetint.FilterOptions{Filters: nvfleetint.Filters{Fields: []nvfleetint.FilterField{
		{Name: "computeZones", Options: []nvfleetint.FilterOption{
			{ID: "zone-id", Options: []nvfleetint.FilterOption{{Value: "child-value"}}},
			{Value: "display-only-zone"},
		}},
		{Name: "empty"},
		{Name: "flat", Options: []nvfleetint.FilterOption{{Value: "flat-value"}}},
	}}}
	var out bytes.Buffer
	if err := renderer.Write(&out, options); err != nil {
		t.Fatalf("render options: %v", err)
	}
	got := out.String()
	zoneStart := strings.Index(got, "\n--compute-zone-ids\n")
	childStart := strings.Index(got, "\n--nodegroup-ids\n")
	emptyStart := strings.Index(got, "\n--empty\n")
	flatStart := strings.Index(got, "\n--flat\n")
	sortStart := strings.Index(got, "\nSorting for ")
	if zoneStart < 0 || childStart <= zoneStart || emptyStart <= childStart || flatStart <= emptyStart || sortStart <= flatStart {
		t.Fatalf("missing or misplaced sections:\n%s", got)
	}
	zoneSection := got[zoneStart:childStart]
	for _, want := range []string{"zone-id", "display-only-zone"} {
		if !strings.Contains(zoneSection, want) {
			t.Errorf("compute-zone section missing %q:\n%s", want, zoneSection)
		}
	}
	if strings.Contains(zoneSection, "child-value") {
		t.Errorf("child advertised as compute-zone value:\n%s", zoneSection)
	}
	childSection := got[childStart:emptyStart]
	if !strings.Contains(childSection, "child-value") || !strings.Contains(childSection, "(in zone-id)") {
		t.Errorf("promoted child or membership missing:\n%s", childSection)
	}
	if !strings.Contains(got[emptyStart:flatStart], "(none)") {
		t.Errorf("empty section missing (none):\n%s", got)
	}
	if !strings.Contains(got[flatStart:sortStart], "flat-value") || strings.Contains(got, "--unused-promoted") {
		t.Errorf("flat field produced incorrect promotion:\n%s", got)
	}
}

func TestOptionsHelpNoteFlagCounts(t *testing.T) {
	for _, tc := range []struct {
		name  string
		flags []string
		want  string
	}{
		{name: "zero", want: "Run 'node options' to list accepted values for available filters and sorting options."},
		{name: "one", flags: []string{"--zone"}, want: "Run 'node options' to list accepted values for --zone."},
		{name: "two", flags: []string{"--zone", "--order"}, want: "Run 'node options' to list accepted values for --zone and --order."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := OptionsHelpNote("node options", tc.flags...); got != tc.want {
				t.Errorf("OptionsHelpNote = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAPIKeyTerminalPasswordReadFailure(t *testing.T) {
	for _, tc := range []struct {
		name       string
		keepStored bool
		wantOutput string
	}{
		{name: "new key", wantOutput: "API key: \n"},
		{name: "stored key", keepStored: true, wantOutput: "API key [press Enter to keep the stored key]: \n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file, err := os.CreateTemp(t.TempDir(), "prompt-input")
			if err != nil {
				t.Fatalf("create prompt input: %v", err)
			}
			t.Cleanup(func() { _ = file.Close() })

			var out bytes.Buffer
			prompt := NewCredentialPrompt(file, &out)
			prompt.terminal = true
			key, set, err := prompt.APIKey(tc.keepStored)
			if err == nil || errors.Is(err, io.EOF) || key != "" || set {
				t.Errorf("APIKey = (%q, %t, %v), want empty key, false, and non-EOF error", key, set, err)
			}
			if got := out.String(); got != tc.wantOutput {
				t.Errorf("output = %q, want %q", got, tc.wantOutput)
			}
		})
	}
}

func TestParseIntListMalformedMembers(t *testing.T) {
	for _, tc := range []struct {
		name    string
		raw     string
		invalid bool
		want    []int
	}{
		{name: "leading empty member", raw: ",8", invalid: true},
		{name: "trailing empty member", raw: "8,", invalid: true},
		{name: "consecutive separators", raw: "8,,4", invalid: true},
		{name: "whitespace member", raw: "8,  ,4", invalid: true},
		{name: "whitespace only", raw: "  \t "},
		{name: "valid values", raw: " 0, 8,4 ", want: []int{0, 8, 4}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseIntList("gpu-count", tc.raw)
			if tc.invalid {
				if got != nil {
					t.Errorf("ParseIntList returned %v, want nil", got)
				}
				if err == nil {
					t.Fatal("ParseIntList returned no error for malformed list")
				}
				for _, want := range []string{"invalid gpu-count", fmt.Sprintf("%q", tc.raw), "empty values are not allowed"} {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error %q does not contain %q", err, want)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseIntList returned error: %v", err)
			}
			if tc.want == nil && got != nil {
				t.Errorf("ParseIntList returned %v, want nil", got)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("ParseIntList returned %v, want %v", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Errorf("ParseIntList returned %v, want %v", got, tc.want)
					break
				}
			}
		})
	}
}

func TestAbsoluteTimeRangeFlagValidation(t *testing.T) {
	validators := []struct {
		name     string
		validate func(string, string, string) error
	}{
		{name: "required", validate: ValidateTimeRangeFlags},
		{name: "optional", validate: ValidateOptionalTimeRangeFlags},
	}
	cases := []struct {
		name, start, end, wantError string
	}{
		{name: "invalid start", start: "yesterday", end: "2026-05-02T00:00:00Z", wantError: "--start must be RFC3339"},
		{name: "both invalid checks start first", start: "yesterday", end: "tomorrow", wantError: "--start must be RFC3339"},
		{name: "invalid end", start: "2026-05-01T00:00:00Z", end: "tomorrow", wantError: "--end must be RFC3339"},
		{name: "padded valid endpoints", start: "  2026-05-01T00:00:00Z\t", end: "\t2026-05-02T00:00:00Z  "},
	}
	for _, validator := range validators {
		for _, tc := range cases {
			t.Run(validator.name+"/"+tc.name, func(t *testing.T) {
				err := validator.validate("", tc.start, tc.end)
				if tc.wantError == "" {
					if err != nil {
						t.Errorf("validation returned unexpected error: %v", err)
					}
				} else if err == nil || err.Error() != tc.wantError {
					t.Errorf("validation error = %v, want %q", err, tc.wantError)
				}
			})
		}
	}
	for _, tc := range []struct {
		name, window, start, end string
	}{
		{name: "empty"},
		{name: "whitespace", window: " \t", start: "\n ", end: " \r\n"},
	} {
		t.Run("optional/"+tc.name, func(t *testing.T) {
			if err := ValidateOptionalTimeRangeFlags(tc.window, tc.start, tc.end); err != nil {
				t.Errorf("optional validation returned unexpected error: %v", err)
			}
		})
	}
}
