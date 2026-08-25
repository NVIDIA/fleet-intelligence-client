// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package cmdutil

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
)

// Exit codes: 0 = success, 1 = general error, 77 = auth/permission failure.
const (
	exitError        = 1
	exitNoPermission = 77
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

// ExitCode returns 77 for API 401/403 responses and 1 for everything else.
func ExitCode(err error) int {
	var apiErr *nvfleetint.APIError
	if errors.As(err, &apiErr) && (apiErr.StatusCode == 401 || apiErr.StatusCode == 403) {
		return exitNoPermission
	}
	return exitError
}

// Write reports a command failure on out, as JSON when the arguments asked for
// JSON output and as a plain line otherwise. args is the raw command line
// because the failure can predate flag parsing.
func Write(out io.Writer, args []string, err error) {
	if WantsJSONOutput(args) {
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

	var apiErr *nvfleetint.APIError
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

// WantsJSONOutput reports whether the raw arguments select JSON output. It
// reads the command line directly so an error raised before or during flag
// parsing is still rendered in the format the user asked for.
func WantsJSONOutput(args []string) bool {
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
