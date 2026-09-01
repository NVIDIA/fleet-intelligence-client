// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import "strings"

// RequestPreview describes an HTTP request a write command would send,
// without sending it. --dry-run renders exactly this: the method, URL, and
// body bytes that would go over the wire, built through the same request
// construction the real call uses so the two can never disagree.
type RequestPreview struct {
	Method string
	URL    string
	Body   []byte
}

// generatedServerURL returns the client's base URL normalized exactly the way
// fleetapi.NewClient normalizes the server it is given: with a trailing
// slash. The generated request builders resolve "./v1/..." against this
// value, so matching the normalization here is what keeps a base URL with a
// path prefix resolving identically for a preview and for the real call.
func (c *Client) generatedServerURL() string {
	server := c.baseURL.String()
	if !strings.HasSuffix(server, "/") {
		server += "/"
	}
	return server
}
