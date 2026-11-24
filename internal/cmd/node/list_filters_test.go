// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package node

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
)

// Verifies local output flags and sort field pass-through
func TestNodeListLocalJSONAndSort(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	raw := `{"nodes":[{"nodeUUID":"node-1","hostname":"gpu-001","healthStatus":"Healthy"}],"hasMore":false,"page":0,"pageSize":20,"total":1}`
	// JSON output presents the page 1-based, which re-serializes top-level keys.
	want := `{"hasMore":false,"nodes":[{"nodeUUID":"node-1","hostname":"gpu-001","healthStatus":"Healthy"}],"page":1,"pageSize":20,"total":1}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/nodes" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("sortBy"); got != "healthStatus" {
			t.Fatalf("unexpected sortBy: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(raw))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{
		"node", "list", "--data-source", "inband", "--output", "json", "--sort-by", "healthStatus",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}
	if strings.TrimSpace(out.String()) != want {
		t.Fatalf("unexpected JSON: got %q want %q", strings.TrimSpace(out.String()), want)
	}
}

// Verifies detail-only sort fields pass through to the API unchanged
func TestNodeListDetailSortFields(t *testing.T) {
	for _, field := range []string{"agentVersion", "kernelVersion", "gpuDriverVersion", "gpuFirmwareVersions"} {
		t.Run(field, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.URL.Query().Get("sortBy"); got != field {
					t.Fatalf("unexpected sortBy: got %q want %q", got, field)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"nodes":[],"hasMore":false,"page":0,"pageSize":20,"total":0}`))
			}))
			defer server.Close()

			cmdtest.SaveConfig(t, server.URL, "test-key")

			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs([]string{
				"node", "list", "--data-source", "inband", "--output", "json", "--sort-by", field,
			})

			if err := cmd.Execute(); err != nil {
				t.Fatalf("command failed: %v", err)
			}
		})
	}
}

