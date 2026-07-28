// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package fleetintelligence

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Verifies base URL validation
func TestNewClientRequiresBaseURL(t *testing.T) {
	_, err := NewClient("", "key")
	if err != ErrMissingBaseURL {
		t.Fatalf("expected ErrMissingBaseURL, got %v", err)
	}
}

// Verifies service key validation
func TestNewClientRequiresServiceKey(t *testing.T) {
	_, err := NewClient("https://example.com", "")
	if err != ErrMissingServiceKey {
		t.Fatalf("expected ErrMissingServiceKey, got %v", err)
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

// Verifies the default HTTP client pins a TLS floor
func TestNewClientDefaultsTLSMinVersion(t *testing.T) {
	client, err := NewClient("https://example.com", "key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	transport, ok := client.httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", client.httpClient.Transport)
	}
	if transport.TLSClientConfig == nil {
		t.Fatal("expected a TLS config on the default transport")
	}
	if transport.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("unexpected TLS MinVersion: %d", transport.TLSClientConfig.MinVersion)
	}
	// Cloning the standard transport must preserve its defaults.
	if transport.Proxy == nil {
		t.Fatal("expected the cloned transport to retain proxy support")
	}
}

// Verifies clients without a caller-supplied HTTP client share one transport,
// so they also share its connection pool and TLS session cache
func TestNewClientSharesDefaultTransport(t *testing.T) {
	first, err := NewClient("https://example.com", "key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}
	second, err := NewClient("https://example.com", "key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	if first.httpClient.Transport != second.httpClient.Transport {
		t.Fatal("expected both clients to share the default transport")
	}
}

// Verifies transport hardening preserves a stricter configured TLS floor
func TestHardenedTransportPreservesStricterTLSMinVersion(t *testing.T) {
	hardened := hardenedTransport(&http.Transport{
		Proxy: http.ProxyFromEnvironment,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS13,
		},
	})

	if hardened == nil {
		t.Fatal("expected a hardened transport")
	}
	if hardened.TLSClientConfig.MinVersion != tls.VersionTLS13 {
		t.Fatalf("expected TLS 1.3 MinVersion to be preserved, got %d", hardened.TLSClientConfig.MinVersion)
	}
}

// Verifies transport hardening supplies a TLS config when the base lacks one
func TestHardenedTransportAddsMissingTLSConfig(t *testing.T) {
	hardened := hardenedTransport(&http.Transport{})

	if hardened == nil {
		t.Fatal("expected a hardened transport")
	}
	if hardened.TLSClientConfig == nil {
		t.Fatal("expected a TLS config to be added")
	}
	if hardened.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("unexpected TLS MinVersion: %d", hardened.TLSClientConfig.MinVersion)
	}
}

// Verifies transport hardening declines round trippers it cannot clone
func TestHardenedTransportRejectsForeignRoundTripper(t *testing.T) {
	if hardened := hardenedTransport(roundTripperFunc(nil)); hardened != nil {
		t.Fatalf("expected nil for a non-*http.Transport base, got %#v", hardened)
	}
}

// Stands in for a custom transport implementation
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
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

	if !client.ServiceKeyConfigured() {
		t.Fatal("expected service key to be configured")
	}
}

// Verifies custom HTTP client injection
func TestNewClientUsesHTTPClientOption(t *testing.T) {
	customHTTPClient := &http.Client{}
	client, err := NewClient("https://example.com", "key", WithHTTPClient(customHTTPClient))
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}
	if client.httpClient != customHTTPClient {
		t.Fatal("expected custom HTTP client to be configured")
	}
}

// Verifies the default per-request timeout is applied
func TestNewClientDefaultsTimeout(t *testing.T) {
	client, err := NewClient("https://example.com", "key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}
	if client.timeout != DefaultTimeout {
		t.Fatalf("expected default timeout %v, got %v", DefaultTimeout, client.timeout)
	}
}

// Verifies WithTimeout overrides the default and ignores non-positive values
func TestWithTimeoutOption(t *testing.T) {
	client, err := NewClient("https://example.com", "key", WithTimeout(5*time.Second))
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}
	if client.timeout != 5*time.Second {
		t.Fatalf("expected timeout 5s, got %v", client.timeout)
	}

	ignored, err := NewClient("https://example.com", "key", WithTimeout(0))
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}
	if ignored.timeout != DefaultTimeout {
		t.Fatalf("expected non-positive timeout to be ignored, got %v", ignored.timeout)
	}
}

// Verifies the timeout is enforced via the request context rather than the
// HTTP client, so it still applies when a shared client is supplied via
// WithHTTPClient.
func TestTimeoutEnforcedWithSharedHTTPClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(time.Second):
			w.WriteHeader(http.StatusOK)
		case <-r.Context().Done():
		}
	}))
	defer server.Close()

	// A shared client with no Timeout of its own; the timeout must come from
	// the per-request context so sharing it never leaks a deadline.
	shared := &http.Client{}
	client, err := NewClient(server.URL, "test-key", WithHTTPClient(shared), WithTimeout(20*time.Millisecond))
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	_, err = client.ListNodes(context.Background(), ListNodesOptions{})
	if err == nil {
		t.Fatal("expected a timeout error")
	}
	if msg := err.Error(); !strings.Contains(msg, "request timed out") {
		t.Fatalf("expected friendly timeout message, got %q", msg)
	}
	// The raw transport error leaks the request URL and "context deadline
	// exceeded"; neither should reach the caller.
	if msg := err.Error(); strings.Contains(msg, "context deadline exceeded") || strings.Contains(msg, server.URL) {
		t.Fatalf("expected raw transport error to be hidden, got %q", msg)
	}

	if shared.Timeout != 0 {
		t.Fatalf("expected shared HTTP client timeout to remain unset, got %v", shared.Timeout)
	}
}
