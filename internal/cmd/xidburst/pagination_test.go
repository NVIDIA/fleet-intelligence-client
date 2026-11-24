// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package xidburst

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
)

// Verifies --all merges pages using the derived has-more signal
func TestXIDBurstListAllFetchesEveryPage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	pages := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/xid/bursts/count" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"total":2}`))
			return
		}
		page := r.URL.Query().Get("page")
		pages++
		w.Header().Set("Content-Type", "application/json")
		switch page {
		case "0":
			_, _ = w.Write([]byte(`{"items":[{"burstId":"burst-1"}],"page":0,"pageSize":1,"total":2,"hasMore":true}`))
		case "1":
			_, _ = w.Write([]byte(`{"items":[{"burstId":"burst-2"}],"page":1,"pageSize":1,"total":2,"hasMore":false}`))
		default:
			t.Fatalf("unexpected page: %q", page)
		}
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"xidburst", "list", "--window", "24h", "--all", "--page-size", "1"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}
	if pages != 2 {
		t.Fatalf("expected 2 API pages, got %d", pages)
	}
	got := out.String()
	if !strings.Contains(got, "burst-1") || !strings.Contains(got, "burst-2") {
		t.Fatalf("expected both pages in output:\n%s", got)
	}
}

// Explicit --page is one-based; verify the zero-based backend request and
// one-based JSON pagination while preserving other response fields.
func TestXIDBurstListExplicitPagePreservesPayload(t *testing.T) {
	for _, cliPage := range []int{1, 3} {
		t.Run(fmt.Sprintf("page_%d", cliPage), func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			apiPage := cliPage - 1
			const pageSize = 7
			body := []byte(fmt.Sprintf(`{"items":[{"burstId":"pagination-burst","nodeUuid":"node-1","xidCount":2}],"page":%d,"pageSize":7,"total":%d,"hasMore":false,"opaquePaginationContract":"preserved"}`, apiPage, apiPage*pageSize+1))
			type request struct{ method, path, page, size, start, end string }
			requests := make(chan request, 4)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query()
				select {
				case requests <- request{r.Method, r.URL.Path, q.Get("page"), q.Get("pageSize"), q.Get("startTime"), q.Get("endTime")}:
				default:
					t.Error("unexpected extra API requests")
				}
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/v1/xid/bursts/page":
					_, _ = w.Write(body)
				case "/v1/xid/bursts/count":
					_, _ = fmt.Fprintf(w, `{"total":%d}`, apiPage*pageSize+1)
				default:
					t.Errorf("unexpected endpoint: %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
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
			if got := len(requests); got != 2 {
				t.Fatalf("API request count = %d, want one page plus one count request", got)
			}
			gotRequest := <-requests
			wantRequest := request{http.MethodGet, "/v1/xid/bursts/page", strconv.Itoa(apiPage), strconv.Itoa(pageSize), gotRequest.start, gotRequest.end}
			if gotRequest != wantRequest {
				t.Fatalf("request = %+v, want %+v", gotRequest, wantRequest)
			}
			if gotRequest.start == "" || gotRequest.end == "" {
				t.Fatal("page request did not freeze the relative time range")
			}
			gotCount := <-requests
			wantCount := request{http.MethodGet, "/v1/xid/bursts/count", "", "", gotRequest.start, gotRequest.end}
			if gotCount != wantCount {
				t.Fatalf("count request = %+v, want same scope without paging: %+v", gotCount, wantCount)
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
