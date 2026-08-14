// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

// Package updatecheck reports whether a newer nvfleetint release is published
// on GitHub. It is CLI-only support code: the SDK never reaches out to GitHub.
package updatecheck

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"
)

const (
	// releasesPage is the repository's release index on the GitHub website.
	releasesPage = "https://github.com/NVIDIA/fleet-intelligence-client/releases"

	// DefaultReleaseURL is the permalink for the newest published release. It
	// resolves to the newest release that is neither a draft nor a prerelease,
	// so a stable install is never nudged onto a release candidate.
	//
	// This is deliberately a github.com web URL rather than the equivalent
	// api.github.com endpoint: the redirect it answers with already names the
	// release, and the website is not subject to the REST API's unauthenticated
	// quota of 60 requests per hour per IP. That quota is shared with every
	// other unauthenticated caller behind the same address, so a courtesy check
	// has no business spending it. install.sh resolves the version to download
	// the same way.
	DefaultReleaseURL = releasesPage + "/latest"

	// releaseTagSegment separates the repository path from the tag in a resolved
	// release URL.
	releaseTagSegment = "/releases/tag/"

	// EnvDisable turns the check off entirely when set to a truthy value.
	EnvDisable = "NVFLEETINT_NO_UPDATE_CHECK"

	// DefaultTimeout bounds the release lookup. It is deliberately short: the
	// check is a courtesy, and `nvfleetint version` must stay fast offline.
	DefaultTimeout = 3 * time.Second
)

// Release describes the newest published nvfleetint release.
type Release struct {
	// Version is the release tag as published, e.g. "v1.2.0".
	Version string `json:"latestVersion"`
	// URL points at the release page for those notes.
	URL string `json:"releaseUrl,omitempty"`
}

// Result reports the outcome of comparing the running build against Release.
type Result struct {
	Release
	// Available reports whether Version is newer than the running build.
	Available bool `json:"updateAvailable"`
}

// Checker resolves the newest published release from GitHub.
type Checker struct {
	// URL is the latest-release permalink; DefaultReleaseURL when empty.
	URL string
	// HTTPClient issues the request; a DefaultTimeout client when nil. Its
	// redirect policy is overridden, since reading the redirect is the point.
	HTTPClient *http.Client
}

// Latest returns the newest published release.
//
// It issues a HEAD against the latest-release permalink and reads the redirect
// instead of following it: GitHub answers with the release's own page, whose
// URL both names the tag and is the page a user wants to open. Fetching that
// page would add nothing.
func (c Checker) Latest(ctx context.Context) (Release, error) {
	requestURL := c.URL
	if requestURL == "" {
		requestURL = DefaultReleaseURL
	}

	client := http.Client{Timeout: DefaultTimeout}
	if c.HTTPClient != nil {
		client = *c.HTTPClient
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodHead, requestURL, nil)
	if err != nil {
		return Release{}, err
	}
	request.Header.Set("User-Agent", "nvfleetint")

	response, err := client.Do(request)
	if err != nil {
		return Release{}, err
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode < 300 || response.StatusCode > 399 {
		return Release{}, fmt.Errorf("release lookup failed: expected a redirect, got status %d", response.StatusCode)
	}
	location := response.Header.Get("Location")
	if location == "" {
		return Release{}, errors.New("release lookup failed: redirect has no Location")
	}
	// Resolve rather than parse: a Location header is allowed to be relative.
	target, err := response.Request.URL.Parse(location)
	if err != nil {
		return Release{}, fmt.Errorf("release lookup failed: %w", err)
	}

	// A repository with no published release redirects to its release index
	// instead of a tag, which is where the REST API would have returned a 404.
	_, tag, found := strings.Cut(target.Path, releaseTagSegment)
	if !found || tag == "" {
		return Release{}, errors.New("release lookup failed: no published release")
	}
	// The tag reaches us as a path segment, so an unusual tag arrives escaped.
	if unescaped, err := url.PathUnescape(tag); err == nil {
		tag = unescaped
	}

	return Release{Version: tag, URL: target.String()}, nil
}

// Check compares current against the newest published release. A build whose
// version is not a release version — the `dev` default of a `go build` — has
// nothing meaningful to compare, so it is reported as up to date without a
// network call.
func Check(ctx context.Context, checker Checker, current string) (Result, error) {
	if !IsReleaseVersion(current) {
		return Result{}, nil
	}

	release, err := checker.Latest(ctx)
	if err != nil {
		return Result{}, err
	}

	newer, err := IsNewer(release.Version, current)
	if err != nil {
		return Result{}, err
	}
	return Result{Release: release, Available: newer}, nil
}

// Disabled reports whether the environment turned the check off.
func Disabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(EnvDisable))) {
	case "", "0", "false", "no":
		return false
	default:
		return true
	}
}

// IsReleaseVersion reports whether version parses as a semantic version, i.e.
// whether it came from a release build's ldflags rather than the `dev` default.
func IsReleaseVersion(version string) bool {
	_, err := parseVersion(version)
	return err == nil
}

// IsNewer reports whether candidate is a later version than current. Both
// accept an optional leading "v": goreleaser injects the bare number while the
// git tag carries the prefix.
func IsNewer(candidate, current string) (bool, error) {
	order, err := compare(candidate, current)
	if err != nil {
		return false, err
	}
	return order > 0, nil
}

