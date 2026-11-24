// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestFetchSigningKeyRetryAfterInterrupted(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != signingKeyPath {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatal(err)
	}
	retryer, ok := client.requestDoer.(*retryingDoer)
	if !ok {
		t.Fatalf("request doer has type %T; want *retryingDoer", client.requestDoer)
	}
	var delays []time.Duration
	retryer.wait = func(_ context.Context, delay time.Duration) error {
		delays = append(delays, delay)
		return context.Canceled
	}
	key, err := client.FetchSigningKey(context.Background())
	if !errors.Is(err, context.Canceled) || key != nil {
		t.Fatalf("got key %q and error %v; want cancellation and no key", key, err)
	}
	if len(delays) != 1 || delays[0] != 7*time.Second {
		t.Errorf("requested delays %v; want one delay of 7s", delays)
	}
	if got := requests.Load(); got != 1 {
		t.Errorf("server received %d requests; want 1", got)
	}
}

func TestFetchSigningKeyTransportDeadlineReportsConfiguredTimeout(t *testing.T) {
	calls := 0
	transport := roundTripperFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, context.DeadlineExceeded
	})
	const timeout = 7 * time.Second
	client, err := NewClient("https://example.com", "test-key", WithHTTPClient(&http.Client{Transport: transport}), WithTimeout(timeout))
	if err != nil {
		t.Fatal(err)
	}
	key, err := client.FetchSigningKey(context.Background())
	if key != nil || err == nil || err.Error() != "request timed out after 7s" {
		t.Fatalf("got key %q and error %v; want no key and configured timeout error", key, err)
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
