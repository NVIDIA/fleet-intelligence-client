// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package cmdutil

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
)

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
