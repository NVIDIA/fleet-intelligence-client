// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/config"
	"github.com/NVIDIA/fleet-intelligence-client/pkg/fleetintelligence"
)

// Verifies the overview table renders summary fields and metrics
func TestOverviewTableCommand(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

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

	if err := config.Save(config.Config{APIURL: server.URL, ServiceKey: "test-key"}); err != nil {
		t.Fatalf("save config failed: %v", err)
	}

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

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("includeMetrics"); got != "true" {
			t.Fatalf("expected includeMetrics=true, got %q (raw %q)", got, r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodesCount":1}`))
	}))
	defer server.Close()

	if err := config.Save(config.Config{APIURL: server.URL, ServiceKey: "test-key"}); err != nil {
		t.Fatalf("save config failed: %v", err)
	}

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

	raw := `{"nodesCount":5}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("includeMetrics"); got != "false" {
			t.Fatalf("unexpected includeMetrics: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(raw))
	}))
	defer server.Close()

	if err := config.Save(config.Config{APIURL: server.URL, ServiceKey: "test-key"}); err != nil {
		t.Fatalf("save config failed: %v", err)
	}

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
	if err := config.Save(config.Config{APIURL: "https://example.com", ServiceKey: "test-key"}); err != nil {
		t.Fatalf("save config failed: %v", err)
	}

	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"overview", "--output", "yaml"})

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected invalid output error")
	}
}

// Verifies metric value formatting edge cases
func TestOverviewMetricValueFormatting(t *testing.T) {
	if got := metricValue(fleetintelligence.OverviewMetric{Value: nil}); got != "-" {
		t.Fatalf("expected placeholder for nil value, got %q", got)
	}
	value := float32(12)
	if got := metricValue(fleetintelligence.OverviewMetric{Value: &value}); got != "12" {
		t.Fatalf("expected bare value without unit, got %q", got)
	}
	pct := float32(42.5)
	if got := metricValue(fleetintelligence.OverviewMetric{Value: &pct, Unit: "%"}); got != "42.5%" {
		t.Fatalf("expected percent unit without space, got %q", got)
	}
	temp := float32(58)
	if got := metricValue(fleetintelligence.OverviewMetric{Value: &temp, Unit: "C"}); got != "58 C" {
		t.Fatalf("expected non-percent unit with space, got %q", got)
	}
	if got := metricLabel(fleetintelligence.OverviewMetric{Description: "GPU util"}); got != "GPU util" {
		t.Fatalf("expected verbatim description fallback label, got %q", got)
	}
}
