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

package fleetintelligence

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/NVIDIA/fleet-intelligence-client/internal/generated/fleetapi"
)

// TagListOptions represents request options for listing unique tags. At most one
// of NodeUUID, NodeGroupID, or ComputeZoneID may be set; Prefix may be combined
// with one resource filter.
type TagListOptions struct {
	Prefix        string
	NodeUUID      string
	NodeGroupID   string
	ComputeZoneID string
}

// TagList represents the unique customer tags with the raw backend payload
type TagList struct {
	Tags    []string `json:"tags,omitempty"`
	RawJSON []byte   `json:"-"`
}

// ListTags retrieves the unique customer tags, optionally filtered by prefix and
// a single node, node group, or compute zone.
func (c *Client) ListTags(ctx context.Context, opts TagListOptions) (TagList, error) {
	ctx, cancel := c.requestContext(ctx)
	defer cancel()

	opts.Prefix = strings.TrimSpace(opts.Prefix)
	opts.NodeUUID = strings.TrimSpace(opts.NodeUUID)
	opts.NodeGroupID = strings.TrimSpace(opts.NodeGroupID)
	opts.ComputeZoneID = strings.TrimSpace(opts.ComputeZoneID)

	if err := validateTagListOptions(opts); err != nil {
		return TagList{}, err
	}

	params := fleetapi.GetV1TagsParams{}
	if opts.Prefix != "" {
		params.Prefix = &opts.Prefix
	}
	if opts.NodeUUID != "" {
		params.NodeUUID = &opts.NodeUUID
	}
	if opts.NodeGroupID != "" {
		params.NodeGroupId = &opts.NodeGroupID
	}
	if opts.ComputeZoneID != "" {
		params.ComputeZoneId = &opts.ComputeZoneID
	}

	resp, err := c.api.GetV1TagsWithResponse(ctx, &params)
	if err != nil {
		return TagList{}, err
	}
	if resp.StatusCode() != http.StatusOK {
		return TagList{}, newAPIError(resp.StatusCode(), resp.Status(), resp.Body)
	}

	var data fleetapi.ModelsListTagsResponse
	if err := json.Unmarshal(resp.Body, &data); err != nil {
		return TagList{}, err
	}

	return TagList{
		Tags:    cloneStringSlice(data.Tags),
		RawJSON: append([]byte(nil), resp.Body...),
	}, nil
}

// Rejects more than one resource-scoped tag filter
func validateTagListOptions(opts TagListOptions) error {
	count := 0
	if opts.NodeUUID != "" {
		count++
	}
	if opts.NodeGroupID != "" {
		count++
	}
	if opts.ComputeZoneID != "" {
		count++
	}
	if count > 1 {
		return fmt.Errorf("at most one of node, node group, or compute zone filter may be provided")
	}
	return nil
}
