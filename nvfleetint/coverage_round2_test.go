// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
	"time"
)

func TestRound2RetryRequiresResponseWithoutError(t *testing.T) {
	get, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	if canRetryRequest(get, nil, nil) {
		t.Fatal("GET with neither response nor error must not be retried")
	}

	head, err := http.NewRequest(http.MethodHead, "https://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !canRetryRequest(head, &http.Response{StatusCode: http.StatusServiceUnavailable}, nil) {
		t.Fatal("HEAD with a 503 response must be retried")
	}
}

func TestRound2SignedReportNilResponse(t *testing.T) {
	err := validateSignedReportContentType(nil)
	if err == nil || err.Error() != "signed inventory report response missing content type" {
		t.Fatalf("validateSignedReportContentType(nil) = %v, want missing content type error", err)
	}
	if got := signedReportFilename(nil); got != "inventory-report.zip" {
		t.Fatalf("signedReportFilename(nil) = %q, want inventory-report.zip", got)
	}
}

func TestRound2DecodeErrorReportUnsupportedView(t *testing.T) {
	got, err := decodeErrorReport([]byte(`{}`), "unknown", "")
	if err == nil || err.Error() != `invalid error report view "unknown"` {
		t.Fatalf("decodeErrorReport error = %v, want invalid view error", err)
	}
	if !reflect.DeepEqual(got, ErrorReport{}) {
		t.Fatalf("decodeErrorReport report = %#v, want zero ErrorReport", got)
	}
}

func TestRound2ReportRawValueString(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
		want string
	}{
		{name: "malformed", raw: "  [1,  ", want: "[1,"},
		{name: "compact JSON", raw: `  [1, 2]  `, want: `[1,2]`},
		{name: "JSON string", raw: `  "hello\nworld"  `, want: "hello\nworld"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := reportRawValueString([]byte(tc.raw)); got != tc.want {
				t.Fatalf("reportRawValueString(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestRound2RequestContextNonPositiveTimeout(t *testing.T) {
	type contextKey struct{}
	for _, tc := range []struct {
		name    string
		timeout time.Duration
	}{
		{name: "zero", timeout: 0},
		{name: "negative", timeout: -time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := context.WithValue(context.Background(), contextKey{}, "known value")
			client := &Client{timeout: tc.timeout}
			got, cancel := client.requestContext(original)
			if got != original {
				t.Fatal("requestContext did not return the original context")
			}
			if _, ok := got.Deadline(); ok {
				t.Fatal("requestContext added a deadline")
			}
			if got.Value(contextKey{}) != "known value" {
				t.Fatal("requestContext lost the context value")
			}
			cancel()
			if got.Err() != nil || got.Value(contextKey{}) != "known value" {
				t.Fatal("cancel made the original context unusable")
			}
		})
	}
}

func TestRound2BaseURLMissingClientOrURL(t *testing.T) {
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

func TestRound2DefaultHTTPClientNilSharedTransport(t *testing.T) {
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
func TestRound2HTTP1TransportTLSFloor(t *testing.T) {
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
