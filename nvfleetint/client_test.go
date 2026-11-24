// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// Stands in for a custom transport implementation
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	return parsed
}

type retryBoundaryTimeoutError struct{}

func (*retryBoundaryTimeoutError) Error() string { return "test network timeout" }

func (*retryBoundaryTimeoutError) Timeout() bool { return true }

func (*retryBoundaryTimeoutError) Temporary() bool { return true }

func assertReportOptionError(t *testing.T, err error, option, value string) {
	t.Helper()
	var optionErr *InvalidOptionError
	if !errors.As(err, &optionErr) || optionErr.Option != option || optionErr.Value != value {
		t.Errorf("got error %v; want InvalidOptionError with option %q and value %q", err, option, value)
	}
}

func alertBoundaryCalls() []struct {
	name string
	path string
	zero any
	call func(context.Context, *Client) (any, error)
} {
	return []struct {
		name string
		path string
		zero any
		call func(context.Context, *Client) (any, error)
	}{
		{"alerts", "/v1/alerts", AlertsPage{}, func(ctx context.Context, c *Client) (any, error) {
			return c.ListAlerts(ctx, ListAlertsOptions{})
		}},
		{"timeline nodes", "/v1/alert_timeline/nodes", AlertTimelineNodesPage{}, func(ctx context.Context, c *Client) (any, error) {
			return c.ListAlertTimelineNodes(ctx, ListAlertTimelineNodesOptions{})
		}},
		{"node timeline", "/v1/alert_timeline/nodes/node-1/alerts", NodeAlertTimelinePage{}, func(ctx context.Context, c *Client) (any, error) {
			return c.ListNodeAlertTimeline(ctx, ListNodeAlertTimelineOptions{NodeUUID: "node-1"})
		}},
		{"timeline detail", "/v1/alert_timeline/nodes/node-1/alerts/alert-1", AlertTimelineDetails{}, func(ctx context.Context, c *Client) (any, error) {
			return c.DescribeAlertTimelineWithOptions(ctx, "node-1", "alert-1", DescribeAlertTimelineOptions{})
		}},
	}
}

type interruptedTagBody struct {
	partial *strings.Reader
	err     error
	closed  bool
}

func (b *interruptedTagBody) Read(p []byte) (int, error) {
	n, err := b.partial.Read(p)
	if err == io.EOF {
		return n, b.err
	}
	return n, err
}

func (b *interruptedTagBody) Close() error {
	b.closed = true
	return nil
}

type xidFailureEndpoint struct {
	name string
	path string
	call func(*Client) (any, error)
	zero any
}

func xidFailureEndpoints() []xidFailureEndpoint {
	return []xidFailureEndpoint{
		{name: "list", path: "/v1/xid/bursts/page", zero: XIDBurstsPage{}, call: func(c *Client) (any, error) {
			return c.ListXIDBursts(context.Background(), ListXIDBurstsOptions{Window: "24h"})
		}},
		{name: "detail", path: "/v1/xid/bursts/burst-1", zero: XIDBurstDetails{}, call: func(c *Client) (any, error) {
			return c.DescribeXIDBurst(context.Background(), "burst-1")
		}},
		{name: "options", path: "/v1/xid/bursts/options", zero: XIDBurstFilterOptions{}, call: func(c *Client) (any, error) {
			return c.GetXIDBurstFilterOptions(context.Background(), XIDBurstFilterOptionsScope{})
		}},
	}
}

type xidOptionsFailingBody struct {
	err    error
	closed bool
}

func (b *xidOptionsFailingBody) Read([]byte) (int, error) { return 0, b.err }

func (b *xidOptionsFailingBody) Close() error {
	b.closed = true
	return nil
}
