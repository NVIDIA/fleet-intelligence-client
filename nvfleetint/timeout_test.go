// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

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

func TestRequestContextNonPositiveTimeout(t *testing.T) {
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
