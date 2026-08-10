// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
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

// Stands in for a custom transport implementation
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// Verifies transient HTTP failures retry the same idempotent request and return
// the first successful response.
func TestRetryingDoerRetriesTransientStatus(t *testing.T) {
	var calls atomic.Int32
	inner := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		call := calls.Add(1)
		status := http.StatusServiceUnavailable
		if call == defaultRequestAttempts {
			status = http.StatusOK
		}
		return &http.Response{
			StatusCode: status,
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewBufferString(http.StatusText(status))),
			Request:    req,
		}, nil
	})}
	doer := &retryingDoer{
		inner:       inner,
		maxAttempts: defaultRequestAttempts,
		delay:       func(int, *http.Response) time.Duration { return 0 },
	}

	req, err := http.NewRequest(http.MethodGet, "https://example.com/v1/nodes?page=4", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	response, err := doer.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", response.StatusCode)
	}
	if got := calls.Load(); got != defaultRequestAttempts {
		t.Fatalf("expected %d attempts, got %d", defaultRequestAttempts, got)
	}
}

// Verifies permanent HTTP failures are returned without retrying.
func TestRetryingDoerDoesNotRetryPermanentStatus(t *testing.T) {
	var calls atomic.Int32
	inner := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewBufferString("bad request")),
			Request:    req,
		}, nil
	})}
	doer := &retryingDoer{inner: inner, maxAttempts: defaultRequestAttempts}

	req, err := http.NewRequest(http.MethodGet, "https://example.com/v1/nodes", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	response, err := doer.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadRequest || calls.Load() != 1 {
		t.Fatalf("unexpected response status/calls: %d/%d", response.StatusCode, calls.Load())
	}
}

// Verifies Retry-After supports both seconds and HTTP-date forms.
func TestResponseRetryAfter(t *testing.T) {
	now := time.Date(2026, time.August, 5, 12, 0, 0, 0, time.UTC)
	maximumDelay := time.Duration(maximumRetryAfterSecs) * time.Second
	tests := []struct {
		name string
		raw  string
		want time.Duration
		ok   bool
	}{
		{name: "seconds", raw: "7", want: 7 * time.Second, ok: true},
		{name: "maximum seconds", raw: strconv.FormatInt(maximumRetryAfterSecs, 10), want: maximumDelay, ok: true},
		{name: "overflowing seconds", raw: strconv.FormatInt(maximumRetryAfterSecs+1, 10), ok: false},
		{name: "date", raw: now.Add(11 * time.Second).Format(http.TimeFormat), want: 11 * time.Second, ok: true},
		{name: "past date", raw: now.Add(-time.Second).Format(http.TimeFormat), want: 0, ok: true},
		{name: "invalid", raw: "later", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := &http.Response{Header: http.Header{"Retry-After": []string{tt.raw}}}
			got, ok := responseRetryAfter(response, now)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("unexpected retry delay: got %v/%t want %v/%t", got, ok, tt.want, tt.ok)
			}
		})
	}
}

// Verifies an oversized Retry-After value falls back to a positive bounded
// delay, so the retry wait cannot be bypassed by duration overflow.
func TestOversizedRetryAfterUsesBackoff(t *testing.T) {
	response := &http.Response{Header: http.Header{
		"Retry-After": []string{strconv.FormatInt(maximumRetryAfterSecs+1, 10)},
	}}
	delay := defaultRetryDelay(1, response)
	if delay <= 0 {
		t.Fatalf("expected positive fallback delay, got %v", delay)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := waitForRetry(ctx, delay); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected retry wait to honor cancellation, got %v", err)
	}
}

// Verifies a canceled request interrupts retry backoff.
func TestWaitForRetryHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := waitForRetry(ctx, time.Minute); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
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

