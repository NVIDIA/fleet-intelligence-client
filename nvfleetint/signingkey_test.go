// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Verifies the signing key is fetched from the well-known endpoint
func TestFetchSigningKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != signingKeyPath {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected authorization header: %q", got)
		}
		if got := r.Header.Get("Accept"); got != signingKeyAcceptHeader {
			t.Fatalf("unexpected accept header: %q", got)
		}
		_, _ = w.Write([]byte("-----BEGIN PUBLIC KEY-----\nkeydata\n-----END PUBLIC KEY-----\n"))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	key, err := client.FetchSigningKey(context.Background())
	if err != nil {
		t.Fatalf("fetch signing key failed: %v", err)
	}
	if !strings.Contains(string(key), "BEGIN PUBLIC KEY") {
		t.Fatalf("unexpected key contents: %q", string(key))
	}
}

// Verifies direct SDK requests use the same central retry behavior as generated
// API calls.
func TestFetchSigningKeyRetriesTransientFailure(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("public key"))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}
	retryer, ok := client.requestDoer.(*retryingDoer)
	if !ok {
		t.Fatalf("unexpected request doer: %T", client.requestDoer)
	}
	retryer.delay = func(int, *http.Response) time.Duration { return 0 }

	key, err := client.FetchSigningKey(context.Background())
	if err != nil {
		t.Fatalf("fetch signing key failed: %v", err)
	}
	if string(key) != "public key" || calls.Load() != 2 {
		t.Fatalf("unexpected key/calls: %q/%d", key, calls.Load())
	}
}

// Verifies a non-200 response from the key endpoint is surfaced as an error
func TestFetchSigningKeyError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	if _, err := client.FetchSigningKey(context.Background()); err == nil {
		t.Fatal("expected error for non-200 response")
	}
}
