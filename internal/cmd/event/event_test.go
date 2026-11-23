// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package event

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdutil"
	"github.com/NVIDIA/fleet-intelligence-client/internal/config"
	clioutput "github.com/NVIDIA/fleet-intelligence-client/internal/output"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
	"github.com/spf13/cobra"
)

// newRootCmd builds a root command carrying only this package's commands, so
// the tests drive them through the same argument path a user types.
func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "nvfleetint",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(NewCmd())
	return root
}

const eventsBody = `{"events":[{"eventId":"e1","nodeUUID":"node-1","component":"GPU","type":"error","name":"xid","message":"boom","timestamp":"2026-05-01T00:00:00Z","createdAt":"2026-05-01T00:00:05Z","suggestedActions":[{"action":"reboot node","code":"R1"}]}],"hasMore":false,"page":0,"pageSize":50,"total":1}`

// Verifies event list table output and time-range translation
func TestEventListTableAbsolute(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/events/count" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"total":1}`))
			return
		}
		if r.URL.Path != "/v1/events/page" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		query := r.URL.Query()
		if got := query.Get("timeMode"); got != "absolute" {
			t.Fatalf("unexpected timeMode: %q", got)
		}
		if got := query.Get("component"); got != "GPU" {
			t.Fatalf("unexpected component: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(eventsBody))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"event", "list", "--start", "2026-05-01T00:00:00Z", "--end", "2026-05-08T00:00:00Z", "--component", "GPU"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	got := out.String()
	// The narrow table exposes these columns/values.
	for _, want := range []string{
		"EVENT ID", "NODE UUID", "COMPONENT", "NAME", "TYPE", "MESSAGE", "TIMESTAMP",
		"e1", "node-1", "GPU", "xid", "boom", "2026-05-01T00:00:00Z",
		"Total Entries: 1",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
	// The verbose createdAt / suggestedActions fields are omitted from the table
	// (still available via -o json).
	for _, absent := range []string{
		"CREATED AT", "SUGGESTED ACTIONS", "2026-05-01T00:00:05Z", "reboot node (R1)",
	} {
		if strings.Contains(got, absent) {
			t.Fatalf("output should not contain %q:\n%s", absent, got)
		}
	}
}

// Verifies event list JSON output presents the page 1-based
func TestEventListJSON(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	want := `{"events":[{"eventId":"e1","nodeUUID":"node-1","component":"GPU","type":"error","name":"xid","message":"boom","timestamp":"2026-05-01T00:00:00.999999Z","createdAt":"2026-05-01T00:00:05Z","suggestedActions":[{"action":"reboot node","code":"R1"}]}],"hasMore":false,"page":1,"pageSize":50,"total":1}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/events/count" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"total":1}`))
			return
		}
		if got := r.URL.Query().Get("window"); got != "" {
			t.Fatalf("unexpected window: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(strings.ReplaceAll(eventsBody, "2026-05-01T00:00:00Z", "2026-05-01T00:00:00.999999Z")))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"event", "list", "--window", "24h", "--output", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}
	if strings.TrimSpace(out.String()) != want {
		t.Fatalf("unexpected JSON:\ngot  %s\nwant %s", strings.TrimSpace(out.String()), want)
	}
}

// Verifies the free-text MESSAGE column is truncated to keep the table readable
// while the full text remains available via -o json, and that the verbose
// createdAt / suggestedActions fields are omitted from the table.
func TestEventRowsTruncatesFreeText(t *testing.T) {
	longMessage := strings.Repeat("x", eventMessageColumnWidth*eventMessageMaxLines+50)

	rows := eventRows([]nvfleetint.Event{{
		EventID:          "e1",
		Message:          longMessage,
		CreatedAt:        "2024-01-15T14:30:00Z",
		SuggestedActions: []nvfleetint.SuggestedAction{{Action: "reboot the node"}},
	}})
	if len(rows) != eventMessageMaxLines {
		t.Fatalf("expected %d rows, got %d", eventMessageMaxLines, len(rows))
	}
	if got := len(rows[0]); got != 8 {
		t.Fatalf("expected 8 columns, got %d: %q", got, rows[0])
	}

	message := rows[len(rows)-1][6]
	if runes := []rune(message); len(runes) != eventMessageColumnWidth {
		t.Fatalf("message column width = %d, want %d", len(runes), eventMessageColumnWidth)
	}
	if !strings.HasSuffix(message, "…") {
		t.Fatalf("message column not ellipsized: %q", message)
	}

	// The omitted verbose fields must not leak into any table cell.
	for _, cell := range rows[0] {
		if strings.Contains(cell, "reboot the node") || strings.Contains(cell, "2024-01-15T14:30:00Z") {
			t.Fatalf("unexpected omitted field in table cell: %q", cell)
		}
	}
}

// Verifies event list rejects a missing time range before any request
func TestEventListRequiresTimeRange(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("did not expect a request for invalid flags")
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"event", "list"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for missing time range")
	}
}

