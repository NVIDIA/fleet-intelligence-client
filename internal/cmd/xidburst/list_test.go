// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package xidburst

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
)

func TestXIDBurstPlatformDisruptionFilterBoundaries(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	type observedRequest struct {
		path  string
		query url.Values
	}
	queries := make(chan observedRequest, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case queries <- observedRequest{r.URL.Path, r.URL.Query()}:
		default:
			t.Error("unexpected extra API request while recorder is full")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/xid/bursts/page":
			_, _ = w.Write([]byte(xidBurstsBody))
		case "/v1/xid/bursts/count":
			_, _ = w.Write([]byte(`{"total":1}`))
		default:
			t.Errorf("unexpected endpoint: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
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
			for _, path := range []string{"/v1/xid/bursts/page", "/v1/xid/bursts/count"} {
				select {
				case observed := <-queries:
					if observed.path != path {
						t.Fatalf("path = %s, want %s", observed.path, path)
					}
					got, present := observed.query["jobDisruptionDueToPlatformIssue"]
					if present != (tt.want != nil) || !reflect.DeepEqual(got, tt.want) {
						t.Fatalf("%s platform disruption query: got %v (present %t), want %v", path, got, present, tt.want)
					}
				default:
					t.Fatalf("expected request to %s", path)
				}
			}
			if !strings.Contains(out.String(), "burst-1") {
				t.Fatalf("output missing burst-1: %s", out.String())
			}
		})
	}
}
