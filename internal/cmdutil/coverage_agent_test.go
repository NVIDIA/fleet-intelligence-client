// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package cmdutil

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/config"
	clioutput "github.com/NVIDIA/fleet-intelligence-client/internal/output"
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

func TestCredentialWarningsLegacyKeyAndCurrentKey(t *testing.T) {
	const legacySecret = "legacy-test-secret"
	t.Setenv(config.EnvLegacyAPIKey, legacySecret)
	t.Setenv(config.EnvAPIKey, "")
	resolved := config.Resolved{
		ConfigError:           errors.New("configuration could not be read"),
		MissingCurrentProfile: "stale-profile",
	}
	want := []string{
		"configuration could not be read; continuing with credentials from the environment",
		"current profile \"stale-profile\" is no longer stored; run `nvfleetint auth use <name>`",
		"NVFLEETINT_SERVICE_KEY is set but no longer read; it was renamed to NVFLEETINT_API_KEY",
	}
	check := func(got, expected []string) {
		t.Helper()
		if len(got) != len(expected) {
			t.Errorf("warnings = %q, want %q", got, expected)
		}
		for i, warning := range got {
			if strings.Contains(warning, legacySecret) {
				t.Errorf("warning %d exposed legacy secret", i)
			}
			if i < len(expected) && warning != expected[i] {
				t.Errorf("warning %d = %q, want %q", i, warning, expected[i])
			}
		}
	}
	check(CredentialWarnings(resolved), want)
	t.Setenv(config.EnvAPIKey, "current-test-key")
	check(CredentialWarnings(resolved), want[:2])
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

type optionsFailingWriter struct {
	out         bytes.Buffer
	marker      string
	err         error
	encountered bool
}

func (w *optionsFailingWriter) Write(p []byte) (int, error) {
	if strings.Contains(w.out.String()+string(p), w.marker) {
		w.encountered = true
		return 0, w.err
	}
	return w.out.Write(p)
}

func TestOptionsRendererPropagatesWriteErrors(t *testing.T) {
	sentinel := errors.New("controlled options write failure")
	renderer := OptionsRenderer{
		Consumers: []string{"node list"},
		Filters: map[string]OptionFlag{
			"computeZones": {Name: "--compute-zone-ids", Promote: "--nodegroup-ids"},
			"nodeGroups":   {Name: "--nodegroup-ids"},
		},
		SortAccepted: func(field string) bool { return field != "rejected-field" },
	}
	nested := nvfleetint.FilterField{Name: "computeZones", Options: []nvfleetint.FilterOption{
		{ID: "zone-id", Options: []nvfleetint.FilterOption{{Value: "child-value"}}},
	}}
	for _, tc := range []struct {
		name, marker, later string
		claimed             bool
	}{
		{name: "preamble", marker: "Filters for 'node list'", later: "--compute-zone-ids"},
		{name: "filter heading", marker: "\n--compute-zone-ids\n", later: "--nodegroup-ids"},
		{name: "promoted heading", marker: "\n--nodegroup-ids\n", later: "Sorting for "},
		{name: "nested reference", marker: "Values nested under these are listed under --nodegroup-ids.", later: "Sorting for ", claimed: true},
		{name: "sorting heading", marker: "Sorting for 'node list'", later: "--sort-by"},
		{name: "sort-by heading", marker: "\n--sort-by\n", later: "--order"},
		{name: "order heading", marker: "\n--order\n", later: "Returned by the API"},
		{name: "rejected notice", marker: "Returned by the API but not accepted", later: "rejected-field"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fields := []nvfleetint.FilterField{nested}
			if tc.claimed {
				fields = append(fields, nvfleetint.FilterField{Name: "nodeGroups", Options: []nvfleetint.FilterOption{{Value: "separate-child"}}})
			}
			options := nvfleetint.FilterOptions{
				Filters: nvfleetint.Filters{Fields: fields},
				Sorting: nvfleetint.SortingOptions{Fields: []string{"hostname", "rejected-field"}, Orders: []string{"asc"}},
			}
			writer := &optionsFailingWriter{marker: tc.marker, err: sentinel}
			err := renderer.Write(writer, options)
			if !errors.Is(err, sentinel) {
				t.Errorf("Write error = %v, want sentinel", err)
			}
			if !writer.encountered {
				t.Errorf("marker %q was not encountered; output:\n%s", tc.marker, writer.out.String())
			}
			if strings.Contains(writer.out.String(), tc.later) {
				t.Errorf("later output %q emitted after failure:\n%s", tc.later, writer.out.String())
			}
		})
	}
}

