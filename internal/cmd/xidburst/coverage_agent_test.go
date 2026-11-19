// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package xidburst

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdutil"
	"github.com/NVIDIA/fleet-intelligence-client/internal/config"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
)

func TestXIDBurstPlatformDisruptionFilterBoundaries(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	queries := make(chan url.Values, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case queries <- r.URL.Query():
		default:
			t.Error("unexpected extra API request while recorder is full")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(xidBurstsBody))
	}))
	defer server.Close()
	cmdtest.SaveConfig(t, server.URL, "test-key")

	for _, tt := range []struct {
		name string
		flag string
		want []string
	}{
		{name: "omitted"},
		{name: "false", flag: "--platform-disruption=false", want: []string{"false"}},
		{name: "true", flag: "--platform-disruption=true", want: []string{"true"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			args := []string{"xidburst", "list", "--window", "24h"}
			if tt.flag != "" {
				args = append(args, tt.flag)
			}
			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs(args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("command failed: %v", err)
			}
			select {
			case query := <-queries:
				got, present := query["jobDisruptionDueToPlatformIssue"]
				if present != (tt.want != nil) || !reflect.DeepEqual(got, tt.want) {
					t.Fatalf("platform disruption query: got %v (present %t), want %v", got, present, tt.want)
				}
			default:
				t.Fatal("expected a request to the server")
			}
			if !strings.Contains(out.String(), "burst-1") {
				t.Fatalf("output missing burst-1: %s", out.String())
			}
		})
	}
}

func TestXIDBurstDescribeSparseDetails(t *testing.T) {
	number := 48
	burst := nvfleetint.XIDBurstDetails{XIDBurst: nvfleetint.XIDBurst{
		XIDNumbers:       []nvfleetint.XIDBurstXID{{}, {XIDNumber: &number, Description: "Memory error"}},
		DeviceIDs:        map[string][]int{"z-device": {94}, "a-device": {48}},
		SuggestedActions: []nvfleetint.SuggestedAction{{Code: "DRAIN", Action: "Drain node"}},
	}}
	rows := xidBurstDescribeRows(burst)
	if got := rows[13]; !reflect.DeepEqual(got, []string{"XIDS", "48"}) {
		t.Errorf("XIDS row = %v, want [XIDS 48]", got)
	}
	wantDetails := [][]string{
		{"XID 48", "Memory error"},
		{"DEVICE a-device", "48"},
		{"DEVICE z-device", "94"},
		{"SUGGESTED ACTION", "DRAIN: Drain node"},
	}
	if got := rows[19:]; !reflect.DeepEqual(got, wantDetails) {
		t.Errorf("detail rows = %v, want %v", got, wantDetails)
	}
	if got := xidBurstDeviceRows(map[string][]int{}); len(got) != 0 {
		t.Errorf("empty device map produced rows: %v", got)
	}
	var out bytes.Buffer
	if err := writeXIDBurstDescribeTable(&out, burst); err != nil {
		t.Fatalf("render describe table: %v", err)
	}
	for _, want := range []string{"XID 48", "Memory error", "DEVICE a-device", "DEVICE z-device", "SUGGESTED ACTION", "DRAIN: Drain node"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("describe output missing %q: %s", want, out.String())
		}
	}
}

type xidBurstFailingWriter struct {
	bytes.Buffer
	failAt int
	calls  int
	err    error
}

func (w *xidBurstFailingWriter) Write(p []byte) (int, error) {
	w.calls++
	if w.calls == w.failAt {
		return 0, w.err
	}
	return w.Buffer.Write(p)
}

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

func TestXIDBurstReadFlagValidationBeforeRequest(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()
	cmdtest.SaveConfig(t, server.URL, "test-key")

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"describe output", []string{"xidburst", "describe", "burst-1", "--output", "yaml"}, `invalid output "yaml": expected table or json`},
		{"describe timeout", []string{"xidburst", "describe", "burst-1", "--timeout", "0s"}, "--timeout must be greater than 0"},
		{"options output", []string{"xidburst", "options", "--output", "yaml"}, `invalid output "yaml": expected table or json`},
		{"options timeout", []string{"xidburst", "options", "--timeout", "0s"}, "--timeout must be greater than 0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want %q", err, tc.want)
			}
			if out.Len() != 0 || requests.Load() != 0 {
				t.Errorf("output = %q, requests = %d; want neither", out.String(), requests.Load())
			}
		})
	}
}

func TestXIDBurstRejectsMalformedFilterLists(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(xidBurstsBody))
	}))
	defer server.Close()
	cmdtest.SaveConfig(t, server.URL, "test-key")

	for _, tt := range []struct {
		name  string
		args  []string
		wants []string
	}{
		{"empty xid", []string{"xidburst", "list", "--window", "24h", "--xid-numbers", "48,,94"}, []string{"--xid-numbers", "empty values are not allowed"}},
		{"negative xid", []string{"xidburst", "list", "--window", "24h", "--xid-numbers=-1"}, []string{"xid-numbers", "-1", "non-negative"}},
		{"empty category", []string{"xidburst", "list", "--window", "24h", "--categories", "GPU,,Memory"}, []string{"--categories", "empty values are not allowed"}},
		{"empty options node group", []string{"xidburst", "options", "--nodegroup-ids", "ng-1,,ng-2"}, []string{"--nodegroup-ids", "empty values are not allowed"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs(tt.args)
			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected an error")
			}
			for _, want := range tt.wants {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
			if got := requests.Load(); got != 0 {
				t.Errorf("expected no requests, got %d", got)
			}
			if out.Len() != 0 {
				t.Errorf("expected no rendered result, got %q", out.String())
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
		{name: "list", args: []string{"xidburst", "list", "--window", "24h"}, path: "/v1/xid/bursts"},
		{name: "list all", args: []string{"xidburst", "list", "--window", "24h", "--all", "--page-size", "1", "--output", "json"}, path: "/v1/xid/bursts", all: true},
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
					_, _ = w.Write([]byte(`{"items":[{"burstId":"burst-1"}],"page":0,"pageSize":1,"total":2}`))
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
