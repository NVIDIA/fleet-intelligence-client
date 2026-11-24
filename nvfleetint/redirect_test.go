// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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

func TestGuardRedirectHTTPLoopbackOriginBoundaries(t *testing.T) {
	const authorization = "Bearer secret"
	for _, tc := range []struct {
		name     string
		via      []string
		target   string
		wantAuth string
	}{
		{name: "implicit to explicit default port", via: []string{"http://localhost/path"}, target: "http://localhost:80/next", wantAuth: authorization},
		{name: "explicit to implicit default port", via: []string{"http://localhost:80/path"}, target: "http://localhost/next", wantAuth: authorization},
		{name: "different port", via: []string{"http://localhost/path"}, target: "http://localhost:8080/next"},
		{name: "multi hop compares original origin", via: []string{"http://localhost", "http://localhost:8080"}, target: "http://localhost:8080/next"},
		{name: "empty via", target: "http://localhost:8080/next", wantAuth: authorization},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := &http.Request{URL: mustParseURL(t, tc.target), Header: make(http.Header)}
			req.Header.Set("Authorization", authorization)
			var via []*http.Request
			for _, raw := range tc.via {
				via = append(via, &http.Request{URL: mustParseURL(t, raw)})
			}
			if err := guardRedirect(req, via); err != nil {
				t.Fatalf("guardRedirect() error = %v; want nil", err)
			}
			if got := req.Header.Get("Authorization"); got != tc.wantAuth {
				t.Errorf("Authorization = %q; want %q", got, tc.wantAuth)
			}
		})
	}
}
