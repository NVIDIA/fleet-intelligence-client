// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Verifies a body ending exactly at the cap reads through untouched
func TestLimitedBodyAllowsBodyAtLimit(t *testing.T) {
	payload := strings.Repeat("a", 64)
	body := newLimitedBody(io.NopCloser(strings.NewReader(payload)), int64(len(payload)))

	read, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read at limit failed: %v", err)
	}
	if string(read) != payload {
		t.Fatalf("unexpected body: %q", read)
	}
	if err := body.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}
}

// Verifies a body one byte past the cap is refused, that no more than the cap
// is ever handed to the caller, and that the failure latches so a caller
// cannot read further by retrying.
func TestLimitedBodyRejectsBodyPastLimit(t *testing.T) {
	const maxBytes = 64
	body := newLimitedBody(io.NopCloser(strings.NewReader(strings.Repeat("a", maxBytes+1))), maxBytes)

	read, err := io.ReadAll(body)
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("expected ErrResponseTooLarge, got %v", err)
	}
	// The byte read to detect the overrun must not reach the caller.
	if len(read) != maxBytes {
		t.Fatalf("read %d bytes, want %d", len(read), maxBytes)
	}
	if _, err := body.Read(make([]byte, 1)); !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("expected latched ErrResponseTooLarge, got %v", err)
	}
}

// Verifies the limit is reported in bytes, so the tighter signing key cap is
// not described using the general response ceiling.
func TestLimitedBodyErrorNamesItsOwnLimit(t *testing.T) {
	body := newLimitedBody(io.NopCloser(strings.NewReader("abcd")), 2)

	_, err := io.ReadAll(body)
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("expected ErrResponseTooLarge, got %v", err)
	}
	if !strings.Contains(err.Error(), "2 bytes") {
		t.Fatalf("error does not name its limit: %v", err)
	}
}

// Verifies a body that never reaches the cap is unaffected by the limiting
// wrapper, and that a body past it fails before a parser can buffer it.
func TestLimitingDoerBoundsResponseBody(t *testing.T) {
	inner := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewBufferString(strings.Repeat("x", 128))),
			Request:    req,
		}, nil
	})}

	tests := []struct {
		name     string
		maxBytes int64
		wantErr  bool
	}{
		{name: "within limit", maxBytes: 256},
		{name: "past limit", maxBytes: 32, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			doer := &limitingDoer{inner: inner, maxBytes: test.maxBytes}
			req, err := http.NewRequest(http.MethodGet, "https://example.com/v1/overview", nil)
			if err != nil {
				t.Fatalf("build request: %v", err)
			}

			resp, err := doer.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			read, err := io.ReadAll(resp.Body)
			if test.wantErr {
				if !errors.Is(err, ErrResponseTooLarge) {
					t.Fatalf("expected ErrResponseTooLarge, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("read failed: %v", err)
			}
			if len(read) != 128 {
				t.Fatalf("unexpected body length: %d", len(read))
			}
		})
	}
}

// Verifies the cap is installed beneath the retry wrapper, so every attempt is
// bounded on its own and direct requestDoer users inherit it.
func TestNewClientInstallsResponseSizeLimit(t *testing.T) {
	client, err := NewClient("https://api.example.com", "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	retryer, ok := client.requestDoer.(*retryingDoer)
	if !ok {
		t.Fatalf("unexpected request doer: %T", client.requestDoer)
	}
	limiter, ok := retryer.inner.(*limitingDoer)
	if !ok {
		t.Fatalf("expected retries to wrap a limiting doer, got %T", retryer.inner)
	}
	if limiter.maxBytes != maxResponseBytes {
		t.Fatalf("unexpected response cap: %d", limiter.maxBytes)
	}
	if limiter.inner != client.httpClient {
		t.Fatalf("limiting doer does not wrap the client's HTTP client")
	}
}

// Verifies an oversized API response is refused before the generated parser
// buffers it, rather than being read into memory in full.
func TestAPIResponseSizeLimitRefusesOversizedBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodesCount":` + strings.Repeat("0", 4096) + `}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}
	// The shipped cap is far larger than a test can practically serve, so
	// tighten it in place to exercise the same code path.
	retryer, ok := client.requestDoer.(*retryingDoer)
	if !ok {
		t.Fatalf("unexpected request doer: %T", client.requestDoer)
	}
	retryer.inner = &limitingDoer{inner: client.httpClient, maxBytes: 512}

	if _, err := client.GetOverview(context.Background(), OverviewOptions{}); !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("expected ErrResponseTooLarge, got %v", err)
	}
}

// Verifies the signing key download is held to its own tighter cap
func TestFetchSigningKeyRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		for written := int64(0); written <= maxSigningKeyBytes; written += 64 << 10 {
			if _, err := w.Write(bytes.Repeat([]byte("k"), 64<<10)); err != nil {
				return
			}
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	if _, err := client.FetchSigningKey(context.Background()); !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("expected ErrResponseTooLarge, got %v", err)
	}
}
