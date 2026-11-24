// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package updatecheck

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// Verifies the tag and the release page URL both come out of the redirect,
// without the page itself ever being fetched.
func TestLatest(t *testing.T) {
	var gotMethod, gotPath string
	var pageRequests int
	server := redirectServer(t, "v1.4.0", func(r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		if strings.Contains(r.URL.Path, releaseTagSegment) {
			pageRequests++
		}
	})

	release, err := Checker{ReleasesURL: server.URL + "/releases"}.Latest(context.Background())
	if err != nil {
		t.Fatalf("Latest failed: %v", err)
	}

	if gotMethod != http.MethodHead {
		t.Fatalf("expected a HEAD request, got %s", gotMethod)
	}
	if gotPath != "/releases/latest" {
		t.Fatalf("unexpected path %q", gotPath)
	}
	if pageRequests != 0 {
		t.Fatalf("expected the redirect not to be followed, got %d page requests", pageRequests)
	}
	if release.Version != "v1.4.0" {
		t.Fatalf("unexpected version %q", release.Version)
	}
	if want := server.URL + "/releases/tag/v1.4.0"; release.URL != want {
		t.Fatalf("release URL = %q, want %q", release.URL, want)
	}
}

// Verifies a relative Location header resolves against the request URL.
func TestLatestRelativeRedirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "/releases/tag/v1.4.0")
		w.WriteHeader(http.StatusFound)
	}))
	defer server.Close()

	release, err := Checker{ReleasesURL: server.URL + "/releases"}.Latest(context.Background())
	if err != nil {
		t.Fatalf("Latest failed: %v", err)
	}
	if release.Version != "v1.4.0" || release.URL != server.URL+"/releases/tag/v1.4.0" {
		t.Fatalf("unexpected release %#v", release)
	}
}

func TestLatestErrors(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		location string
		wantErr  string
	}{
		// A repository with no release redirects to its release index, where the
		// REST API would have answered 404.
		{name: "no release", status: http.StatusFound, location: "/releases", wantErr: "no published release"},
		{name: "empty tag", status: http.StatusFound, location: "/releases/tag/", wantErr: "no published release"},
		{name: "no redirect", status: http.StatusOK, wantErr: "expected a redirect, got status 200"},
		{name: "not found", status: http.StatusNotFound, wantErr: "expected a redirect, got status 404"},
		{name: "no location", status: http.StatusFound, wantErr: "redirect has no Location"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if test.location != "" {
					w.Header().Set("Location", test.location)
				}
				w.WriteHeader(test.status)
			}))
			defer server.Close()

			if _, err := (Checker{ReleasesURL: server.URL}).Latest(context.Background()); err == nil ||
				!strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("expected error containing %q, got %v", test.wantErr, err)
			}
		})
	}
}

// Verifies a named release resolves to its own page without following the
// latest-release permalink.
func TestRelease(t *testing.T) {
	var gotMethod, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	checker := Checker{ReleasesURL: server.URL + "/releases"}
	// The "v" is optional: a user reads the bare number off `version` output.
	for _, requested := range []string{"v1.1.0", "1.1.0", "V1.1.0"} {
		release, err := checker.Release(context.Background(), requested)
		if err != nil {
			t.Fatalf("Release(%q) failed: %v", requested, err)
		}
		if gotMethod != http.MethodHead {
			t.Fatalf("expected a HEAD request, got %s", gotMethod)
		}
		if gotPath != "/releases/tag/v1.1.0" {
			t.Fatalf("unexpected path %q for %q", gotPath, requested)
		}
		if release.Version != "v1.1.0" {
			t.Fatalf("Release(%q).Version = %q", requested, release.Version)
		}
		if want := server.URL + "/releases/tag/v1.1.0"; release.URL != want {
			t.Fatalf("release URL = %q, want %q", release.URL, want)
		}
	}
}

// Verifies an unpublished version is refused, and says so in a way the user can
// act on: this is the error behind `upgrade --version` on a typo.
func TestReleaseNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	_, err := Checker{ReleasesURL: server.URL}.Release(context.Background(), "9.9.9")
	if !errors.Is(err, ErrReleaseNotFound) {
		t.Fatalf("expected ErrReleaseNotFound, got %v", err)
	}
	for _, want := range []string{"v9.9.9", releasesPage} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error missing %q: %v", want, err)
		}
	}
}

// Verifies a malformed version fails without a network call, and an unexpected
// status is not mistaken for a published release.
func TestReleaseErrors(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	checker := Checker{ReleasesURL: server.URL}
	for _, requested := range []string{"", "  ", "latest", "1.x.0"} {
		if _, err := checker.Release(context.Background(), requested); err == nil {
			t.Fatalf("expected %q to be rejected", requested)
		}
	}
	if requests != 0 {
		t.Fatalf("expected a malformed version not to reach the network, got %d requests", requests)
	}

	_, err := checker.Release(context.Background(), "1.1.0")
	if err == nil || !strings.Contains(err.Error(), "unexpected status 500") {
		t.Fatalf("expected the status to surface, got %v", err)
	}
}

