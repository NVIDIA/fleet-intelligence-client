// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package cmdutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	clioutput "github.com/NVIDIA/fleet-intelligence-client/internal/output"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
)

// WriteRequestPreview renders what a write command would send for --dry-run:
// the HTTP method, URL, and request body it built, without sending it. Table
// output prints these plainly; -o json emits a structured preview document
// instead of a backend payload, since no request is made. The command exits 0
// either way.
func WriteRequestPreview(w io.Writer, common Resolved, preview nvfleetint.RequestPreview) error {
	if common.Output == clioutput.FormatJSON {
		return clioutput.WriteJSON(w, struct {
			Method string          `json:"method"`
			URL    string          `json:"url"`
			Body   json.RawMessage `json:"body,omitempty"`
		}{
			Method: preview.Method,
			URL:    preview.URL,
			Body:   json.RawMessage(preview.Body),
		})
	}

	body, err := prettyJSON(preview.Body)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(w, "%s %s\n\n%s\n\nDry run: no request was sent.\n", preview.Method, preview.URL, body)
	return err
}

// prettyJSON indents a JSON request body for readable table output.
func prettyJSON(body []byte) (string, error) {
	if len(body) == 0 {
		return "", nil
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, body, "", "  "); err != nil {
		return "", err
	}
	return buf.String(), nil
}
