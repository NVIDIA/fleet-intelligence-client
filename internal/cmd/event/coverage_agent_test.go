// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package event

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdutil"
	"github.com/NVIDIA/fleet-intelligence-client/internal/config"
	clioutput "github.com/NVIDIA/fleet-intelligence-client/internal/output"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
)

func TestEventCommandErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		path string
		all  bool
	}{
		{name: "list", args: []string{"event", "list", "--window", "24h"}, path: "/v1/events"},
		{name: "list all", args: []string{"event", "list", "--window", "24h", "--all", "--page-size", "1", "--output", "json"}, path: "/v1/events", all: true},
		{name: "buckets", args: []string{"event", "buckets", "--window", "24h"}, path: "/v1/events/buckets"},
		{name: "options", args: []string{"event", "options"}, path: "/v1/events/options"},
	}

	type request struct {
		method, path, page, pageSize string
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for _, missingProfile := range []bool{false, true} {
				name := "forbidden"
				if missingProfile {
					name = "missing profile"
				}
				t.Run(name, func(t *testing.T) {
					t.Setenv("HOME", t.TempDir())
					t.Setenv("USERPROFILE", os.Getenv("HOME"))
					var mu sync.Mutex
					var requests []request
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						query := r.URL.Query()
						mu.Lock()
						requests = append(requests, request{r.Method, r.URL.Path, query.Get("page"), query.Get("pageSize")})
						mu.Unlock()
						w.Header().Set("Content-Type", "application/json")
						if tc.all && query.Get("page") == "0" {
							_, _ = w.Write([]byte(`{"events":[{"eventId":"e1"}],"page":0,"pageSize":1,"total":2,"hasMore":true}`))
							return
						}
						w.WriteHeader(http.StatusForbidden)
						_, _ = w.Write([]byte(`{"error":"forbidden","details":"no event access"}`))
					}))
					defer server.Close()
					cmdtest.SaveConfig(t, server.URL, "test-key")

					args := append([]string(nil), tc.args...)
					if missingProfile {
						args = append(args, "--profile", "nonexistent")
					}
					var out bytes.Buffer
					cmd := newRootCmd()
					cmd.SetOut(&out)
					cmd.SetArgs(args)
					err := cmd.Execute()
					if missingProfile {
						if !errors.Is(err, config.ErrProfileNotFound) {
							t.Errorf("error = %v, want profile not found", err)
						}
					} else {
						var apiErr *nvfleetint.APIError
						if !errors.As(err, &apiErr) {
							t.Errorf("error = %v, want APIError", err)
						} else if apiErr.StatusCode != http.StatusForbidden || apiErr.Message != "forbidden" || apiErr.Details != "no event access" {
							t.Errorf("API error = %+v, want status 403, forbidden, no event access", apiErr)
						}
					}
					if out.Len() != 0 {
						t.Errorf("unexpected result output: %q", out.String())
					}

					mu.Lock()
					defer mu.Unlock()
					wantCount := 1
					if missingProfile {
						wantCount = 0
					} else if tc.all {
						wantCount = 2
					}
					if len(requests) != wantCount {
						t.Errorf("requests = %+v, want %d requests", requests, wantCount)
					}
					for i, got := range requests {
						if got.method != http.MethodGet || got.path != tc.path {
							t.Errorf("request %d = %+v, want GET %s", i, got, tc.path)
						}
						if tc.all && (i >= 2 || got.page != []string{"0", "1"}[i] || got.pageSize != "1") {
							t.Errorf("request %d = %+v, want pages 0 then 1 with pageSize 1", i, got)
						}
					}
				})
			}
		})
	}
}

func TestEventCommandValidationBeforeRequest(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"list pagination conflict", []string{"event", "list", "--window", "24h", "--all", "--page", "1"}, "--page cannot be used with --all"},
		{"buckets invalid output", []string{"event", "buckets", "--window", "24h", "--output", "yaml"}, `invalid output "yaml": expected table or json`},
		{"buckets missing time range", []string{"event", "buckets"}, "a time range is required: use --window"},
		{"options invalid timeout", []string{"event", "options", "--timeout", "0s"}, "--timeout must be greater than 0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			var mu sync.Mutex
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
				mu.Lock()
				requests++
				mu.Unlock()
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")

			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want %q", err, tc.want)
			}
			if out.Len() != 0 {
				t.Errorf("unexpected output: %q", out.String())
			}
			mu.Lock()
			defer mu.Unlock()
			if requests != 0 {
				t.Errorf("requests = %d, want 0", requests)
			}
		})
	}
}

type eventFailingWriter struct {
	bytes.Buffer
	writes int
	failAt int
	err    error
}

func (w *eventFailingWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.writes == w.failAt {
		return 0, w.err
	}
	return w.Buffer.Write(p)
}

func TestEventTableWriteErrors(t *testing.T) {
	sentinel := errors.New("injected write failure")
	count := 3
	preamble := "Filters for 'event list' and 'event buckets' (sorting applies to list only)\n" + cmdutil.OptionsPreamble + "\n"
	tests := []struct {
		name   string
		failAt int
		want   string
		write  func(io.Writer) error
	}{
		{"list table", 1, "", func(w io.Writer) error {
			return writeEventListOutput(w, cmdutil.Resolved{Output: clioutput.FormatTable}, eventListOutput{Events: []nvfleetint.Event{{EventID: "e1", Message: "boom"}}})
		}},
		{"buckets table", 1, "", func(w io.Writer) error {
			return writeEventBucketsTable(w, nvfleetint.EventBuckets{BucketInterval: "1h", Buckets: []nvfleetint.EventBucket{{StartTime: "2026-05-01T00:00:00Z", Count: &count}}})
		}},
		{"options preamble", 1, "", func(w io.Writer) error {
			return writeEventOptionsTable(w, nvfleetint.EventFilterOptions{EventNames: []string{"xid"}})
		}},
		{"options section", 2, preamble, func(w io.Writer) error {
			return writeEventOptionsTable(w, nvfleetint.EventFilterOptions{EventNames: []string{"xid"}})
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := &eventFailingWriter{failAt: tc.failAt, err: sentinel}
			if err := tc.write(w); !errors.Is(err, sentinel) {
				t.Errorf("error = %v, want injected write failure", err)
			}
			if got := w.String(); got != tc.want {
				t.Errorf("accepted output = %q, want %q", got, tc.want)
			}
		})
	}
}
