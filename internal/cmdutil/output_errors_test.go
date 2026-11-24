// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package cmdutil

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	clioutput "github.com/NVIDIA/fleet-intelligence-client/internal/output"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
)

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
