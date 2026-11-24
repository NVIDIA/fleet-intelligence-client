// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"crypto/tls"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

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
		return
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
		return
	}
	if hardened.TLSClientConfig == nil {
		t.Fatal("expected a TLS config to be added")
		return
	}
	if hardened.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("unexpected TLS MinVersion: %d", hardened.TLSClientConfig.MinVersion)
	}
}

// Verifies transport hardening bounds the connection pool, which net/http
// leaves unlimited per host by default
func TestHardenedTransportBoundsConnectionPool(t *testing.T) {
	hardened := hardenedTransport(&http.Transport{})

	if hardened == nil {
		t.Fatal("expected a hardened transport")
		return
	}
	if hardened.MaxConnsPerHost != maxConnsPerHost {
		t.Fatalf("unexpected MaxConnsPerHost: got %d, want %d", hardened.MaxConnsPerHost, maxConnsPerHost)
	}
	if hardened.MaxIdleConns != maxIdleConns {
		t.Fatalf("unexpected MaxIdleConns: got %d, want %d", hardened.MaxIdleConns, maxIdleConns)
	}
	if hardened.MaxIdleConnsPerHost != maxIdleConnsPerHost {
		t.Fatalf("unexpected MaxIdleConnsPerHost: got %d, want %d", hardened.MaxIdleConnsPerHost, maxIdleConnsPerHost)
	}
}

// Verifies transport hardening keeps a caller's tighter connection limits
func TestHardenedTransportPreservesTighterConnectionLimits(t *testing.T) {
	hardened := hardenedTransport(&http.Transport{
		MaxConnsPerHost:     4,
		MaxIdleConns:        3,
		MaxIdleConnsPerHost: 2,
	})

	if hardened == nil {
		t.Fatal("expected a hardened transport")
		return
	}
	if hardened.MaxConnsPerHost != 4 {
		t.Fatalf("unexpected MaxConnsPerHost: got %d, want 4", hardened.MaxConnsPerHost)
	}
	if hardened.MaxIdleConns != 3 {
		t.Fatalf("unexpected MaxIdleConns: got %d, want 3", hardened.MaxIdleConns)
	}
	if hardened.MaxIdleConnsPerHost != 2 {
		t.Fatalf("unexpected MaxIdleConnsPerHost: got %d, want 2", hardened.MaxIdleConnsPerHost)
	}
}

// Verifies the shared default transport carries the connection pool bounds
func TestDefaultHTTPClientBoundsConnectionPool(t *testing.T) {
	transport, ok := defaultHTTPClient().Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected an *http.Transport, got %T", defaultHTTPClient().Transport)
		return
	}

	if transport.MaxConnsPerHost != maxConnsPerHost {
		t.Fatalf("unexpected MaxConnsPerHost: got %d, want %d", transport.MaxConnsPerHost, maxConnsPerHost)
	}
	if transport.MaxIdleConnsPerHost != maxIdleConnsPerHost {
		t.Fatalf("unexpected MaxIdleConnsPerHost: got %d, want %d", transport.MaxIdleConnsPerHost, maxIdleConnsPerHost)
	}
}

// Verifies transport hardening declines round trippers it cannot clone
func TestHardenedTransportRejectsForeignRoundTripper(t *testing.T) {
	if hardened := hardenedTransport(roundTripperFunc(nil)); hardened != nil {
		t.Fatalf("expected nil for a non-*http.Transport base, got %#v", hardened)
	}
}

// Verifies custom HTTP client injection. The supplied client is used through a
// shallow copy so the redirect guard can be installed without mutating a client
// the caller may share; everything else must carry over.
func TestNewClientUsesHTTPClientOption(t *testing.T) {
	transport := &http.Transport{}
	customHTTPClient := &http.Client{Transport: transport, Timeout: 7 * time.Second}
	client, err := NewClient("https://example.com", "key", WithHTTPClient(customHTTPClient))
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}
	if client.httpClient.Transport != transport {
		t.Fatal("expected custom HTTP client transport to be configured")
	}
	if client.httpClient.Timeout != 7*time.Second {
		t.Fatalf("expected supplied timeout to carry over, got %v", client.httpClient.Timeout)
	}
	if client.httpClient.CheckRedirect == nil {
		t.Fatal("expected the redirect guard to be installed")
	}
	if customHTTPClient.CheckRedirect != nil {
		t.Fatal("expected the caller's HTTP client to be left unmutated")
	}
}

func TestDefaultHTTPClientNilSharedTransport(t *testing.T) {
	defaultHTTPClient() // Complete initialization before temporarily replacing the transport.
	original := sharedTransport
	sharedTransport = nil
	t.Cleanup(func() { sharedTransport = original })

	client := defaultHTTPClient()
	if client.Transport != nil {
		t.Fatalf("Transport = %v, want nil", client.Transport)
	}
	if client.CheckRedirect == nil {
		t.Fatal("CheckRedirect is nil")
	}
	origin := httptest.NewRequest(http.MethodGet, "https://example.com", nil)
	redirect := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
	if err := client.CheckRedirect(redirect, []*http.Request{origin}); !errors.Is(err, ErrInsecureBaseURL) {
		t.Fatalf("CheckRedirect error = %v, want ErrInsecureBaseURL", err)
	}
}

// HTTP/1-only clients still receive the TLS floor without mutating their base.
func TestHTTP1TransportTLSFloor(t *testing.T) {
	t.Setenv("GODEBUG", os.Getenv("GODEBUG")+",http2client=0")
	base := &http.Transport{}
	got := hardenedTransport(base)
	if got == nil || got == base {
		t.Fatal("expected a distinct hardened transport")
	}
	if got.TLSClientConfig == nil || got.TLSClientConfig.MinVersion < tls.VersionTLS12 {
		t.Fatalf("HTTP/1 transport TLS config = %#v, want TLS 1.2 or newer", got.TLSClientConfig)
	}
	if base.TLSClientConfig != nil {
		t.Fatalf("caller transport was mutated: %#v", base.TLSClientConfig)
	}
	if got.MaxConnsPerHost != maxConnsPerHost || got.MaxIdleConnsPerHost != maxIdleConnsPerHost {
		t.Fatalf("connection limits = %d/%d, want %d/%d", got.MaxConnsPerHost, got.MaxIdleConnsPerHost, maxConnsPerHost, maxIdleConnsPerHost)
	}
}
