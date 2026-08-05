// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"fmt"
	"strings"
)

// MinPageSize and MaxPageSize bound the page size the API accepts, per
// api/openapi/openapi.yaml. The CLI mirrors these in internal/clihelpers; they
// are restated here because the SDK is used directly by callers who never go
// through the CLI's flag validation.
const (
	MinPageSize = 1
	MaxPageSize = 100
)

// ValidateResourceID checks an identifier that will be interpolated into the
// request URL path and returns it trimmed. name is the caller-facing label used
// in the error message, e.g. "node UUID".
//
// It is exported so the CLI can reject a hostile identifier before it builds a
// client, without the two layers drifting apart on what "valid" means.
//
// The generated client percent-escapes path parameters, so separators, query
// markers, and fragments cannot break out of their segment. Dot segments are
// the exception: "." and ".." survive escaping intact and are then resolved
// away when the operation path is joined to the base URL, so a caller-supplied
// ".." silently re-targets the request at a different endpoint (/v1/nodes/..
// resolves to /v1/). An empty value collapses the same way, turning an
// item request into a request against the collection.
//
// Path parameters are declared as bare strings in the OpenAPI spec — there is
// no UUID format to enforce — so this rejects only what would change which
// endpoint is called or what would smuggle control characters into the URL.
func ValidateResourceID(name, value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	if trimmed == "." || trimmed == ".." {
		return "", fmt.Errorf("invalid %s %q: value would redirect the request to a different API path", name, trimmed)
	}
	if strings.ContainsAny(trimmed, "/\\") {
		return "", fmt.Errorf("invalid %s %q: expected a single path segment", name, trimmed)
	}
	for _, r := range trimmed {
		// Reported without the value: echoing control bytes back through a
		// terminal is its own small hazard.
		if r < 0x20 || r == 0x7f {
			return "", fmt.Errorf("invalid %s: contains a control character", name)
		}
	}

	return trimmed, nil
}

// Checks the paging parameters shared by every list call. Page is 0-based in
// the SDK (the CLI presents it 1-based). Both are pointers because an unset
// value means "let the backend apply its default" and is always allowed.
func validatePagination(page, pageSize *int) error {
	if page != nil && *page < 0 {
		return fmt.Errorf("invalid page %d: expected a non-negative page number", *page)
	}
	if pageSize != nil && (*pageSize < MinPageSize || *pageSize > MaxPageSize) {
		return fmt.Errorf("invalid page size %d: expected %d-%d", *pageSize, MinPageSize, MaxPageSize)
	}

	return nil
}
