// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package tag

import (
	"net/http"
	"net/url"
	"sync"

	"github.com/spf13/cobra"
)

// newRootCmd builds a root command carrying only this package's commands, so
// the tests drive them through the same argument path a user types.
func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "nvfleetint",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(NewCmd())
	return root
}

type tagRecordedRequest struct {
	method string
	path   string
	query  url.Values
}

type tagRequestRecorder struct {
	mu       sync.Mutex
	requests []tagRecordedRequest
}

func (rec *tagRequestRecorder) record(r *http.Request) {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	rec.requests = append(rec.requests, tagRecordedRequest{r.Method, r.URL.Path, r.URL.Query()})
}

func (rec *tagRequestRecorder) snapshot() []tagRecordedRequest {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	return append([]tagRecordedRequest(nil), rec.requests...)
}
