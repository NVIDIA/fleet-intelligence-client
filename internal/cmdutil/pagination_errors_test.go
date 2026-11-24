// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package cmdutil

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestFetchAllRawPagesRejectsMalformedItemsAfterValidPage(t *testing.T) {
	hasMore := true
	var requested []int
	result, err := FetchAllRawPages("items", 3, func(page int) (RawPage, error) {
		requested = append(requested, page)
		if page == 3 {
			return RawPage{RawJSON: []byte(`{"items":[{"id":1}]}`), Page: page, HasMore: &hasMore}, nil
		}
		return RawPage{RawJSON: []byte(`{"items":{}}`), Page: page, HasMore: &hasMore}, nil
	})
	if err == nil || !strings.Contains(err.Error(), "decode items items") {
		t.Errorf("FetchAllRawPages error = %v, want items decode error", err)
	}
	if result.Items != nil || result.Pagination != (MergedPagination{}) {
		t.Errorf("FetchAllRawPages result = %+v, want zero result", result)
	}
	if len(requested) != 2 || requested[0] != 3 || requested[1] != 4 {
		t.Errorf("requested pages = %v, want [3 4]", requested)
	}
}

func TestFetchAllRawPagesStopsAtLimitWithEmptyPages(t *testing.T) {
	hasMore := true
	const startPage = 3
	var requested []int
	result, err := FetchAllRawPages("items", startPage, func(page int) (RawPage, error) {
		requested = append(requested, page)
		return RawPage{RawJSON: []byte(`{"items":[]}`), Page: page, HasMore: &hasMore}, nil
	})
	if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("stopped after %d pages while fetching items", MaxPages)) {
		t.Errorf("FetchAllRawPages error = %v, want page limit error", err)
	}
	if result.Items != nil || result.Pagination != (MergedPagination{}) {
		t.Errorf("FetchAllRawPages result = %+v, want zero result", result)
	}
	if len(requested) != MaxPages {
		t.Errorf("requested %d pages, want %d", len(requested), MaxPages)
	}
	for i, page := range requested {
		if page != startPage+i {
			t.Errorf("requested page at index %d = %d, want %d", i, page, startPage+i)
			break
		}
	}
}

func TestOneIndexRawPagePreservesNonIntegerPage(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "string", body: `{ "page": "3", "items": [1], "total": 5 }`},
		{name: "object", body: `{ "page": {"number": 3}, "items": [1], "total": 5 }`},
		{name: "fractional number", body: `{ "page": 3.5, "items": [1], "total": 5 }`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := []byte(tc.body)
			if got := OneIndexRawPage(input); !bytes.Equal(got, input) {
				t.Errorf("OneIndexRawPage = %q, want unchanged %q", got, input)
			}
		})
	}
}
