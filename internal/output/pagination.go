// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package output

// The CLI presents pages 1-based while the API pages from 0. These convert
// between the two. They live here rather than beside the page-fetching helpers
// because they are arithmetic on the numbers a footer shows, which is what
// lets internal/output render one without depending on the fetching code.

// TotalPages returns the number of pages needed to hold total items.
func TotalPages(total, pageSize int) int {
	if total <= 0 || pageSize <= 0 {
		return 0
	}
	return (total-1)/pageSize + 1
}

// OneBasedPage converts an SDK page to the CLI's 1-based page number and
// bounds it to the available pages. Empty results are represented as page 0 of
// 0 so the displayed page never exceeds the total page count.
func OneBasedPage(page, pageSize, total int) int {
	totalPages := TotalPages(total, pageSize)
	if totalPages == 0 {
		return 0
	}
	if page < 0 {
		return 1
	}
	if page >= totalPages {
		return totalPages
	}
	return page + 1
}
