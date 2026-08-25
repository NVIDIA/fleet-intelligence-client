// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package output

import "testing"

// Verifies page counts and display pages remain internally consistent.
func TestPaginationDisplay(t *testing.T) {
	tests := []struct {
		name      string
		page      int
		pageSize  int
		total     int
		wantPages int
		wantPage  int
	}{
		{name: "empty first page", page: 0, pageSize: 20, total: 0, wantPages: 0, wantPage: 0},
		{name: "empty requested page", page: 1, pageSize: 20, total: 0, wantPages: 0, wantPage: 0},
		{name: "normal first page", page: 0, pageSize: 20, total: 50, wantPages: 3, wantPage: 1},
		{name: "normal last page", page: 2, pageSize: 20, total: 50, wantPages: 3, wantPage: 3},
		{name: "past last page", page: 9, pageSize: 20, total: 50, wantPages: 3, wantPage: 3},
		{name: "invalid negative page", page: -1, pageSize: 20, total: 50, wantPages: 3, wantPage: 1},
		{name: "invalid page size", page: 0, pageSize: 0, total: 50, wantPages: 0, wantPage: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TotalPages(tt.total, tt.pageSize); got != tt.wantPages {
				t.Fatalf("unexpected total pages: got %d want %d", got, tt.wantPages)
			}
			if got := OneBasedPage(tt.page, tt.pageSize, tt.total); got != tt.wantPage {
				t.Fatalf("unexpected display page: got %d want %d", got, tt.wantPage)
			}
		})
	}
}
