// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/NVIDIA/fleet-intelligence-client/pkg/fleetintelligence"
)

type errorOutput struct {
	Error errorDetails `json:"error"`
}

type errorDetails struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	StatusCode int    `json:"statusCode,omitempty"`
	Status     string `json:"status,omitempty"`
	Details    string `json:"details,omitempty"`
}

func main() {
	if err := execute(context.Background(), os.Args[1:]); err != nil {
		writeCLIError(os.Stderr, os.Args[1:], err)
		os.Exit(1)
	}
}

func writeCLIError(out io.Writer, args []string, err error) {
	if wantsJSONOutput(args) {
		if encodeErr := json.NewEncoder(out).Encode(newErrorOutput(err)); encodeErr == nil {
			return
		}
	}
	fmt.Fprintln(out, err)
}

func newErrorOutput(err error) errorOutput {
	details := errorDetails{
		Code:    "command_error",
		Message: err.Error(),
	}

	var apiErr *fleetintelligence.APIError
	if errors.As(err, &apiErr) {
		details.Code = "api_error"
		details.StatusCode = apiErr.StatusCode
		details.Status = apiErr.Status
		details.Details = apiErr.Details
		if strings.TrimSpace(apiErr.Message) != "" {
			details.Message = apiErr.Message
		}
	}

	return errorOutput{Error: details}
}

func wantsJSONOutput(args []string) bool {
	for i, arg := range args {
		switch {
		case arg == "--output" || arg == "-o":
			return i+1 < len(args) && args[i+1] == "json"
		case strings.HasPrefix(arg, "--output="):
			return strings.TrimPrefix(arg, "--output=") == "json"
		case strings.HasPrefix(arg, "-o="):
			return strings.TrimPrefix(arg, "-o=") == "json"
		case strings.HasPrefix(arg, "-o") && len(arg) > len("-o"):
			return strings.TrimPrefix(arg, "-o") == "json"
		}
	}
	return false
}
