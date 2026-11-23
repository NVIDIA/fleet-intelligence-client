// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package overview

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
	"github.com/NVIDIA/fleet-intelligence-client/internal/config"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
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

// Verifies the overview table renders summary fields and metrics
func TestOverviewTableCommand(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/overview" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Has("includeMetrics") {
			t.Fatalf("expected includeMetrics omitted, got %q", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodesCount":10,"healthNodeCount":7,"healthPercentage":70,"nodeGroupCount":3,"computeZoneCount":2,"gpusCount":80,"metrics":[{"name":"gpu_utilization","unit":"%","value":42.5}]}`))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"overview"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	got := out.String()
	// The metric label is preserved verbatim (not upper-cased), and a "%" unit
	// is formatted without a space to match the summary percentage ("70%").
	for _, want := range []string{"FIELD", "VALUE", "NODES", "10", "GPUS", "80", "HEALTH PERCENTAGE", "70%", "gpu_utilization", "42.5%"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q: %q", want, got)
		}
	}
	if strings.Contains(got, "GPU_UTILIZATION") {
		t.Fatalf("metric label should not be upper-cased: %q", got)
	}
}

// Verifies --include-metrics=true is forwarded explicitly
func TestOverviewForwardsIncludeMetricsTrue(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("includeMetrics"); got != "true" {
			t.Fatalf("expected includeMetrics=true, got %q (raw %q)", got, r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodesCount":1}`))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"overview", "--include-metrics=true"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}
}

// Verifies --include-metrics=false is forwarded and JSON output is raw
func TestOverviewJSONWithoutMetrics(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	raw := `{"nodesCount":5}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("includeMetrics"); got != "false" {
			t.Fatalf("unexpected includeMetrics: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(raw))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"overview", "--include-metrics=false", "--output", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	if strings.TrimSpace(out.String()) != raw {
		t.Fatalf("unexpected JSON output: %q", out.String())
	}
}

// Verifies invalid output format is rejected
func TestOverviewRejectsInvalidOutput(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	cmdtest.SaveConfig(t, "https://example.com", "test-key")

	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"overview", "--output", "yaml"})

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected invalid output error")
	}
}

// Verifies metric value formatting edge cases
func TestOverviewMetricValueFormatting(t *testing.T) {
	if got := metricValue(nvfleetint.OverviewMetric{Value: nil}); got != "-" {
		t.Fatalf("expected placeholder for nil value, got %q", got)
	}
	value := float32(12)
	if got := metricValue(nvfleetint.OverviewMetric{Value: &value}); got != "12" {
		t.Fatalf("expected bare value without unit, got %q", got)
	}
	pct := float32(42.5)
	if got := metricValue(nvfleetint.OverviewMetric{Value: &pct, Unit: "%"}); got != "42.5%" {
		t.Fatalf("expected percent unit without space, got %q", got)
	}
	temp := float32(58)
	if got := metricValue(nvfleetint.OverviewMetric{Value: &temp, Unit: "C"}); got != "58 C" {
		t.Fatalf("expected non-percent unit with space, got %q", got)
	}
	if got := metricLabel(nvfleetint.OverviewMetric{Description: "GPU util"}); got != "GPU util" {
		t.Fatalf("expected verbatim description fallback label, got %q", got)
	}
}

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
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			t.Setenv(config.EnvAPIKey, "")
			t.Setenv(config.EnvAPIURL, "")
			t.Setenv(config.EnvLegacyAPIKey, "")
			tc.run(t)
		})
	}
}
