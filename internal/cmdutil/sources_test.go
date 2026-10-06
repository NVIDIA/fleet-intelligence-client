// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package cmdutil

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
)

func TestWriteBothSourceTablesConcurrentFetchOrderedOutput(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	started := make(chan nvfleetint.DataSource, 2)
	release := make(chan struct{})
	done := make(chan error, 1)
	var out bytes.Buffer
	go func() {
		done <- WriteBothSourceTables(ctx, &out, "test report",
			func(ctx context.Context, source nvfleetint.DataSource) (string, error) {
				started <- source
				select {
				case <-release:
					return string(source), nil
				case <-ctx.Done():
					return "", ctx.Err()
				}
			},
			func(w io.Writer, result string) error {
				_, err := fmt.Fprintln(w, result)
				return err
			},
		)
	}()
	// Neither fetch can finish until both have started.
	seen := map[nvfleetint.DataSource]bool{}
	for range 2 {
		select {
		case source := <-started:
			seen[source] = true
		case <-ctx.Done():
			t.Fatal("source fetches did not run concurrently")
		}
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !seen[nvfleetint.DataSourceInband] || !seen[nvfleetint.DataSourceOOB] {
		t.Fatalf("wrong sources: %v", seen)
	}
	if want := "In-band\ninband\n\nOut-of-band\noob\n"; out.String() != want {
		t.Fatalf("output = %q, want %q", out.String(), want)
	}
}

func TestWriteBothSourceTablesFetchFailurePreventsRendering(t *testing.T) {
	for _, source := range []nvfleetint.DataSource{nvfleetint.DataSourceInband, nvfleetint.DataSourceOOB} {
		t.Run(string(source), func(t *testing.T) {
			failure := errors.New("request failed")
			var out bytes.Buffer
			writes := 0
			err := WriteBothSourceTables(context.Background(), &out, "test report",
				func(_ context.Context, requested nvfleetint.DataSource) (string, error) {
					if requested == source {
						return "", failure
					}
					return "success", nil
				},
				func(io.Writer, string) error { writes++; return nil },
			)
			label := "In-band"
			if source == nvfleetint.DataSourceOOB {
				label = "Out-of-band"
			}
			if !errors.Is(err, failure) || !strings.Contains(err.Error(), "fetch "+label+" test report") {
				t.Fatalf("lost error context: %v", err)
			}
			if writes != 0 || out.Len() != 0 {
				t.Fatalf("partial rendering: writes=%d output=%q", writes, &out)
			}
		})
	}
}

func TestWriteBothSourceTablesStopsOnRenderError(t *testing.T) {
	failure := errors.New("write failed")
	var out bytes.Buffer
	writes := 0
	err := WriteBothSourceTables(context.Background(), &out, "test report",
		func(_ context.Context, source nvfleetint.DataSource) (string, error) { return string(source), nil },
		func(io.Writer, string) error { writes++; return failure },
	)
	if !errors.Is(err, failure) || writes != 1 || strings.Contains(out.String(), "Out-of-band") {
		t.Fatalf("rendering continued after failure: err=%v writes=%d output=%q", err, writes, &out)
	}
}
