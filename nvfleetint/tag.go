// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"bytes"
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

// MaxTagLength is the longest a single tag may be. It restates the backend's
// own limit so an over-long tag is named locally rather than returned as an
// opaque 400.
const MaxTagLength = 50

// reservedTagValues are the tags the backend refuses because they read as an
// absent or boolean value once a tag list is serialized.
var reservedTagValues = map[string]struct{}{
	"null":      {},
	"none":      {},
	"undefined": {},
	"true":      {},
	"false":     {},
}

// SetNodeTagsOptions represents request options for replacing a node's tags.
// Tags is the complete set the node should carry afterwards; an empty slice
// removes every tag.
type SetNodeTagsOptions struct {
	Tags []string
}

// NodeTags represents the tags carried by a single node with the raw backend
// payload.
type NodeTags struct {
	NodeUUID string   `json:"nodeUUID,omitempty"`
	Tags     []string `json:"tags,omitempty"`
	RawJSON  []byte   `json:"-"`
}

// SetNodeTags replaces every tag on a node with opts.Tags and returns the tags
// the node carries afterwards. This is a replacement, not a merge: a tag the
// node already has that is absent from opts.Tags is removed, and an empty
// opts.Tags clears the node's tags entirely.
//
// Tags are validated against the backend's rules before any request is made;
// see MaxTagLength and the format described on the `tag set` command.
func (c *Client) SetNodeTags(ctx context.Context, nodeUUID string, opts SetNodeTagsOptions) (NodeTags, error) {
	ctx, cancel := c.requestContext(ctx)
	defer cancel()

	nodeUUID, tags, payload, err := c.buildSetNodeTagsRequest(nodeUUID, opts)
	if err != nil {
		return NodeTags{}, err
	}

	resp, err := c.api.PutV1NodesNodeUuidTagsWithBodyWithResponse(ctx, nodeUUID, "application/json", bytes.NewReader(payload))
	if err != nil {
		return NodeTags{}, err
	}
	if resp.StatusCode() != http.StatusOK {
		return NodeTags{}, newAPIError(resp.StatusCode(), resp.Status(), resp.Body)
	}

	var data fleetapi.ModelsSetNodeTagsResponse
	if err := json.Unmarshal(resp.Body, &data); err != nil {
		return NodeTags{}, err
	}

	// The backend echoes the node it wrote, but fall back to the requested UUID
	// so the result always names the node even if that field is omitted.
	uuid := stringValue(data.NodeUUID)
	if uuid == "" {
		uuid = nodeUUID
	}

	// Likewise fall back to the tags that were just written when the field is
	// omitted. An explicit empty list still reports a cleared node, since that
	// arrives as a non-nil pointer to an empty slice.
	written := tags
	if data.Tags != nil {
		written = cloneStringSlice(data.Tags)
	}

	return NodeTags{
		NodeUUID: uuid,
		Tags:     written,
		RawJSON:  append([]byte(nil), resp.Body...),
	}, nil
}

// PreviewSetNodeTags builds the request SetNodeTags would send, without
// sending it. Building the body needs no network call, so there is no request
// context to bound; both PreviewSetNodeTags and SetNodeTags build the body
// through buildSetNodeTagsRequest, so a preview can never disagree with the
// request that is actually issued.
func (c *Client) PreviewSetNodeTags(_ context.Context, nodeUUID string, opts SetNodeTagsOptions) (RequestPreview, error) {
	nodeUUID, _, payload, err := c.buildSetNodeTagsRequest(nodeUUID, opts)
	if err != nil {
		return RequestPreview{}, err
	}

	req, err := fleetapi.NewPutV1NodesNodeUuidTagsRequestWithBody(c.generatedServerURL(), nodeUUID, "application/json", bytes.NewReader(payload))
	if err != nil {
		return RequestPreview{}, err
	}

	return RequestPreview{Method: req.Method, URL: req.URL.String(), Body: payload}, nil
}

// Validates the tags and builds the JSON body SetNodeTags and
// PreviewSetNodeTags both send.
func (c *Client) buildSetNodeTagsRequest(nodeUUID string, opts SetNodeTagsOptions) (string, []string, []byte, error) {
	nodeUUID = strings.TrimSpace(nodeUUID)
	if nodeUUID == "" {
		return "", nil, nil, fmt.Errorf("node UUID is required")
	}

	tags, err := normalizeTags(opts.Tags)
	if err != nil {
		return "", nil, nil, err
	}

	payload, err := json.Marshal(fleetapi.ModelsSetNodeTagsRequest{Tags: tags})
	if err != nil {
		return "", nil, nil, err
	}

	return nodeUUID, tags, payload, nil
}

// Trims, validates, and rejects duplicates in a requested tag set. The result
// is always non-nil: the request body serializes tags unconditionally, and
// clearing a node's tags has to send [] rather than null.
func normalizeTags(tags []string) ([]string, error) {
	normalized := make([]string, 0, len(tags))
	seen := make(map[string]struct{}, len(tags))

	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if err := validateTag(tag); err != nil {
			return nil, err
		}
		if _, duplicate := seen[tag]; duplicate {
			return nil, fmt.Errorf("duplicate tag %q", tag)
		}
		seen[tag] = struct{}{}
		normalized = append(normalized, tag)
	}

	return normalized, nil
}

// Checks one tag against the backend's documented format. Reporting the first
// rule a tag breaks locally keeps a typo in a long tag list from costing a
// round trip, but the backend remains authoritative: a tag that passes here can
// still be rejected there.
func validateTag(tag string) error {
	if tag == "" {
		return fmt.Errorf("tag must not be empty")
	}
	for _, char := range tag {
		if !isTagCharacter(char) {
			return fmt.Errorf(
				"invalid tag %q: use lowercase letters, digits, hyphens, and underscores",
				tag,
			)
		}
	}
	// Safe to index by byte from here: every character is ASCII.
	if len(tag) > MaxTagLength {
		return fmt.Errorf("invalid tag %q: longer than the %d-character maximum", tag, MaxTagLength)
	}
	if isTagSeparator(rune(tag[0])) || isTagSeparator(rune(tag[len(tag)-1])) {
		return fmt.Errorf("invalid tag %q: must start and end with a letter or digit", tag)
	}
	for i := 1; i < len(tag); i++ {
		if isTagSeparator(rune(tag[i])) && isTagSeparator(rune(tag[i-1])) {
			return fmt.Errorf("invalid tag %q: hyphens and underscores cannot be consecutive", tag)
		}
	}
	if _, reserved := reservedTagValues[tag]; reserved {
		return fmt.Errorf("invalid tag %q: reserved", tag)
	}

	return nil
}

// Reports whether a character may appear anywhere in a tag
func isTagCharacter(char rune) bool {
	return (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || isTagSeparator(char)
}

// Reports whether a character is one of the two tag separators
func isTagSeparator(char rune) bool {
	return char == '-' || char == '_'
}
