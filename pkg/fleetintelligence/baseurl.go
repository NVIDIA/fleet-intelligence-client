// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package fleetintelligence

// This file owns the rules for what counts as an acceptable API base URL.
// Every request carries the NGC service key in an Authorization header, so a
// plaintext endpoint would put a long-lived credential on the wire. The
// validation lives here (rather than in the CLI) so it applies uniformly to
// the CLI, environment overrides, the on-disk config, and SDK embedders.

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// ErrInsecureBaseURL indicates the base URL would send the service key over an
// unencrypted connection. Plain http is accepted only for loopback hosts.
var ErrInsecureBaseURL = errors.New("insecure base URL: https is required")

// ValidateBaseURL reports whether rawURL is usable as a Fleet Intelligence API
// root. It must be an absolute https URL, except that plain http is permitted
// for loopback hosts (127.0.0.0/8, ::1, localhost) so local mock servers and
// tests keep working without exposing credentials off the machine.
func ValidateBaseURL(rawURL string) error {
	parsedURL, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return fmt.Errorf("invalid API URL: %w", err)
	}
	if !parsedURL.IsAbs() || parsedURL.Host == "" {
		return fmt.Errorf("invalid API URL %q: absolute https URL is required", rawURL)
	}

	switch strings.ToLower(parsedURL.Scheme) {
	case "https":
		return nil
	case "http":
		if isLoopbackHost(parsedURL.Hostname()) {
			return nil
		}
		return fmt.Errorf(
			"invalid API URL %q: %w (plain http is allowed only for localhost)",
			rawURL, ErrInsecureBaseURL,
		)
	default:
		return fmt.Errorf("invalid API URL %q: absolute https URL is required", rawURL)
	}
}

// isLoopbackHost reports whether host addresses the local machine. host is
// expected to come from url.Hostname(), which strips the port and IPv6
// brackets.
func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
