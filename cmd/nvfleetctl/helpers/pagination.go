// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package helpers

import (
	"encoding/json"
	"fmt"
)

const (
	// MinPageSize is the smallest page size accepted by list commands
	MinPageSize = 1
	// MaxPageSize is the largest page size accepted by list commands
	MaxPageSize = 100
	// MaxPages bounds all-page pagination when the API keeps reporting more data
	MaxPages = 10000
)

// Represents one paginated raw API response
type RawPage struct {
	RawJSON  []byte
	Page     int
	PageSize int
	Total    int
	HasMore  *bool
}

// Represents normalized JSON for all-page output
type MergedJSONResult struct {
	Items      []json.RawMessage `json:"items"`
	Pagination MergedPagination  `json:"pagination"`
}

// Represents metadata for normalized all-page JSON
type MergedPagination struct {
	Page         int  `json:"page"`
	PageSize     int  `json:"pageSize"`
	Total        int  `json:"total"`
	HasMore      bool `json:"hasMore"`
	PagesFetched int  `json:"pagesFetched"`
}

// Fetches and merges raw API item payloads across pages
func FetchAllRawPages(itemKey string, startPage int, fetch func(page int) (RawPage, error)) (MergedJSONResult, error) {
	result := MergedJSONResult{
		Items: []json.RawMessage{},
	}
	for offset := 0; ; offset++ {
		if offset >= MaxPages {
			return MergedJSONResult{}, fmt.Errorf("stopped after %d pages while fetching %s", MaxPages, itemKey)
		}

		page, err := fetch(startPage + offset)
		if err != nil {
			return MergedJSONResult{}, err
		}
		items, err := ExtractRawItems(page.RawJSON, itemKey)
		if err != nil {
			return MergedJSONResult{}, err
		}

		if result.Pagination.PagesFetched == 0 {
			result.Pagination.Page = page.Page
			result.Pagination.PageSize = page.PageSize
		}
		result.Items = append(result.Items, items...)
		result.Pagination.Total = page.Total
		result.Pagination.PagesFetched++

		// page.HasMore is authoritative when the API reports it.
		if page.HasMore != nil {
			result.Pagination.HasMore = *page.HasMore
			if !*page.HasMore {
				return result, nil
			}
			continue
		}

		// No HasMore signal: infer termination from Total and page contents.
		// Stop once we have collected everything the API claims exists.
		if page.Total > 0 && len(result.Items) >= page.Total {
			result.Pagination.HasMore = false
			return result, nil
		}
		// A short or empty page means there is nothing left to fetch. This is
		// the forward-progress guard for when Total is unreported or not yet
		// reached and the API returns items without a count: a final page
		// smaller than PageSize ends traversal without an extra empty request.
		// MaxPages remains the backstop against an API that never shrinks a
		// page (e.g. returning the same full page repeatedly).
		if len(items) == 0 || (page.PageSize > 0 && len(items) < page.PageSize) {
			result.Pagination.HasMore = false
			return result, nil
		}
	}
}

// OneIndexRawPage rewrites the 0-indexed "page" field in a raw list payload to
// its 1-indexed equivalent so JSON consumers see the CLI's 1-based paging
// contract. The original bytes are returned unchanged when the payload has no
// usable page field.
func OneIndexRawPage(data []byte) []byte {
	var body map[string]json.RawMessage
	if err := json.Unmarshal(data, &body); err != nil {
		return data
	}
	raw, ok := body["page"]
	if !ok {
		return data
	}
	var page *int
	if err := json.Unmarshal(raw, &page); err != nil {
		return data
	}
	if page == nil || *page == int(^uint(0)>>1) {
		return data
	}
	normalized, err := json.Marshal(*page + 1)
	if err != nil {
		return data
	}
	body["page"] = normalized
	out, err := json.Marshal(body)
	if err != nil {
		return data
	}
	return out
}

// Extracts raw array items from an API response object
func ExtractRawItems(data []byte, itemKey string) ([]json.RawMessage, error) {
	var body map[string]json.RawMessage
	if err := json.Unmarshal(data, &body); err != nil {
		return nil, err
	}

	rawItems, ok := body[itemKey]
	if !ok || string(rawItems) == "null" {
		return []json.RawMessage{}, nil
	}

	var items []json.RawMessage
	if err := json.Unmarshal(rawItems, &items); err != nil {
		return nil, fmt.Errorf("decode %s items: %w", itemKey, err)
	}
	return items, nil
}
