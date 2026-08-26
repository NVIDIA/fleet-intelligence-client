// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

// PageInfo is the pagination envelope carried by every paginated list
// response. It lets a caller page through any list endpoint without restating
// the envelope once per response type.
type PageInfo struct {
	Page     int
	PageSize int
	Total    int
	// HasMore is nil when the endpoint does not report it, which callers must
	// treat as "not reported" rather than as false. Every endpoint the SDK
	// exposes today reports or derives it, so this is a contract for endpoints
	// added later rather than a case any current response hits.
	HasMore *bool
	RawJSON []byte
}

// Paginated is implemented by every paginated SDK list response.
type Paginated interface {
	PageInfo() PageInfo
}

// Derives whether further pages remain from the page counters, for endpoints
// that report no hasMore field of their own.
func hasMoreFromCounts(page, pageSize, total int) bool {
	if page < 0 || pageSize <= 0 || total <= 0 {
		return false
	}
	// Page is 0-indexed, so the first (page+1) pages have been seen so far.
	return (page+1)*pageSize < total
}
