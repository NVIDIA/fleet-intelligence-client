// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"errors"
	"net/url"
	"strings"
	"testing"
)

// Verifies base URL validation
func TestNewClientRequiresBaseURL(t *testing.T) {
	_, err := NewClient("", "key")
	if err != ErrMissingBaseURL {
		t.Fatalf("expected ErrMissingBaseURL, got %v", err)
	}
}

// Verifies API key validation
func TestNewClientRequiresAPIKey(t *testing.T) {
	_, err := NewClient("https://example.com", "")
	if err != ErrMissingAPIKey {
		t.Fatalf("expected ErrMissingAPIKey, got %v", err)
	}
}

// Verifies plaintext remote endpoints are rejected before any request is made
func TestNewClientRejectsInsecureBaseURL(t *testing.T) {
	_, err := NewClient("http://example.com", "key")
	if !errors.Is(err, ErrInsecureBaseURL) {
		t.Fatalf("expected ErrInsecureBaseURL, got %v", err)
	}
}

// Verifies plaintext loopback endpoints remain usable for local development
func TestNewClientAllowsInsecureLoopbackBaseURL(t *testing.T) {
	if _, err := NewClient("http://127.0.0.1:8080", "key"); err != nil {
		t.Fatalf("expected loopback http to be accepted, got %v", err)
	}
}

// Verifies client configuration accessors
func TestNewClientStoresConfiguration(t *testing.T) {
	client, err := NewClient("https://example.com", "key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	if client.BaseURL() != "https://example.com" {
		t.Fatalf("unexpected base URL: %q", client.BaseURL())
	}

	if !client.APIKeyConfigured() {
		t.Fatal("expected API key to be configured")
	}
}

func TestValidateStepDurationBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name    string
		step    string
		wantErr string
	}{
		{name: "maximum duration", step: "9223372036854775807ns"},
		{name: "nanosecond overflow", step: "9223372036854775808ns", wantErr: `invalid step "9223372036854775808ns": duration is too large; maximum is 2562047h47m16.854775807s`},
		{name: "compound overflow", step: "2562047h47m16.854775808s", wantErr: `invalid step "2562047h47m16.854775808s": duration is too large; maximum is 2562047h47m16.854775807s`},
		{name: "one minute", step: "1m"},
		{name: "below minimum", step: "59s", wantErr: `invalid step "59s": expected at least 1m`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateStep(tc.step)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateStep(%q) returned %v; want nil", tc.step, err)
				}
			} else if err == nil || err.Error() != tc.wantErr {
				t.Fatalf("ValidateStep(%q) returned %v; want %q", tc.step, err, tc.wantErr)
			}
		})
	}
}

func TestMalformedBaseURLParseErrors(t *testing.T) {
	for _, tc := range []struct {
		name    string
		baseURL string
	}{
		{name: "invalid percent escape", baseURL: "  https://example.com/%zz  "},
		{name: "invalid port", baseURL: "https://example.com:bad"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			validateErr := ValidateBaseURL(tc.baseURL)
			optionCalled := false
			client, clientErr := NewClient(tc.baseURL, "test-key", func(*Client) {
				optionCalled = true
			})
			if client != nil || optionCalled {
				t.Errorf("NewClient returned client %v and called option %t; want nil and false", client, optionCalled)
			}
			for name, err := range map[string]error{"ValidateBaseURL": validateErr, "NewClient": clientErr} {
				var parseErr *url.Error
				if err == nil || !strings.HasPrefix(err.Error(), "invalid API URL:") || !errors.As(err, &parseErr) || errors.Is(err, ErrInsecureBaseURL) {
					t.Errorf("%s returned %v; want wrapped URL parse error with invalid API URL prefix", name, err)
				}
			}
		})
	}
}

func TestBaseURLMissingClientOrURL(t *testing.T) {
	var nilClient *Client
	for _, tc := range []struct {
		name   string
		client *Client
	}{
		{name: "nil client", client: nilClient},
		{name: "nil base URL", client: &Client{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.client.BaseURL(); got != "" {
				t.Fatalf("BaseURL() = %q, want empty string", got)
			}
		})
	}
}
