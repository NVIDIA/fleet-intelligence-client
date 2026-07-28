// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package fleetintelligence

import (
	"errors"
	"testing"
)

func TestValidateBaseURLAcceptsSecureAndLoopbackURLs(t *testing.T) {
	cases := []string{
		"https://api.example.com",
		"https://api.example.com/prefix",
		"HTTPS://API.EXAMPLE.COM",
		"  https://api.example.com  ",
		"http://127.0.0.1:8080",
		"http://127.0.0.2",
		"http://[::1]:8080",
		"http://localhost:9000",
		"HTTP://LOCALHOST",
	}

	for _, rawURL := range cases {
		if err := ValidateBaseURL(rawURL); err != nil {
			t.Errorf("ValidateBaseURL(%q) = %v, want nil", rawURL, err)
		}
	}
}

func TestValidateBaseURLRejectsInsecureRemoteURLs(t *testing.T) {
	cases := []string{
		"http://api.example.com",
		"http://192.168.1.10",
		"http://localhost.evil.example.com",
		"http://[2001:db8::1]:8080",
	}

	for _, rawURL := range cases {
		err := ValidateBaseURL(rawURL)
		if err == nil {
			t.Errorf("ValidateBaseURL(%q) = nil, want error", rawURL)
			continue
		}
		if !errors.Is(err, ErrInsecureBaseURL) {
			t.Errorf("ValidateBaseURL(%q) = %v, want ErrInsecureBaseURL", rawURL, err)
		}
	}
}

func TestValidateBaseURLRejectsMalformedURLs(t *testing.T) {
	cases := []string{
		"example.com",
		"/v1/nodes",
		"https://",
		"ftp://api.example.com",
		"file:///etc/passwd",
	}

	for _, rawURL := range cases {
		err := ValidateBaseURL(rawURL)
		if err == nil {
			t.Errorf("ValidateBaseURL(%q) = nil, want error", rawURL)
			continue
		}
		// Malformed URLs are a shape problem, not a transport-security one.
		if errors.Is(err, ErrInsecureBaseURL) {
			t.Errorf("ValidateBaseURL(%q) = %v, want a non-insecure error", rawURL, err)
		}
	}
}

func TestIsLoopbackHost(t *testing.T) {
	loopback := []string{"localhost", "LocalHost", "127.0.0.1", "127.5.6.7", "::1"}
	for _, host := range loopback {
		if !isLoopbackHost(host) {
			t.Errorf("isLoopbackHost(%q) = false, want true", host)
		}
	}

	remote := []string{"", "example.com", "10.0.0.1", "0.0.0.0", "2001:db8::1", "localhost.example.com"}
	for _, host := range remote {
		if isLoopbackHost(host) {
			t.Errorf("isLoopbackHost(%q) = true, want false", host)
		}
	}
}
