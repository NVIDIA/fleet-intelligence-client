// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
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

func main() {
	if err := execute(context.Background(), os.Args[1:]); err != nil {
		writeCLIError(os.Stderr, os.Args[1:], err)
		os.Exit(exitCodeFor(err))
	}
}

// exitCodeFor returns 77 for API 401/403 responses and 1 for everything else.
func exitCodeFor(err error) int {
	var apiErr *nvfleetint.APIError
	if errors.As(err, &apiErr) && (apiErr.StatusCode == 401 || apiErr.StatusCode == 403) {
		return exitNoPermission
	}
	return exitError
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