// Verifies event buckets table output, footer, and max-buckets translation
func TestEventBucketsTable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/events/buckets" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("maxBuckets"); got != "50" {
			t.Fatalf("unexpected maxBuckets: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"bucketInterval":"1h","buckets":[{"startTime":"2026-05-01T00:00:00Z","endTime":"2026-05-01T01:00:00Z","firstEventTime":"2026-05-01T00:12:00Z","count":3}]}`))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"event", "buckets", "--window", "24h", "--max-buckets", "50"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{"START TIME", "END TIME", "COUNT", "FIRST EVENT TIME", "3", "Bucket Interval: 1h", "Total Buckets: 1"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

// Verifies event buckets rejects an out-of-range max-buckets value
func TestEventBucketsMaxBucketsValidation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("did not expect a request for invalid flags")
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	for _, value := range []string{"0", "2000"} {
		cmd := newRootCmd()
		cmd.SetOut(new(bytes.Buffer))
		cmd.SetArgs([]string{"event", "buckets", "--window", "24h", "--max-buckets", value})
		if err := cmd.Execute(); err == nil {
			t.Fatalf("expected error for --max-buckets %s", value)
		}
	}
}

func TestEventMessageContinuationRows(t *testing.T) {
	rows := eventRows([]nvfleetint.Event{{EventID: "first", Message: "one\ntwo\nthree\nfour", Timestamp: "2026-11-05T13:03:09.999999Z"}, {EventID: "second", Message: "next"}})
	if len(rows) != 4 || rows[0][0] != "first" || rows[3][0] != "second" {
		t.Fatalf("unexpected rows: %q", rows)
	}
	for _, row := range rows[1:3] {
		for col, cell := range row {
			if col != 6 && cell != "" {
				t.Fatalf("continuation column %d repeated: %q", col, cell)
			}
		}
	}
	var out bytes.Buffer
	if err := clioutput.WriteTable(&out, []string{"EVENT ID", "NODE UUID", "DATA SOURCE", "COMPONENT", "NAME", "TYPE", "MESSAGE", "TIMESTAMP"}, rows); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 5 {
		t.Fatalf("rendered lines = %d: %s", len(lines), out.String())
	}
	if strings.Index(lines[1], "one") != strings.Index(lines[2], "two") {
		t.Fatalf("message lines not aligned: %s", out.String())
	}
}

func TestEventTableTimestampRounding(t *testing.T) {
	const original = "2026-11-05T13:03:09.999999Z"
	const want = "2026-11-05T13:03:10Z"
	events := []nvfleetint.Event{{Timestamp: original}}
	if got := eventRows(events)[0][7]; got != want {
		t.Errorf("event timestamp = %q, want %q", got, want)
	}
	if events[0].Timestamp != original {
		t.Fatal("table rendering changed original timestamp")
	}
	buckets := []nvfleetint.EventBucket{{StartTime: original, EndTime: original, FirstEventTime: original}}
	row := eventBucketRows(buckets)[0]
	for _, column := range []int{0, 1, 3} {
		if row[column] != want {
			t.Errorf("bucket column %d = %q, want %q", column, row[column], want)
		}
	}
	if buckets[0].StartTime != original || buckets[0].EndTime != original || buckets[0].FirstEventTime != original {
		t.Fatal("table rendering changed bucket timestamps")
	}
}

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

// Explicit --page is one-based; verify the zero-based backend request and
// one-based JSON pagination while preserving other response fields.
func TestRound2ExplicitPageContract(t *testing.T) {
	for _, cliPage := range []int{1, 3} {
		t.Run(fmt.Sprintf("page_%d", cliPage), func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			apiPage := cliPage - 1
			const pageSize = 7
			body := []byte(fmt.Sprintf(`{"events":[{"eventId":"pagination-event","nodeUUID":"node-1","name":"xid","timestamp":"2026-05-01T00:00:00Z"}],"page":%d,"pageSize":7,"total":%d,"hasMore":false,"opaquePaginationContract":"preserved"}`, apiPage, apiPage*pageSize+1))
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
			args := []string{"event", "list", "--window", "24h"}
			args = append(args, "--page", strconv.Itoa(cliPage), "--page-size", strconv.Itoa(pageSize), "--output", "json")
			cmd.SetArgs(args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("command failed: %v", err)
			}
			if got := len(requests); got != 1 {
				t.Fatalf("API request count = %d, want 1", got)
			}
			gotRequest := <-requests
			wantRequest := request{http.MethodGet, "/v1/events", strconv.Itoa(apiPage), strconv.Itoa(pageSize)}
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