// Verifies the default HTTP client also carries the redirect guard
func TestDefaultHTTPClientInstallsRedirectGuard(t *testing.T) {
	client, err := NewClient("https://example.com", "key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}
	if client.httpClient.CheckRedirect == nil {
		t.Fatal("expected the redirect guard to be installed")
	}
}

// Verifies an https -> http redirect is refused rather than followed, so the
// bearer token never reaches a cleartext connection.
func TestGuardRedirectRejectsHTTPSDowngrade(t *testing.T) {
	via := []*http.Request{{URL: mustParseURL(t, "https://api.example.com/v1/nodes")}}
	req := &http.Request{URL: mustParseURL(t, "http://api.example.com/v1/nodes"), Header: http.Header{}}
	req.Header.Set("Authorization", "Bearer secret")

	err := guardRedirect(req, via)
	if err == nil {
		t.Fatal("expected the downgrade to be refused")
	}
	if !errors.Is(err, ErrInsecureBaseURL) {
		t.Fatalf("expected ErrInsecureBaseURL, got %v", err)
	}
	if strings.Contains(err.Error(), "/v1/nodes") {
		t.Fatalf("expected the request path to stay out of the error, got %q", err)
	}
}

// Verifies the Authorization header is dropped on any cross-origin hop,
// including the same-host cases net/http's own check lets through.
func TestGuardRedirectStripsAuthorizationCrossOrigin(t *testing.T) {
	cases := []struct {
		name     string
		from     string
		to       string
		stripped bool
	}{
		{name: "same origin", from: "https://api.example.com/a", to: "https://api.example.com/b"},
		{name: "default port is implied", from: "https://api.example.com/a", to: "https://api.example.com:443/b"},
		{name: "different host", from: "https://api.example.com/a", to: "https://evil.example.net/b", stripped: true},
		{name: "subdomain", from: "https://example.com/a", to: "https://sub.example.com/b", stripped: true},
		{name: "different port", from: "https://api.example.com/a", to: "https://api.example.com:8443/b", stripped: true},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			via := []*http.Request{{URL: mustParseURL(t, testCase.from)}}
			req := &http.Request{URL: mustParseURL(t, testCase.to), Header: http.Header{}}
			req.Header.Set("Authorization", "Bearer secret")

			if err := guardRedirect(req, via); err != nil {
				t.Fatalf("guard redirect failed: %v", err)
			}

			got := req.Header.Get("Authorization")
			if testCase.stripped && got != "" {
				t.Fatalf("expected the Authorization header to be stripped, got %q", got)
			}
			if !testCase.stripped && got == "" {
				t.Fatal("expected the Authorization header to be preserved")
			}
		})
	}
}

// Verifies a same-origin redirect is still followed with credentials intact,
// end to end through the SDK.
func TestSameOriginRedirectKeepsAuthorization(t *testing.T) {
	var authorized []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorized = append(authorized, r.Header.Get("Authorization"))
		if r.URL.Path != "/moved" {
			http.Redirect(w, r, "/moved", http.StatusTemporaryRedirect)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodes":[]}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}
	if _, err := client.ListNodes(context.Background(), ListNodesOptions{}); err != nil {
		t.Fatalf("list nodes failed: %v", err)
	}

	if len(authorized) != 2 {
		t.Fatalf("expected the redirect to be followed, got %d requests", len(authorized))
	}
	for i, header := range authorized {
		if header != "Bearer test-key" {
			t.Fatalf("request %d lost its Authorization header: %q", i, header)
		}
	}
}

// Verifies the guard restates net/http's redirect cap, which installing a
// CheckRedirect would otherwise remove.
func TestRedirectGuardEnforcesRedirectLimit(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.Redirect(w, r, "/next", http.StatusTemporaryRedirect)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}
	if _, err := client.ListNodes(context.Background(), ListNodesOptions{}); err == nil {
		t.Fatal("expected the redirect chain to be stopped")
	}
	if requests > maxRedirects+1 {
		t.Fatalf("expected at most %d requests, got %d", maxRedirects+1, requests)
	}
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	return parsed
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