// Notice renders the upgrade prompt shown when an update is available. It
// returns an empty string when there is nothing to say.
func Notice(result Result, current string) string {
	if !result.Available {
		return ""
	}

	var builder strings.Builder
	fmt.Fprintf(&builder, "\nUpdate available: %s (current %s)\n", result.Version, displayVersion(current))
	if result.URL != "" {
		fmt.Fprintf(&builder, "Release notes: %s\n", result.URL)
	}
	fmt.Fprintf(&builder, "Upgrade: %s\n", upgradeCommand())
	return builder.String()
}

// upgradeCommand returns the install command for the running platform, matching
// the README's install instructions.
func upgradeCommand() string {
	if runtime.GOOS == "windows" {
		return "Invoke-WebRequest " + releasesPage + "/latest/download/install.ps1 -OutFile install.ps1; .\\install.ps1"
	}
	return "curl -fsSL " + releasesPage + "/latest/download/install.sh | bash"
}

// displayVersion prints the running version the way releases are tagged, so the
// two versions in the notice are directly comparable.
func displayVersion(version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		return "unknown"
	}
	if strings.HasPrefix(version, "v") || strings.HasPrefix(version, "V") {
		return version
	}
	return "v" + version
}

// semver holds a parsed semantic version. Build metadata is discarded: it is
// explicitly not part of version precedence.
type semver struct {
	major      uint64
	minor      uint64
	patch      uint64
	prerelease []string
}

// compare returns -1, 0, or 1 as a orders before, with, or after b.
func compare(a, b string) (int, error) {
	left, err := parseVersion(a)
	if err != nil {
		return 0, err
	}
	right, err := parseVersion(b)
	if err != nil {
		return 0, err
	}

	for _, pair := range [][2]uint64{
		{left.major, right.major},
		{left.minor, right.minor},
		{left.patch, right.patch},
	} {
		if pair[0] != pair[1] {
			return sign(pair[0], pair[1]), nil
		}
	}
	return comparePrerelease(left.prerelease, right.prerelease), nil
}

// parseVersion parses a semantic version with an optional leading "v". A
// missing minor or patch is treated as zero, so "v1" and "v1.2" both parse.
func parseVersion(version string) (semver, error) {
	trimmed := strings.TrimSpace(version)
	trimmed = strings.TrimPrefix(strings.TrimPrefix(trimmed, "v"), "V")
	if trimmed == "" {
		return semver{}, errors.New("empty version")
	}

	// Build metadata is ignored for precedence, so drop it before anything else.
	if index := strings.IndexByte(trimmed, '+'); index >= 0 {
		trimmed = trimmed[:index]
	}

	parsed := semver{}
	if index := strings.IndexByte(trimmed, '-'); index >= 0 {
		prerelease := trimmed[index+1:]
		if prerelease == "" {
			return semver{}, fmt.Errorf("invalid version %q: empty prerelease", version)
		}
		parsed.prerelease = strings.Split(prerelease, ".")
		trimmed = trimmed[:index]
	}

	parts := strings.Split(trimmed, ".")
	if len(parts) > 3 {
		return semver{}, fmt.Errorf("invalid version %q: too many numeric parts", version)
	}
	targets := []*uint64{&parsed.major, &parsed.minor, &parsed.patch}
	for i, part := range parts {
		value, err := parseNumeric(part)
		if err != nil {
			return semver{}, fmt.Errorf("invalid version %q: %w", version, err)
		}
		*targets[i] = value
	}
	return parsed, nil
}

// comparePrerelease applies the semver precedence rules for prerelease
// identifiers: a version without one outranks a version with one, numeric
// identifiers compare numerically and rank below alphanumeric ones, and a
// shorter run of otherwise-equal identifiers ranks lower.
func comparePrerelease(left, right []string) int {
	switch {
	case len(left) == 0 && len(right) == 0:
		return 0
	case len(left) == 0:
		return 1
	case len(right) == 0:
		return -1
	}

	for i := 0; i < len(left) && i < len(right); i++ {
		if left[i] == right[i] {
			continue
		}
		leftValue, leftNumeric := numericIdentifier(left[i])
		rightValue, rightNumeric := numericIdentifier(right[i])
		switch {
		case leftNumeric && rightNumeric:
			return sign(leftValue, rightValue)
		case leftNumeric:
			return -1
		case rightNumeric:
			return 1
		case left[i] < right[i]:
			return -1
		default:
			return 1
		}
	}
	return sign(uint64(len(left)), uint64(len(right)))
}

// numericIdentifier reports whether identifier is a numeric prerelease
// identifier, returning its value when it is.
func numericIdentifier(identifier string) (uint64, bool) {
	value, err := parseNumeric(identifier)
	if err != nil {
		return 0, false
	}
	return value, true
}

// parseNumeric parses a version component, rejecting anything that is not a
// plain run of digits — strconv.ParseUint alone would accept "+1" and "_1".
func parseNumeric(part string) (uint64, error) {
	if part == "" {
		return 0, errors.New("empty numeric component")
	}
	var value uint64
	for _, char := range part {
		if char < '0' || char > '9' {
			return 0, fmt.Errorf("non-numeric component %q", part)
		}
		digit := uint64(char - '0')
		if value > (1<<63)/10 {
			return 0, fmt.Errorf("numeric component %q out of range", part)
		}
		value = value*10 + digit
	}
	return value, nil
}

// sign returns -1, 0, or 1 as a orders before, with, or after b.
func sign(a, b uint64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
