// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package alert

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
)

// Verifies alert describe timeline output
func TestAlertDescribeTable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/alert_timeline/nodes/node-1/alerts/alert-1" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		query := r.URL.Query()
		if query.Get("order") != "asc" || query.Get("page") != "1" || query.Get("pageSize") != "1" {
			t.Fatalf("unexpected query: %v", query)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"alertUuid":"alert-1","nodeUuid":"node-1","component":"gpu","page":1,"pageSize":1,"total":3,"hasMore":true,"timeline":[{"eventType":"triggered","alertStatus":"Critical","eventTimestamp":"2026-05-01T00:00:00Z","message":"GPU critical"}]}`))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"alert", "describe", "alert-1", "--node", "node-1", "--order", "asc", "--page", "2", "--page-size", "1"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("describe failed: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "TIMESTAMP") || !strings.Contains(got, "triggered") || !strings.Contains(got, "GPU critical") || !strings.Contains(got, "Page: 2  Total Pages: 3") {
		t.Fatalf("describe output missing fields: %q", got)
	}
}

// Verifies long free-text columns are truncated in the table while JSON keeps
// the full payload
func TestAlertDescribeTruncatesLongMessage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	longMessage := strings.Repeat("x", 200)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"alertUuid":"alert-1","nodeUuid":"node-1","component":"gpu","timeline":[{"eventType":"triggered","alertStatus":"Critical","eventTimestamp":"2026-05-01T00:00:00Z","message":"` + longMessage + `"}]}`))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var table bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&table)
	cmd.SetArgs([]string{"alert", "describe", "alert-1", "--node", "node-1"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("describe failed: %v", err)
	}
	if got := table.String(); strings.Contains(got, longMessage) {
		t.Fatalf("expected truncated message in table, got %q", got)
	}
	if got := table.String(); !strings.Contains(got, "…") {
		t.Fatalf("expected ellipsis in truncated table, got %q", got)
	}

	var jsonOut bytes.Buffer
	jsonCmd := newRootCmd()
	jsonCmd.SetOut(&jsonOut)
	jsonCmd.SetArgs([]string{"alert", "describe", "alert-1", "--node", "node-1", "-o", "json"})
	if err := jsonCmd.Execute(); err != nil {
		t.Fatalf("describe json failed: %v", err)
	}
	if got := jsonOut.String(); !strings.Contains(got, longMessage) {
		t.Fatalf("expected full message in json output, got %q", got)
	}
}

func TestAlertDescribePropagatesTableWriterError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	type request struct{ method, path string }
	requests := make(chan request, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case requests <- request{r.Method, r.URL.Path}:
		default:
			t.Error("unexpected extra API request while recorder is full")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"alertUuid":"alert-1","nodeUuid":"node-1","timeline":[{"eventType":"triggered","message":"GPU critical"}]}`))
	}))
	defer server.Close()
	cmdtest.SaveConfig(t, server.URL, "test-key")

	sentinel := errors.New("describe output failed")
	cmd := newRootCmd()
	cmd.SetOut(alertErrorWriter{sentinel})
	cmd.SetArgs([]string{"alert", "describe", "alert-1", "--node", "node-1"})
	if err := cmd.Execute(); !errors.Is(err, sentinel) {
		t.Errorf("error = %v, want sentinel", err)
	}
	select {
	case got := <-requests:
		if got.method != http.MethodGet || got.path != "/v1/alert_timeline/nodes/node-1/alerts/alert-1" {
			t.Errorf("unexpected request: %+v", got)
		}
	default:
		t.Error("expected detail request")
	}
	select {
	case extra := <-requests:
		t.Errorf("unexpected extra request: %+v", extra)
	default:
	}
}

func TestAlertDescribeJSONPaginationPreservesPayload(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	const payload = `{"alertUuid":"alert-1","nodeUuid":"node-1","page":1,"pageSize":1,"total":3,"timeline":[{"eventType":"triggered","message":"GPU critical"}],"unknown":{"nested":{"value":"retained"}}}`
	type request struct {
		method, path   string
		page, pageSize []string
	}
	requests := make(chan request, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		select {
		case requests <- request{r.Method, r.URL.Path, query["page"], query["pageSize"]}:
		default:
			t.Error("unexpected extra API request while recorder is full")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(payload))
	}))
	defer server.Close()
	cmdtest.SaveConfig(t, server.URL, "test-key")

	for _, tc := range []struct {
		name           string
		flags          []string
		page, pageSize []string
		outputPage     float64
	}{
		{name: "paginated", flags: []string{"--page", "2", "--page-size", "1"}, page: []string{"1"}, pageSize: []string{"1"}, outputPage: 2},
		{name: "full timeline", outputPage: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := []string{"alert", "describe", "alert-1", "--node", "node-1", "--output", "json"}
			args = append(args, tc.flags...)
			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs(args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("describe failed: %v", err)
			}
			var gotRequest request
			select {
			case gotRequest = <-requests:
			default:
				t.Fatal("expected a describe request")
			}
			if gotRequest.method != http.MethodGet || gotRequest.path != "/v1/alert_timeline/nodes/node-1/alerts/alert-1" || !reflect.DeepEqual(gotRequest.page, tc.page) || !reflect.DeepEqual(gotRequest.pageSize, tc.pageSize) {
				t.Errorf("unexpected request: %+v", gotRequest)
			}
			var got, want map[string]any
			if err := json.Unmarshal(out.Bytes(), &got); err != nil {
				t.Fatalf("decode output %q: %v", out.String(), err)
			}
			if err := json.Unmarshal([]byte(payload), &want); err != nil {
				t.Fatal(err)
			}
			want["page"] = tc.outputPage
			if !reflect.DeepEqual(got, want) {
				t.Errorf("output = %#v, want %#v", got, want)
			}
		})
	}
	select {
	case extra := <-requests:
		t.Errorf("unexpected extra request: %+v", extra)
	default:
	}
}
