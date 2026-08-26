// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package cmdutil

import (
	"bytes"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
)

// Verifies hasMore-driven page traversal
func TestFetchAllRawPagesWithHasMore(t *testing.T) {
	calls := []int{}
	result, err := FetchAllRawPages("items", 0, func(page int) (RawPage, error) {
		calls = append(calls, page)
		hasMore := page == 0
		if page == 0 {
			return RawPage{RawJSON: []byte(`{"items":[{"id":"a"}]}`), Page: 0, PageSize: 1, Total: 2, HasMore: &hasMore}, nil
		}
		return RawPage{RawJSON: []byte(`{"items":[{"id":"b"}]}`), Page: 1, PageSize: 1, Total: 2, HasMore: &hasMore}, nil
	})
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if len(calls) != 2 || calls[0] != 0 || calls[1] != 1 {
		t.Fatalf("unexpected calls: %#v", calls)
	}
	if len(result.Items) != 2 || result.Pagination.Page != 0 || result.Pagination.PageSize != 1 || result.Pagination.Total != 2 || result.Pagination.HasMore || result.Pagination.PagesFetched != 2 {
		t.Fatalf("unexpected result: %#v", result)
	}
}

// Verifies total-driven page traversal
func TestFetchAllRawPagesWithTotal(t *testing.T) {
	result, err := FetchAllRawPages("alerts", 1, func(page int) (RawPage, error) {
		switch page {
		case 1:
			return RawPage{RawJSON: []byte(`{"alerts":[{"id":"a"}]}`), Page: 1, PageSize: 1, Total: 2}, nil
		case 2:
			return RawPage{RawJSON: []byte(`{"alerts":[{"id":"b"}]}`), Page: 2, PageSize: 1, Total: 2}, nil
		default:
			t.Fatalf("unexpected Page: %d", page)
			return RawPage{}, nil
		}
	})
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if len(result.Items) != 2 || result.Pagination.Page != 1 || result.Pagination.PagesFetched != 2 {
		t.Fatalf("unexpected result: %#v", result)
	}
}

// Verifies empty pages stop traversal
func TestFetchAllRawPagesStopsOnEmptyPage(t *testing.T) {
	result, err := FetchAllRawPages("items", 0, func(page int) (RawPage, error) {
		return RawPage{RawJSON: []byte(`{"items":[]}`), Page: page, PageSize: 10, Total: 0}, nil
	})
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if len(result.Items) != 0 || result.Pagination.PagesFetched != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
}

// Verifies a short page stops traversal when Total is unreported (0) and
// HasMore is nil, so item-bearing pages cannot walk to MaxPages.
func TestFetchAllRawPagesStopsOnShortPageWithoutTotal(t *testing.T) {
	pages := 0
	result, err := FetchAllRawPages("items", 0, func(page int) (RawPage, error) {
		pages++
		switch page {
		case 0:
			return RawPage{RawJSON: []byte(`{"items":[{"id":"a"},{"id":"b"}]}`), Page: 0, PageSize: 2, Total: 0}, nil
		case 1:
			// Final page is shorter than PageSize: must stop here.
			return RawPage{RawJSON: []byte(`{"items":[{"id":"c"}]}`), Page: 1, PageSize: 2, Total: 0}, nil
		default:
			t.Fatalf("traversal did not stop on short page; fetched page %d", page)
			return RawPage{}, nil
		}
	})
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if len(result.Items) != 3 || result.Pagination.PagesFetched != 2 || result.Pagination.HasMore {
		t.Fatalf("unexpected result: %#v", result)
	}
	if pages != 2 {
		t.Fatalf("expected 2 fetches, got %d", pages)
	}
}

// Verifies fetch errors are returned
func TestFetchAllRawPagesReturnsFetchError(t *testing.T) {
	_, err := FetchAllRawPages("items", 0, func(_ int) (RawPage, error) {
		return RawPage{}, errors.New("backend failed")
	})
	if err == nil || !strings.Contains(err.Error(), "backend failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Verifies raw page numbers are normalized only when safe to increment.
func TestOneIndexRawPage(t *testing.T) {
	maxInt := strconv.Itoa(int(^uint(0) >> 1))
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "page", raw: `{"page":0}`, want: `{"page":1}`},
		{name: "empty page", raw: `{"page":0,"pageSize":20,"total":0}`, want: `{"page":0,"pageSize":20,"total":0}`},
		{name: "empty requested page", raw: `{"page":1,"pageSize":20,"total":0}`, want: `{"page":0,"pageSize":20,"total":0}`},
		{name: "past last page", raw: `{"page":9,"pageSize":20,"total":50}`, want: `{"page":3,"pageSize":20,"total":50}`},
		{name: "missing", raw: `{"items":[]}`, want: `{"items":[]}`},
		{name: "null", raw: `{"page":null}`, want: `{"page":null}`},
		{name: "max int", raw: `{"page":` + maxInt + `}`, want: `{"page":` + maxInt + `}`},
		{name: "invalid json", raw: `not-json`, want: `not-json`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := string(OneIndexRawPage([]byte(tt.raw))); got != tt.want {
				t.Fatalf("unexpected raw Page: got %q want %q", got, tt.want)
			}
		})
	}
}

// Verifies raw item extraction behavior
func TestExtractRawItems(t *testing.T) {
	items, err := ExtractRawItems([]byte(`{"items":[{"id":"a"}]}`), "items")
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	if len(items) != 1 || string(items[0]) != `{"id":"a"}` {
		t.Fatalf("unexpected items: %#v", items)
	}

	items, err = ExtractRawItems([]byte(`{"missing":null}`), "items")
	if err != nil {
		t.Fatalf("extract missing failed: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected no items, got %#v", items)
	}
}

// Verifies invalid payload errors
func TestExtractRawItemsRejectsInvalidJSON(t *testing.T) {
	if _, err := ExtractRawItems([]byte(`not-json`), "items"); err == nil {
		t.Fatal("expected invalid JSON error")
	}
	if _, err := ExtractRawItems([]byte(`{"items":{}}`), "items"); err == nil {
		t.Fatal("expected invalid item array error")
	}
}

// Verifies merged empty-list JSON uses page 0 when there are no pages.
func TestWritePaginatedListJSONEmpty(t *testing.T) {
	var out bytes.Buffer
	result := MergedJSONResult{
		Items: []json.RawMessage{},
		Pagination: MergedPagination{
			Page: 1, PageSize: 20, Total: 0, PagesFetched: 1,
		},
	}
	if err := WritePaginatedListJSON(&out, nil, result); err != nil {
		t.Fatalf("write paginated JSON failed: %v", err)
	}

	var got MergedJSONResult
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode paginated JSON failed: %v", err)
	}
	if got.Pagination.Page != 0 {
		t.Fatalf("unexpected page: got %d want 0", got.Pagination.Page)
	}
}
