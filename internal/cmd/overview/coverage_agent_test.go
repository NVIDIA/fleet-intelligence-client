// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package overview

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
	"github.com/NVIDIA/fleet-intelligence-client/internal/config"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
)

func TestOverviewCommandFailures(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  func(*testing.T)
	}{
		{"missing credentials", func(t *testing.T) {
			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs([]string{"overview"})

			err := cmd.Execute()
			if !errors.Is(err, config.ErrNoProfile) {
				t.Fatalf("expected no profile error, got %v", err)
			}
			if !strings.Contains(err.Error(), "nvfleetint auth add <name>") || !strings.Contains(err.Error(), config.EnvAPIKey) {
				t.Errorf("credential error lacks setup guidance: %v", err)
			}
			if out.Len() != 0 {
				t.Errorf("unexpected result output: %q", out.String())
			}
		}},
		{"forbidden overview", func(t *testing.T) {
			type request struct{ method, path, query string }
			var mu sync.Mutex
			var requests []request
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				requests = append(requests, request{r.Method, r.URL.Path, r.URL.RawQuery})
				mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"Forbidden","details":"no overview access"}`))
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")

			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs([]string{"overview", "--include-metrics=false", "--output", "json"})
			err := cmd.Execute()
			var apiErr *nvfleetint.APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("expected API error, got %v", err)
			}
			if apiErr.StatusCode != http.StatusForbidden || apiErr.Message != "Forbidden" || apiErr.Details != "no overview access" {
				t.Errorf("unexpected API error: %+v", apiErr)
			}
			if out.Len() != 0 {
				t.Errorf("unexpected result output: %q", out.String())
			}
			mu.Lock()
			got := append([]request(nil), requests...)
			mu.Unlock()
			if len(got) != 1 || got[0] != (request{http.MethodGet, "/v1/overview", "includeMetrics=false"}) {
				t.Errorf("unexpected requests: %+v", got)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv(config.EnvAPIKey, "")
			t.Setenv(config.EnvAPIURL, "")
			t.Setenv(config.EnvLegacyAPIKey, "")
			tc.run(t)
		})
	}
}