type promptErrorReader struct {
	input string
	err   error
}

func (r *promptErrorReader) Read(p []byte) (int, error) {
	n := copy(p, r.input)
	r.input = r.input[n:]
	return n, r.err
}

func TestPromptsPropagateInputErrors(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input string
	}{
		{name: "no input"},
		{name: "partial answer", input: "yes"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sentinel := errors.New("controlled prompt read failure")
			t.Run("APIKey", func(t *testing.T) {
				var out bytes.Buffer
				prompt := NewCredentialPrompt(&promptErrorReader{input: tc.input, err: sentinel}, &out)
				key, set, err := prompt.APIKey(true)
				if !errors.Is(err, sentinel) || key != "" || set {
					t.Errorf("APIKey = (%q, %t, %v), want empty key, false, and sentinel error", key, set, err)
				}
			})
			t.Run("APIURL", func(t *testing.T) {
				var out bytes.Buffer
				prompt := NewCredentialPrompt(&promptErrorReader{input: tc.input, err: sentinel}, &out)
				url, err := prompt.APIURL("https://example.com")
				if !errors.Is(err, sentinel) || url != "" {
					t.Errorf("APIURL = (%q, %v), want empty URL and sentinel error", url, err)
				}
			})
			t.Run("Confirm", func(t *testing.T) {
				var out bytes.Buffer
				const summary = "Delete the selected resource"
				err := Confirm(&promptErrorReader{input: tc.input, err: sentinel}, &out, summary)
				if !errors.Is(err, sentinel) || errors.Is(err, ErrAborted) {
					t.Errorf("Confirm error = %v, want sentinel and not ErrAborted", err)
				}
				if !strings.Contains(out.String(), summary) || !strings.Contains(out.String(), "Are you sure? [y/N]: ") {
					t.Errorf("Confirm output missing summary or question: %q", out.String())
				}
			})
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

func TestWriteRawOutputFlags(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		json bool
	}{
		{name: "long equals json", args: []string{"--output=json"}, json: true},
		{name: "short equals json", args: []string{"-o=json"}, json: true},
		{name: "short attached json", args: []string{"-ojson"}, json: true},
		{name: "long equals table", args: []string{"--output=table"}},
		{name: "short equals table", args: []string{"-o=table"}},
		{name: "short attached table", args: []string{"-otable"}},
		{name: "unrelated", args: []string{"--unrelated"}},
		{name: "long missing value", args: []string{"--output"}},
		{name: "short missing value", args: []string{"-o"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			args := append([]string{"node"}, tc.args...)
			Write(&out, args, errors.New("bad input"))
			if !tc.json {
				if got := out.String(); got != "bad input\n" {
					t.Errorf("output = %q, want %q", got, "bad input\n")
				}
				return
			}
			var got struct {
				Error struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal(out.Bytes(), &got); err != nil {
				t.Fatalf("decode output %q: %v", out.String(), err)
			}
			if got.Error.Code != "command_error" || got.Error.Message != "bad input" {
				t.Errorf("error = %+v, want command_error and bad input", got.Error)
			}
		})
	}
}

type errorFirstWriteFails struct {
	out      bytes.Buffer
	attempts int
	err      error
}

func (w *errorFirstWriteFails) Write(p []byte) (int, error) {
	w.attempts++
	if w.attempts == 1 {
		return 0, w.err
	}
	return w.out.Write(p)
}

func TestWriteFallsBackAfterJSONWriteFailure(t *testing.T) {
	writer := &errorFirstWriteFails{err: errors.New("controlled write failure")}
	Write(writer, []string{"node", "--output=json"}, errors.New("bad input"))
	if writer.attempts != 2 {
		t.Errorf("write attempts = %d, want 2", writer.attempts)
	}
	if got := writer.out.String(); got != "bad input\n" {
		t.Errorf("accepted output = %q, want %q", got, "bad input\n")
	}
}

func TestWriteRequestPreviewBodyBoundaries(t *testing.T) {
	const method = "POST"
	const url = "https://example.invalid/v1/tags"
	const prefix = method + " " + url + "\n\n"
	const suffix = "\n\nDry run: no request was sent.\n"
	for _, tc := range []struct {
		name       string
		body       []byte
		wantOutput string
		invalid    bool
	}{
		{name: "nil body", wantOutput: prefix + suffix},
		{name: "empty body", body: []byte{}, wantOutput: prefix + suffix},
		{name: "malformed body", body: []byte(`{"tags":`), invalid: true},
		{name: "nested body", body: []byte(`{"tags":[{"name":"gpu","metadata":{"count":2}}]}`), wantOutput: prefix + "{\n  \"tags\": [\n    {\n      \"name\": \"gpu\",\n      \"metadata\": {\n        \"count\": 2\n      }\n    }\n  ]\n}" + suffix},
	} {
		t.Run("table/"+tc.name, func(t *testing.T) {
			var out bytes.Buffer
			err := WriteRequestPreview(&out, Resolved{Output: clioutput.FormatTable}, nvfleetint.RequestPreview{Method: method, URL: url, Body: tc.body})
			if tc.invalid {
				var syntaxErr *json.SyntaxError
				if !errors.As(err, &syntaxErr) {
					t.Errorf("error = %v, want JSON syntax error", err)
				}
			} else if err != nil {
				t.Errorf("WriteRequestPreview: %v", err)
			}
			if got := out.String(); got != tc.wantOutput {
				t.Errorf("output = %q, want %q", got, tc.wantOutput)
			}
		})
	}

	t.Run("table/write failure", func(t *testing.T) {
		sentinel := errors.New("controlled preview write failure")
		writer := &errorFirstWriteFails{err: sentinel}
		err := WriteRequestPreview(writer, Resolved{Output: clioutput.FormatTable}, nvfleetint.RequestPreview{Method: method, URL: url})
		if !errors.Is(err, sentinel) || writer.attempts != 1 {
			t.Errorf("error = %v, write attempts = %d; want sentinel and one attempt", err, writer.attempts)
		}
	})

	for _, tc := range []struct {
		name    string
		body    []byte
		invalid bool
	}{
		{name: "absent body"},
		{name: "malformed body", body: []byte(`{"tags":`), invalid: true},
	} {
		t.Run("json/"+tc.name, func(t *testing.T) {
			var out bytes.Buffer
			err := WriteRequestPreview(&out, Resolved{Output: clioutput.FormatJSON}, nvfleetint.RequestPreview{Method: method, URL: url, Body: tc.body})
			if tc.invalid {
				if err == nil || out.Len() != 0 {
					t.Errorf("error = %v, output = %q; want encoding error and no output", err, out.String())
				}
				return
			}
			if err != nil {
				t.Fatalf("WriteRequestPreview: %v", err)
			}
			var document map[string]json.RawMessage
			if err := json.Unmarshal(out.Bytes(), &document); err != nil {
				t.Fatalf("decode preview %q: %v", out.String(), err)
			}
			var gotMethod, gotURL string
			if err := json.Unmarshal(document["method"], &gotMethod); err != nil {
				t.Fatalf("decode method: %v", err)
			}
			if err := json.Unmarshal(document["url"], &gotURL); err != nil {
				t.Fatalf("decode URL: %v", err)
			}
			if _, exists := document["body"]; exists || gotMethod != method || gotURL != url {
				t.Errorf("document = %s; want method %q, URL %q, and no body", out.String(), method, url)
			}
		})
	}
}

type sourceTableFailingWriter struct {
	out      bytes.Buffer
	attempts int
	failAt   int
	err      error
}

func (w *sourceTableFailingWriter) Write(p []byte) (int, error) {
	w.attempts++
	if w.attempts == w.failAt {
		return 0, w.err
	}
	return w.out.Write(p)
}

func TestWriteBothSourceTablesPropagatesHeadingAndSeparatorErrors(t *testing.T) {
	for _, tc := range []struct {
		name       string
		failAt     int
		wantOutput string
		wantRender []string
	}{
		{name: "in-band heading", failAt: 1, wantOutput: ""},
		{name: "separator", failAt: 2, wantOutput: "In-band\n", wantRender: []string{"inband"}},
		{name: "out-of-band heading", failAt: 3, wantOutput: "In-band\n\n", wantRender: []string{"inband"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sentinel := errors.New("controlled source table write failure")
			writer := &sourceTableFailingWriter{failAt: tc.failAt, err: sentinel}
			var rendered []string
			err := WriteBothSourceTables(context.Background(), writer, "nodes",
				func(_ context.Context, source nvfleetint.DataSource) (string, error) {
					return string(source), nil
				},
				func(_ io.Writer, result string) error {
					rendered = append(rendered, result)
					return nil
				},
			)
			if !errors.Is(err, sentinel) {
				t.Errorf("error = %v, want sentinel", err)
			}
			if got := writer.out.String(); got != tc.wantOutput {
				t.Errorf("output = %q, want %q", got, tc.wantOutput)
			}
			if writer.attempts != tc.failAt {
				t.Errorf("write attempts = %d, want %d", writer.attempts, tc.failAt)
			}
			if len(rendered) != len(tc.wantRender) {
				t.Fatalf("rendered = %q, want %q", rendered, tc.wantRender)
			}
			for i := range rendered {
				if rendered[i] != tc.wantRender[i] {
					t.Errorf("rendered = %q, want %q", rendered, tc.wantRender)
					break
				}
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

func TestFetchAllRawPagesRejectsMalformedItemsAfterValidPage(t *testing.T) {
	hasMore := true
	var requested []int
	result, err := FetchAllRawPages("items", 3, func(page int) (RawPage, error) {
		requested = append(requested, page)
		if page == 3 {
			return RawPage{RawJSON: []byte(`{"items":[{"id":1}]}`), Page: page, HasMore: &hasMore}, nil
		}
		return RawPage{RawJSON: []byte(`{"items":{}}`), Page: page, HasMore: &hasMore}, nil
	})
	if err == nil || !strings.Contains(err.Error(), "decode items items") {
		t.Errorf("FetchAllRawPages error = %v, want items decode error", err)
	}
	if result.Items != nil || result.Pagination != (MergedPagination{}) {
		t.Errorf("FetchAllRawPages result = %+v, want zero result", result)
	}
	if len(requested) != 2 || requested[0] != 3 || requested[1] != 4 {
		t.Errorf("requested pages = %v, want [3 4]", requested)
	}
}

func TestFetchAllRawPagesStopsAtLimitWithEmptyPages(t *testing.T) {
	hasMore := true
	const startPage = 3
	var requested []int
	result, err := FetchAllRawPages("items", startPage, func(page int) (RawPage, error) {
		requested = append(requested, page)
		return RawPage{RawJSON: []byte(`{"items":[]}`), Page: page, HasMore: &hasMore}, nil
	})
	if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("stopped after %d pages while fetching items", MaxPages)) {
		t.Errorf("FetchAllRawPages error = %v, want page limit error", err)
	}
	if result.Items != nil || result.Pagination != (MergedPagination{}) {
		t.Errorf("FetchAllRawPages result = %+v, want zero result", result)
	}
	if len(requested) != MaxPages {
		t.Errorf("requested %d pages, want %d", len(requested), MaxPages)
	}
	for i, page := range requested {
		if page != startPage+i {
			t.Errorf("requested page at index %d = %d, want %d", i, page, startPage+i)
			break
		}
	}
}

func TestOneIndexRawPagePreservesNonIntegerPage(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "string", body: `{ "page": "3", "items": [1], "total": 5 }`},
		{name: "object", body: `{ "page": {"number": 3}, "items": [1], "total": 5 }`},
		{name: "fractional number", body: `{ "page": 3.5, "items": [1], "total": 5 }`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := []byte(tc.body)
			if got := OneIndexRawPage(input); !bytes.Equal(got, input) {
				t.Errorf("OneIndexRawPage = %q, want unchanged %q", got, input)
			}
		})
	}
}
