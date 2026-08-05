// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// Builds nested JSON arrays of the requested depth wrapped in an object, so the
// document stays valid at any depth. The returned body nests depth+1 levels.
func nestedJSON(depth int) string {
	return `{"a":` + strings.Repeat("[", depth) + strings.Repeat("]", depth) + `}`
}

// Verifies the client applies the documented defaults
func TestNewClientDefaultsResponseLimits(t *testing.T) {
	client, err := NewClient("https://api.example.com", "key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}
	if client.maxResponseBytes != DefaultMaxResponseBytes {
		t.Fatalf("unexpected byte limit: %d", client.maxResponseBytes)
	}
	if client.maxJSONDepth != DefaultMaxJSONDepth {
		t.Fatalf("unexpected depth limit: %d", client.maxJSONDepth)
	}
}

// Verifies the limit options are applied and that non-positive values are
// ignored rather than disabling the guard
func TestResponseLimitOptions(t *testing.T) {
	client, err := NewClient("https://api.example.com", "key",
		WithMaxResponseBytes(1024), WithMaxJSONDepth(4))
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}
	if client.maxResponseBytes != 1024 || client.maxJSONDepth != 4 {
		t.Fatalf("options not applied: %d / %d", client.maxResponseBytes, client.maxJSONDepth)
	}

	unchanged, err := NewClient("https://api.example.com", "key",
		WithMaxResponseBytes(0), WithMaxJSONDepth(-1))
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}
	if unchanged.maxResponseBytes != DefaultMaxResponseBytes || unchanged.maxJSONDepth != DefaultMaxJSONDepth {
		t.Fatalf("non-positive limits were applied: %d / %d",
			unchanged.maxResponseBytes, unchanged.maxJSONDepth)
	}
}

// Verifies an oversized response is rejected instead of being buffered whole
func TestResponseByteLimitRejectsOversizedBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodesCount":` + strings.Repeat("1", 4096) + `}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key", WithMaxResponseBytes(256))
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	_, err = client.GetOverview(context.Background(), OverviewOptions{})
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("expected ErrResponseTooLarge, got %v", err)
	}
}

