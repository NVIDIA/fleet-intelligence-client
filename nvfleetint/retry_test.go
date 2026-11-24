// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

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

func TestRetryingDoerNetworkTimeoutBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name        string
		maxAttempts int
		wantCalls   int
		wantWaits   int
		wantSuccess bool
	}{
		{name: "timeout then success", maxAttempts: 2, wantCalls: 2, wantWaits: 1, wantSuccess: true},
		{name: "zero attempts", maxAttempts: 0, wantCalls: 1},
		{name: "negative attempts", maxAttempts: -1, wantCalls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			timeoutErr := &retryBoundaryTimeoutError{}
			calls := 0
			var waits []time.Duration
			const retryDelay = 123 * time.Millisecond
			inner := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				if calls == 1 || !tc.wantSuccess {
					return nil, timeoutErr
				}
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("success")), Request: req}, nil
			})}
			doer := &retryingDoer{
				inner: inner, maxAttempts: tc.maxAttempts,
				delay: func(int, *http.Response) time.Duration { return retryDelay },
				wait: func(_ context.Context, delay time.Duration) error {
					waits = append(waits, delay)
					return nil
				},
			}
			req, err := http.NewRequest(http.MethodGet, "https://example.com/", nil)
			if err != nil {
				t.Fatal(err)
			}
			response, err := doer.Do(req)
			if response != nil && response.Body != nil {
				defer response.Body.Close()
			}
			if calls != tc.wantCalls || len(waits) != tc.wantWaits {
				t.Fatalf("got %d attempts and %d waits; want %d and %d", calls, len(waits), tc.wantCalls, tc.wantWaits)
			}
			if tc.wantSuccess {
				if err != nil || response == nil || response.StatusCode != http.StatusOK || response.Body == nil {
					t.Fatalf("got response %#v and error %v; want successful response", response, err)
				}
				body, readErr := io.ReadAll(response.Body)
				if readErr != nil || string(body) != "success" || waits[0] != retryDelay {
					t.Fatalf("got body %q, read error %v, and wait %v", body, readErr, waits[0])
				}
			} else if response != nil || !errors.Is(err, timeoutErr) {
				t.Fatalf("got response %#v and error %v; want original timeout", response, err)
			}
		})
	}
}

func TestDefaultRetryDelayNilResponseCapped(t *testing.T) {
	delay := defaultRetryDelay(6, nil)
	if delay < maximumRetryDelay/2 || delay > maximumRetryDelay+maximumRetryDelay/2 {
		t.Fatalf("got delay %v; want between %v and %v", delay, maximumRetryDelay/2, maximumRetryDelay+maximumRetryDelay/2)
	}
}

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

func TestRetryRequiresResponseWithoutError(t *testing.T) {
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
