// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package computezone

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

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

// updateServer serves the read-modify-write pair, recording every request the
// command issued.
func updateServer(t *testing.T, requests *[]string, body *[]byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*requests = append(*requests, r.Method+" "+r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"computezones":[{"id":"cz-1","name":"East","type":"datacenter",` +
				`"contact":{"email":"ops@example.com","pic":"Jane Doe"},` +
				`"location":{"city":"Baltimore","region":"us-east-1","latitude":39.04581234}}],"page":0,"pageSize":20,"total":1}`))
			return
		}

		read, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body failed: %v", err)
		}
		*body = read
		_, _ = w.Write([]byte(`{"id":"cz-1"}`))
	}))
}

type computeZoneErrorWriter struct {
	err error
}

func (w computeZoneErrorWriter) Write(_ []byte) (int, error) {
	return 0, w.err
}