// Verifies a tag read off the wire is held to the same shape as one the user
// asked for: it too ends up as an argument to the installer.
func TestLatestRejectsHostileTag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "/releases/tag/v1.0.0-%24%28curl%20evil.test%29")
		w.WriteHeader(http.StatusFound)
	}))
	defer server.Close()

	_, err := Checker{ReleasesURL: server.URL}.Latest(context.Background())
	if err == nil || !strings.Contains(err.Error(), "is not a release tag") {
		t.Fatalf("expected a hostile tag to be refused, got %v", err)
	}
}

func TestCheck(t *testing.T) {
	server := redirectServer(t, "v1.4.0", nil)
	checker := Checker{ReleasesURL: server.URL}

	tests := []struct {
		name    string
		current string
		want    bool
	}{
		// goreleaser injects the bare number while the tag carries the "v".
		{name: "older", current: "1.3.9", want: true},
		{name: "older with prefix", current: "v1.3.9", want: true},
		{name: "current", current: "1.4.0"},
		{name: "prerelease of the same version", current: "1.4.0-rc.1", want: true},
		{name: "newer", current: "2.0.0"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := Check(context.Background(), checker, test.current)
			if err != nil {
				t.Fatalf("Check failed: %v", err)
			}
			if result.Available != test.want {
				t.Fatalf("Available = %v, want %v", result.Available, test.want)
			}
			if result.Version != "v1.4.0" {
				t.Fatalf("unexpected latest version %q", result.Version)
			}
		})
	}
}

// Verifies a non-release build short-circuits before any network call.
func TestCheckSkipsDevBuild(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		requests++
	}))
	defer server.Close()

	result, err := Check(context.Background(), Checker{ReleasesURL: server.URL}, "dev")
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if result.Available || result.Version != "" {
		t.Fatalf("expected an empty result, got %#v", result)
	}
	if requests != 0 {
		t.Fatalf("expected no request for a dev build, got %d", requests)
	}
}

func TestNotice(t *testing.T) {
	result := Result{Release: Release{Version: "v1.4.0", URL: "https://example.test/v1.4.0"}, Available: true}

	notice := Notice(result, "1.0.0")
	for _, want := range []string{"Update available: v1.4.0 (current v1.0.0)", "https://example.test/v1.4.0", "Upgrade: "} {
		if !strings.Contains(notice, want) {
			t.Fatalf("notice missing %q: %q", want, notice)
		}
	}

	result.Available = false
	if notice := Notice(result, "1.4.0"); notice != "" {
		t.Fatalf("expected no notice when up to date, got %q", notice)
	}
}

func TestReleaseLookupsRejectInvalidRequests(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	lookups := []struct {
		name string
		call func(context.Context, Checker) (Release, error)
	}{
		{"Latest", func(ctx context.Context, checker Checker) (Release, error) { return checker.Latest(ctx) }},
		{"Release", func(ctx context.Context, checker Checker) (Release, error) { return checker.Release(ctx, "1.2.3") }},
	}
	cases := []struct {
		name      string
		ctx       context.Context
		url       string
		cancelled bool
	}{
		{"malformed URL", context.Background(), "://bad", false},
		{"cancelled context", cancelled, server.URL, true},
	}
	for _, lookup := range lookups {
		for _, tc := range cases {
			t.Run(lookup.name+"/"+tc.name, func(t *testing.T) {
				got, err := lookup.call(tc.ctx, Checker{ReleasesURL: tc.url, HTTPClient: server.Client()})
				if err == nil {
					t.Error("expected an error")
				}
				if tc.cancelled && !errors.Is(err, context.Canceled) {
					t.Errorf("error = %v, want context.Canceled", err)
				}
				if got != (Release{}) {
					t.Errorf("release = %+v, want zero value", got)
				}
				if got := requests.Load(); got != 0 {
					t.Errorf("server received %d requests, want none", got)
				}
			})
		}
	}
}

func TestCheckRejectsUnparseablePublishedTag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "/releases/tag/v1.x.0")
		w.WriteHeader(http.StatusFound)
	}))
	defer server.Close()

	got, err := Check(context.Background(), Checker{ReleasesURL: server.URL + "/releases", HTTPClient: server.Client()}, "1.0.0")
	if err == nil {
		t.Error("Check succeeded with an unparseable published tag")
	}
	if got != (Result{}) {
		t.Errorf("result = %+v, want zero value", got)
	}
}

func TestLatestRejectsMalformedLocation(t *testing.T) {
	requests := make(chan struct{ method, path string }, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- struct{ method, path string }{r.Method, r.URL.Path}
		w.Header().Set("Location", "/releases/tag/%zz")
		w.WriteHeader(http.StatusMultipleChoices)
	}))
	defer server.Close()

	got, err := (Checker{ReleasesURL: server.URL + "/releases", HTTPClient: server.Client()}).Latest(context.Background())
	if err == nil || !strings.Contains(err.Error(), "release lookup failed") {
		t.Errorf("Latest error = %v, want release lookup failed", err)
	}
	if got != (Release{}) {
		t.Errorf("release = %+v, want zero value", got)
	}
	select {
	case request := <-requests:
		if request.method != http.MethodHead || request.path != "/releases/latest" {
			t.Errorf("request = %s %s, want HEAD /releases/latest", request.method, request.path)
		}
	default:
		t.Error("server did not receive the latest-release request")
	}
}
