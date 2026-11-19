// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package xidburst

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strconv"
	"testing"
)

// Explicit --page is one-based; verify the zero-based backend request and
// one-based JSON pagination while preserving other response fields.
func TestRound2ExplicitPageContract(t *testing.T) {
	for _, cliPage := range []int{1, 3} {
		t.Run(fmt.Sprintf("page_%d", cliPage), func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			apiPage := cliPage - 1
			const pageSize = 7
			body := []byte(fmt.Sprintf(`{"items":[{"burstId":"pagination-burst","nodeUuid":"node-1","xidCount":2}],"page":%d,"pageSize":7,"total":%d,"hasMore":false,"opaquePaginationContract":"preserved"}`, apiPage, apiPage*pageSize+1))
			type request struct{ method, path, page, size string }
			requests := make(chan request, 4)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query()
				select {
				case requests <- request{r.Method, r.URL.Path, q.Get("page"), q.Get("pageSize")}:
				default:
					t.Error("unexpected extra API requests")
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(body)
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")
			cmd := newRootCmd()
			var out bytes.Buffer
			cmd.SetOut(&out)
			args := []string{"xidburst", "list", "--window", "24h"}
			args = append(args, "--page", strconv.Itoa(cliPage), "--page-size", strconv.Itoa(pageSize), "--output", "json")
			cmd.SetArgs(args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("command failed: %v", err)
			}
			if got := len(requests); got != 1 {
				t.Fatalf("API request count = %d, want 1", got)
			}
			gotRequest := <-requests
			wantRequest := request{http.MethodGet, "/v1/xid/bursts", strconv.Itoa(apiPage), strconv.Itoa(pageSize)}
			if gotRequest != wantRequest {
				t.Fatalf("request = %+v, want %+v", gotRequest, wantRequest)
			}
			var got, want map[string]any
			if err := json.Unmarshal(out.Bytes(), &got); err != nil {
				t.Fatalf("invalid command JSON %q: %v", out.String(), err)
			}
			if err := json.Unmarshal(body, &want); err != nil {
				t.Fatal(err)
			}
			// The documented CLI contract normalizes API page 0 to CLI page 1.
			// See output.OneBasedPage and the existing command JSON tests.
			want["page"] = float64(cliPage)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("JSON result = %#v, want normalized CLI result %#v", got, want)
			}
		})
	}
}