// Verifies a body at the limit still succeeds, so the guard is off-by-one safe
func TestResponseByteLimitAllowsBodyAtLimit(t *testing.T) {
	body := `{"nodesCount":10}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key", WithMaxResponseBytes(int64(len(body))))
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	got, err := client.GetOverview(context.Background(), OverviewOptions{})
	if err != nil {
		t.Fatalf("overview failed: %v", err)
	}
	if got.NodesCount == nil || *got.NodesCount != 10 {
		t.Fatalf("unexpected nodes count: %#v", got.NodesCount)
	}
}

// Verifies a declared Content-Length above the limit fails before the payload
// is transferred
func TestResponseByteLimitRejectsDeclaredLength(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		payload := []byte(`{"nodesCount":` + strings.Repeat("1", 4096) + `}`)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key", WithMaxResponseBytes(64))
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	_, err = client.GetOverview(context.Background(), OverviewOptions{})
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("expected ErrResponseTooLarge, got %v", err)
	}
	if !strings.Contains(err.Error(), "declared") {
		t.Fatalf("expected the declared-length path, got %v", err)
	}
}

// Verifies a chunked response with no declared length is still bounded
func TestResponseByteLimitRejectsUnboundedChunkedBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatalf("response writer is not a flusher")
		}
		_, _ = w.Write([]byte(`{"nodesCount":`))
		flusher.Flush()
		for range 64 {
			_, _ = w.Write([]byte(strings.Repeat("1", 512)))
			flusher.Flush()
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key", WithMaxResponseBytes(1024))
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	_, err = client.GetOverview(context.Background(), OverviewOptions{})
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("expected ErrResponseTooLarge, got %v", err)
	}
}

// Verifies a deeply nested JSON response is rejected before decoding
func TestResponseDepthLimitRejectsDeeplyNestedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(nestedJSON(200)))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	_, err = client.GetOverview(context.Background(), OverviewOptions{})
	if !errors.Is(err, ErrResponseTooDeep) {
		t.Fatalf("expected ErrResponseTooDeep, got %v", err)
	}
}

// Verifies a realistically nested response is untouched by the depth guard
func TestResponseDepthLimitAllowsOrdinaryPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodesCount":10,"metrics":[{"name":"gpu_utilization","value":42.5}]}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	got, err := client.GetOverview(context.Background(), OverviewOptions{})
	if err != nil {
		t.Fatalf("overview failed: %v", err)
	}
	if len(got.Metrics) != 1 {
		t.Fatalf("unexpected metrics: %#v", got.Metrics)
	}
}

// Verifies non-JSON payloads skip the depth scan, so brace-heavy binary report
// downloads are not mistaken for nested documents
func TestResponseDepthLimitSkipsNonJSONContentType(t *testing.T) {
	response := &http.Response{Header: http.Header{}}
	response.Header.Set("Content-Type", "application/zip")
	if got := responseDepthLimit(response, 8); got != 0 {
		t.Fatalf("expected depth checking to be skipped, got %d", got)
	}

	response.Header.Set("Content-Type", "application/json; charset=utf-8")
	if got := responseDepthLimit(response, 8); got != 8 {
		t.Fatalf("expected depth limit 8, got %d", got)
	}

	response.Header.Set("Content-Type", "application/problem+json")
	if got := responseDepthLimit(response, 8); got != 8 {
		t.Fatalf("expected depth limit 8 for a +json type, got %d", got)
	}

	if got := responseDepthLimit(response, 0); got != 0 {
		t.Fatalf("expected a non-positive limit to disable the scan, got %d", got)
	}
}

// Verifies the depth scanner ignores structural characters inside strings and
// tracks depth across chunk boundaries
func TestJSONDepthScanner(t *testing.T) {
	cases := []struct {
		name    string
		chunks  []string
		max     int
		wantErr bool
	}{
		{name: "at limit", chunks: []string{`{"a":[1,2]}`}, max: 2},
		{name: "over limit", chunks: []string{`{"a":[[1]]}`}, max: 2, wantErr: true},
		{name: "braces in string", chunks: []string{`{"a":"[[[[[[[[[["}`}, max: 1},
		{name: "escaped quote in string", chunks: []string{`{"a":"\"[[[[["}`}, max: 1},
		{name: "escaped backslash ends string", chunks: []string{`{"a":"x\\"}`}, max: 1},
		{name: "split across chunks", chunks: []string{`{"a":`, `[[1]]}`}, max: 2, wantErr: true},
		{name: "string split across chunks", chunks: []string{`{"a":"[[`, `[["}`}, max: 1},
		{name: "siblings do not accumulate", chunks: []string{`{"a":[1],"b":[2],"c":[3]}`}, max: 2},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			scanner := &jsonDepthScanner{max: testCase.max}
			var err error
			for _, chunk := range testCase.chunks {
				if err = scanner.scan([]byte(chunk)); err != nil {
					break
				}
			}
			if testCase.wantErr != (err != nil) {
				t.Fatalf("wantErr=%v, got %v", testCase.wantErr, err)
			}
			if testCase.wantErr && !errors.Is(err, ErrResponseTooDeep) {
				t.Fatalf("expected ErrResponseTooDeep, got %v", err)
			}
		})
	}
}

// Verifies the guarded body keeps failing once a limit is crossed, so an
// ignored error cannot yield a silently truncated document
func TestGuardedBodyErrorIsSticky(t *testing.T) {
	body := newGuardedBody(io.NopCloser(strings.NewReader(strings.Repeat("x", 64))), 8, 0)

	buf := make([]byte, 32)
	if _, err := body.Read(buf); !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("expected ErrResponseTooLarge, got %v", err)
	}
	n, err := body.Read(buf)
	if n != 0 || !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("expected the error to stick, got n=%d err=%v", n, err)
	}
	if err := body.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}
}

// Verifies the signing key fetch, which reads its body outside the generated
// client, is bounded too
func TestFetchSigningKeyIsBounded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-pem-file")
		_, _ = w.Write([]byte(strings.Repeat("A", 8192)))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key", WithMaxResponseBytes(128))
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	if _, err := client.FetchSigningKey(context.Background()); !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("expected ErrResponseTooLarge, got %v", err)
	}
}