// Verifies the verificationCheck sort field reaches the API unchanged, and that
// the retired "integrityCheck" spelling is never sent in its place
func TestNodeListSortVerificationCheck(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("sortBy"); got != "verificationCheck" {
			t.Fatalf("unexpected sortBy: got %q want %q", got, "verificationCheck")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodes":[],"hasMore":false,"page":0,"pageSize":20,"total":0}`))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"node", "list", "--data-source", "inband", "--output", "json", "--sort-by", "verificationCheck"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}
}

// Verifies node list help and sort errors name "verificationCheck" and never
// the retired "integrityCheck" alias
func TestNodeListSortHelpUsesVerificationCheck(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	usage := newNodeListCmd().Flags().Lookup("sort-by").Usage
	if !strings.Contains(usage, "verificationCheck") {
		t.Fatalf("sort-by usage missing verificationCheck: %q", usage)
	}
	if strings.Contains(usage, "integrityCheck") {
		t.Fatalf("sort-by usage still mentions integrityCheck: %q", usage)
	}

	_, err := nodeListOptions(nodeListFlags{view: "detail", sortBy: "bogus"})
	if err == nil {
		t.Fatal("expected invalid sort-by error")
	}
	if !strings.Contains(err.Error(), "verificationCheck") || strings.Contains(err.Error(), "integrityCheck") {
		t.Fatalf("unexpected sort-by error: %v", err)
	}

	_, err = nodeListOptions(nodeListFlags{view: "basic", sortBy: "verificationCheck"})
	if err == nil {
		t.Fatal("expected basic view sort error")
	}
	if strings.Contains(err.Error(), "integrityCheck") {
		t.Fatalf("basic view sort error mentions integrityCheck: %v", err)
	}
}

// Verifies table output and filter translation
func TestNodeListTableFiltersAndSort(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/nodes" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		query := r.URL.Query()
		if got := query.Get("view"); got != "detail" {
			t.Fatalf("unexpected view: %q", got)
		}
		if got := query["nodeUUIDs"]; !slices.Equal(got, []string{"node-1", "node-2"}) {
			t.Fatalf("unexpected nodeUUIDs: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query["healthStatuses"]; !slices.Equal(got, []string{"Healthy", "Degraded"}) {
			t.Fatalf("unexpected healthStatuses: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query.Get("hostname"); got != "gpu" {
			t.Fatalf("unexpected hostname: %q", got)
		}
		if got := query["agentStatuses"]; !slices.Equal(got, []string{"Online"}) {
			t.Fatalf("unexpected agentStatuses: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query["verificationChecks"]; !slices.Equal(got, []string{"Verified"}) {
			t.Fatalf("unexpected verificationChecks: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query["firmwareChecks"]; !slices.Equal(got, []string{"Unknown"}) {
			t.Fatalf("unexpected firmwareChecks: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query["computeZoneIds"]; !slices.Equal(got, []string{"cz-1", "cz-2"}) {
			t.Fatalf("unexpected computeZoneIds: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query["computeZoneNames"]; !slices.Equal(got, []string{"East"}) {
			t.Fatalf("unexpected computeZoneNames: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query["nodeGroupIds"]; !slices.Equal(got, []string{"ng-1"}) {
			t.Fatalf("unexpected nodeGroupIds: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query["nodeGroupNames"]; !slices.Equal(got, []string{"Training"}) {
			t.Fatalf("unexpected nodeGroupNames: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query["gpuTypes"]; !slices.Equal(got, []string{"NVIDIA-H100"}) {
			t.Fatalf("unexpected gpuTypes: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query["gpuCounts"]; !slices.Equal(got, []string{"8", "4"}) {
			t.Fatalf("unexpected gpuCounts: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query["publicIPs"]; !slices.Equal(got, []string{"203.0.113.10"}) {
			t.Fatalf("unexpected publicIPs: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query["privateIPs"]; !slices.Equal(got, []string{"10.0.0.10"}) {
			t.Fatalf("unexpected privateIPs: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query.Get("sortBy"); got != "computezone" {
			t.Fatalf("unexpected sortBy: %q", got)
		}
		if got := query.Get("order"); got != "desc" {
			t.Fatalf("unexpected order: %q", got)
		}
		if got := query.Get("pageSize"); got != "10" {
			t.Fatalf("unexpected pageSize: %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodes":[{"nodeUUID":"node-1","hostname":"gpu-001","computeZone":"East","nodeGroup":"Training","healthStatus":"Healthy","gpuType":"NVIDIA-H100","gpuCount":8,"verificationCheck":"Verified","firmwareCheck":"Unknown","agentStatus":"Online"}],"hasMore":false,"page":0,"pageSize":10,"total":1}`))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"node", "list", "--data-source", "inband", "--node-uuids", "node-1,node-2", "--health", "Healthy,Degraded", "--hostname", "gpu", "--agent-status", "Online", "--verification-check", "Verified", "--firmware-check", "Unknown", "--compute-zone-ids", "cz-1,cz-2", "--compute-zone-names", "East", "--nodegroup-ids", "ng-1", "--nodegroup-names", "Training", "--gpu-type", "NVIDIA-H100", "--gpu-count", "8,4", "--public-ip", "203.0.113.10", "--private-ip", "10.0.0.10", "--sort-by", "computezone", "--order", "desc", "--page-size", "10"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{"UUID", "HOSTNAME", "COMPUTE ZONE", "NODE GROUP", "HEALTH", "GPU TYPE", "ATTESTATION", "FIRMWARE CHECK", "AGENT STATUS", "node-1", "gpu-001", "East", "Training", "Verified", "Unknown", "Online", "Page: 1"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q: %q", want, got)
		}
	}
}

func TestNodeVerificationPolicyStatusFilters(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	statuses := []string{"Full Attestation", "Partial Attestation", "Verification Failed"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !slices.Equal(r.URL.Query()["verificationChecks"], statuses) {
			t.Errorf("incorrect verification filters: %s", r.URL)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodes":[{"nodeUUID":"node-1","verificationCheck":"Full Attestation"}],"total":1}`))
	}))
	defer server.Close()
	cmdtest.SaveConfig(t, server.URL, "test-key")
	for _, source := range []string{"inband", "oob"} {
		var out bytes.Buffer
		cmd := newRootCmd()
		cmd.SetOut(&out)
		cmd.SetArgs([]string{"node", "list", "--data-source", source, "--verification-check", strings.Join(statuses, ",")})
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "Full Attestation") {
			t.Fatalf("missing status: %s", &out)
		}
	}
	var help bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&help)
	cmd.SetArgs([]string{"node", "list", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, status := range statuses {
		if !strings.Contains(help.String(), status) {
			t.Fatalf("missing help status %s", status)
		}
	}
}
