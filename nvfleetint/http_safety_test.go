// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestRetryingDoerDoesNotReplayGETBody(t *testing.T) {
	calls := 0
	original := &http.Response{StatusCode: http.StatusServiceUnavailable, Body: io.NopCloser(strings.NewReader("unavailable")), Header: make(http.Header)}
	transport := roundTripperFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return original, nil
	})
	doer := &retryingDoer{inner: &http.Client{Transport: transport}, maxAttempts: defaultRequestAttempts}
	req, err := http.NewRequest(http.MethodGet, "https://example.com", strings.NewReader("body"))
	if err != nil {
		t.Fatal(err)
	}
	response, err := doer.Do(req)
	if response != nil && response.Body != nil {
		defer response.Body.Close()
	}
	if err != nil || response != original {
		t.Fatalf("got response %p and error %v; want original response %p", response, err, original)
	}
	if calls != 1 {
		t.Errorf("transport called %d times; want 1", calls)
	}
}

func TestFetchSigningKeyCustomRedirectPolicy(t *testing.T) {
	const key = "known signing key"
	policyErr := errors.New("redirect rejected by caller")
	for _, tc := range []struct {
		name        string
		sameOrigin  bool
		tls         bool
		policyError error
		wantError   error
		wantCalls   int64
		wantTargets int64
	}{
		{name: "same origin policy rejects", sameOrigin: true, policyError: policyErr, wantError: policyErr, wantCalls: 1},
		{name: "cross port policy allows", wantCalls: 1, wantTargets: 1},
		{name: "https downgrade rejected before policy", tls: true, wantError: ErrInsecureBaseURL},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var origins, targets, callbacks atomic.Int64
			callbackAuth := make(chan string, 1)
			targetAuth := make(chan string, 1)
			targetHandler := func(w http.ResponseWriter, r *http.Request) {
				targets.Add(1)
				select {
				case targetAuth <- r.Header.Get("Authorization"):
				default:
				}
				_, _ = w.Write([]byte(key))
			}
			destination := httptest.NewServer(http.HandlerFunc(targetHandler))
			defer destination.Close()

			originHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/redirected" {
					targetHandler(w, r)
					return
				}
				origins.Add(1)
				if r.Method != http.MethodGet || r.URL.Path != signingKeyPath || r.Header.Get("Authorization") != "Bearer test-key" {
					t.Errorf("unexpected origin request: %s %s, authorization %q", r.Method, r.URL.Path, r.Header.Get("Authorization"))
				}
				location := destination.URL + "/redirected"
				if tc.sameOrigin {
					location = "/redirected"
				}
				http.Redirect(w, r, location, http.StatusFound)
			})
			var origin *httptest.Server
			if tc.tls {
				origin = httptest.NewTLSServer(originHandler)
			} else {
				origin = httptest.NewServer(originHandler)
			}
			defer origin.Close()

			supplied := origin.Client()
			supplied.CheckRedirect = func(req *http.Request, _ []*http.Request) error {
				callbacks.Add(1)
				select {
				case callbackAuth <- req.Header.Get("Authorization"):
				default:
				}
				return tc.policyError
			}
			client, err := NewClient(origin.URL, "test-key", WithHTTPClient(supplied))
			if err != nil {
				t.Fatal(err)
			}
			if client.httpClient == supplied {
				t.Fatal("client retained the supplied HTTP client instead of a shallow copy")
			}
			got, err := client.FetchSigningKey(context.Background())
			if tc.wantError != nil {
				if !errors.Is(err, tc.wantError) || got != nil {
					t.Fatalf("got key %q and error %v; want no key and %v", got, err, tc.wantError)
				}
			} else if err != nil || string(got) != key {
				t.Fatalf("got key %q and error %v; want %q", got, err, key)
			}
			if origins.Load() != 1 || callbacks.Load() != tc.wantCalls || targets.Load() != tc.wantTargets {
				t.Fatalf("origin requests %d, callbacks %d, destination requests %d; want 1, %d, %d", origins.Load(), callbacks.Load(), targets.Load(), tc.wantCalls, tc.wantTargets)
			}
			if tc.wantTargets == 1 {
				if auth := <-callbackAuth; auth != "" {
					t.Errorf("callback saw authorization %q; want none", auth)
				}
				if auth := <-targetAuth; auth != "" {
					t.Errorf("destination saw authorization %q; want none", auth)
				}
			}

			// Calling the supplied policy directly must still invoke the original
			// callback, without the SDK's guard or changes to its result.
			before := callbacks.Load()
			probe, err := http.NewRequest(http.MethodGet, destination.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			probe.Header.Set("Authorization", "probe")
			// Drain the earlier callback observation before probing the policy.
			if tc.wantCalls == 1 && tc.wantTargets == 0 {
				<-callbackAuth
			}
			if supplied.CheckRedirect == nil {
				t.Fatal("supplied redirect policy was cleared")
			}
			if err := supplied.CheckRedirect(probe, nil); err != tc.policyError || callbacks.Load() != before+1 {
				t.Errorf("supplied policy returned %v and invoked %d callbacks; want %v and one callback", err, callbacks.Load()-before, tc.policyError)
			}
			select {
			case auth := <-callbackAuth:
				if auth != "probe" {
					t.Errorf("supplied policy saw authorization %q; want probe", auth)
				}
			default:
				t.Error("supplied policy did not record the probe")
			}
		})
	}
}
