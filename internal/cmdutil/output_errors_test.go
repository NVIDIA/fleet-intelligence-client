// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package cmdutil

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	clioutput "github.com/NVIDIA/fleet-intelligence-client/internal/output"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
)

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
