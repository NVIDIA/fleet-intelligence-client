// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"

	"github.com/NVIDIA/fleet-intelligence-client/internal/generated/fleetapi"
)

// RequestPreview describes the API request a dry-run would send.
type RequestPreview struct {
	Method string          `json:"method"`
	URL    string          `json:"url"`
	Body   json.RawMessage `json:"body,omitempty"`
}

// PreviewUpdateComputeZone returns the update request after applying the same
// read-modify-write merge as UpdateComputeZone, without sending the write. The
// API has no conditional-update mechanism, so a later update can overwrite
// concurrent changes made after this preview's read.
func (c *Client) PreviewUpdateComputeZone(ctx context.Context, opts UpdateComputeZoneOptions) (RequestPreview, error) {
	body, err := c.buildUpdateComputeZoneRequest(ctx, opts)
	if err != nil {
		return RequestPreview{}, err
	}
	data, err := json.Marshal(body)
	if err != nil {
		return RequestPreview{}, err
	}

	req, err := fleetapi.NewPutV1ComputezonesRequestWithBody(
		generatedServerURL(c.baseURL.String()),
		"application/json",
		bytes.NewReader(data),
	)
	if err != nil {
		return RequestPreview{}, err
	}

	return RequestPreview{
		Method: req.Method,
		URL:    req.URL.String(),
		Body:   json.RawMessage(data),
	}, nil
}

// Normalizes a base URL exactly the way fleetapi.NewClient does before the
// generated request builders resolve a path against it. Reusing that same
// string is what keeps a preview URL identical to the URL actually requested,
// including for base URLs carrying a path prefix, query, or fragment.
func generatedServerURL(server string) string {
	if strings.HasSuffix(server, "/") {
		return server
	}
	return server + "/"
}
