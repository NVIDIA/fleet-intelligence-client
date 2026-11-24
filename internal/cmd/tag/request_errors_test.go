// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package tag

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
)

func TestTagListPermissionError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	var rec tagRequestRecorder
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.record(r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"permission denied","details":"tag access required"}`))
	}))
	defer server.Close()
	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"tag", "list"})
	err := cmd.Execute()
	var apiErr *nvfleetint.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %v, want APIError", err)
	}
	if apiErr.StatusCode != http.StatusForbidden || apiErr.Message != "permission denied" || apiErr.Details != "tag access required" {
		t.Fatalf("unexpected API error: %+v", apiErr)
	}
	want := []tagRecordedRequest{{http.MethodGet, "/v1/tags", url.Values{}}}
	if got := rec.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("requests = %+v, want %+v", got, want)
	}
	if out.Len() != 0 {
		t.Fatalf("unexpected result output: %q", out.String())
	}
}
