// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package xidburst

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"sync"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdutil"
	"github.com/NVIDIA/fleet-intelligence-client/internal/config"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
)

func TestXIDBurstRenderingWriteFailures(t *testing.T) {
	sentinel := errors.New("output failure")
	t.Run("list table", func(t *testing.T) {
		w := &xidBurstFailingWriter{failAt: 1, err: sentinel}
		err := writeXIDBurstListOutput(w, cmdutil.Resolved{}, xidBurstListOutput{Bursts: []nvfleetint.XIDBurst{{BurstID: "burst-1"}}})
		if !errors.Is(err, sentinel) || w.calls != 1 || w.Len() != 0 {
			t.Fatalf("error = %v, calls = %d, accepted = %q", err, w.calls, w.String())
		}
	})
	preamble := "Filters for 'xidburst list'\n" + cmdutil.OptionsPreamble + "\n"
	for _, tc := range []struct {
		name   string
		failAt int
		prefix string
	}{
		{"preamble", 1, ""},
		{"data source", 2, preamble},
		{"first section", 3, preamble + "Data source: inband\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := &xidBurstFailingWriter{failAt: tc.failAt, err: sentinel}
			err := writeXIDBurstOptionsTable(w, nvfleetint.XIDBurstFilterOptions{DataSource: "inband"})
			if !errors.Is(err, sentinel) || w.calls != tc.failAt || w.String() != tc.prefix {
				t.Fatalf("error = %v, calls = %d, accepted = %q; want sentinel, %d, %q", err, w.calls, w.String(), tc.failAt, tc.prefix)
			}
		})
	}
}

func TestXIDBurstCommandAPIErrorPropagation(t *testing.T) {
	tests := []struct {
		name string
		args []string
		path string
		all  bool
	}{
		{name: "list", args: []string{"xidburst", "list", "--window", "24h"}, path: "/v1/xid/bursts/page"},
		{name: "list all", args: []string{"xidburst", "list", "--window", "24h", "--all", "--page-size", "1", "--output", "json"}, path: "/v1/xid/bursts/page", all: true},
		{name: "describe", args: []string{"xidburst", "describe", "burst-1"}, path: "/v1/xid/bursts/burst-1"},
		{name: "options", args: []string{"xidburst", "options"}, path: "/v1/xid/bursts/options"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			type request struct{ method, path, page, pageSize, authorization string }
			var mu sync.Mutex
			var requests []request
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				requests = append(requests, request{r.Method, r.URL.Path, r.URL.Query().Get("page"), r.URL.Query().Get("pageSize"), r.Header.Get("Authorization")})
				mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				if tc.all && r.URL.Query().Get("page") == "0" {
					_, _ = w.Write([]byte(`{"items":[{"burstId":"burst-1"}],"page":0,"pageSize":1,"hasMore":true}`))
					return
				}
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"forbidden","details":"no XID burst access"}`))
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")

			t.Run("API error", func(t *testing.T) {
				var out bytes.Buffer
				cmd := newRootCmd()
				cmd.SetOut(&out)
				cmd.SetArgs(tc.args)
				err := cmd.Execute()
				var apiErr *nvfleetint.APIError
				if !errors.As(err, &apiErr) {
					t.Fatalf("expected APIError, got %T: %v", err, err)
				}
				if apiErr.StatusCode != http.StatusForbidden || apiErr.Message != "forbidden" || apiErr.Details != "no XID burst access" {
					t.Errorf("unexpected API error: %+v", apiErr)
				}
				if out.Len() != 0 {
					t.Errorf("unexpected command output: %q", out.String())
				}
				mu.Lock()
				observed := append([]request(nil), requests...)
				mu.Unlock()
				wantCount := 1
				if tc.all {
					wantCount = 2
				}
				if len(observed) != wantCount {
					t.Fatalf("got requests %+v, want %d requests", observed, wantCount)
				}
				var pages []string
				for _, got := range observed {
					if got.method != http.MethodGet || got.path != tc.path || got.authorization != "Bearer test-key" {
						t.Errorf("unexpected request: %+v; want GET %s with test key", got, tc.path)
					}
					if tc.all && got.pageSize != "1" {
						t.Errorf("pageSize = %q, want 1", got.pageSize)
					}
					pages = append(pages, got.page)
				}
				if tc.all && !reflect.DeepEqual(pages, []string{"0", "1"}) {
					t.Errorf("requested pages = %v, want [0 1]", pages)
				}
			})

			t.Run("missing profile", func(t *testing.T) {
				mu.Lock()
				requests = nil
				mu.Unlock()
				var out bytes.Buffer
				cmd := newRootCmd()
				cmd.SetOut(&out)
				args := append([]string(nil), tc.args...)
				cmd.SetArgs(append(args, "--profile", "nonexistent"))
				err := cmd.Execute()
				if !errors.Is(err, config.ErrProfileNotFound) {
					t.Errorf("expected profile not found error, got %v", err)
				}
				if out.Len() != 0 {
					t.Errorf("unexpected command output: %q", out.String())
				}
				mu.Lock()
				defer mu.Unlock()
				if len(requests) != 0 {
					t.Errorf("unexpected requests: %+v", requests)
				}
			})
		})
	}
}
