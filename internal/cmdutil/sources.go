// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package cmdutil

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
)

// WriteBothSourceTables fetches both sources concurrently and renders them in
// in-band, then out-of-band order. A fetch error prevents all output. The caller
// handles pagination and table columns; this helper owns headings and spacing.
// Fetch is called concurrently and must not mutate shared request options.
func WriteBothSourceTables[T any](
	ctx context.Context,
	w io.Writer,
	resource string,
	fetch func(context.Context, nvfleetint.NodeDataSource) (T, error),
	write func(io.Writer, T) error,
) error {
	sources := [2]struct {
		source nvfleetint.NodeDataSource
		label  string
	}{
		{nvfleetint.NodeDataSourceInband, "In-band"},
		{nvfleetint.NodeDataSourceOOB, "Out-of-band"},
	}
	var results [2]T
	var errs [2]error
	var wg sync.WaitGroup
	wg.Add(len(sources))
	for i, source := range sources {
		go func() {
			defer wg.Done()
			results[i], errs[i] = fetch(ctx, source.source)
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			return fmt.Errorf("fetch %s %s: %w", sources[i].label, resource, err)
		}
	}
	for i, source := range sources {
		if i > 0 {
			if _, err := fmt.Fprintln(w); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(w, source.label); err != nil {
			return err
		}
		if err := write(w, results[i]); err != nil {
			return err
		}
	}
	return nil
}
