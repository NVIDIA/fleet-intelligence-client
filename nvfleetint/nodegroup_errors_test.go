// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

func TestListNodeGroupsInterruptedResponseBody(t *testing.T) {
	sentinel := errors.New("interrupted node group response body")
	body := &interruptedTagBody{partial: strings.NewReader(`{"nodeGroups":[`), err: sentinel}
	requests := 0
	transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		requests++
		if req.Method != http.MethodGet || req.URL.Path != "/v1/nodegroups" {
			t.Errorf("got request %s %s; want GET /v1/nodegroups", req.Method, req.URL.Path)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: body, Request: req}, nil
	})
	client, err := NewClient("https://example.com", "test-key", WithHTTPClient(&http.Client{Transport: transport}))
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.ListNodeGroups(context.Background(), ListNodeGroupsOptions{})
	if !errors.Is(err, sentinel) || !reflect.DeepEqual(got, NodeGroupsPage{}) {
		t.Errorf("got result %#v and error %v; want zero result and sentinel error", got, err)
	}
	if requests != 1 || !body.closed {
		t.Errorf("got %d requests and body closed %t; want one request and closed body", requests, body.closed)
	}
}

func TestListNodeGroupsMalformedViewResponses(t *testing.T) {
	for _, view := range []NodeGroupView{NodeGroupViewBasic, NodeGroupViewDetail} {
		t.Run(string(view), func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				requests.Add(1)
				if req.Method != http.MethodGet || req.URL.Path != "/v1/nodegroups" || req.URL.Query().Get("view") != string(view) {
					t.Errorf("got request %s %s; want GET /v1/nodegroups with view=%s", req.Method, req.URL.String(), view)
				}
				w.Header().Set("Content-Type", "text/plain")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{!`))
			}))
			defer server.Close()
			client, err := NewClient(server.URL, "test-key")
			if err != nil {
				t.Fatal(err)
			}
			got, err := client.ListNodeGroups(context.Background(), ListNodeGroupsOptions{View: view})
			var syntaxErr *json.SyntaxError
			if !errors.As(err, &syntaxErr) || !reflect.DeepEqual(got, NodeGroupsPage{}) {
				t.Errorf("got result %#v and error %v; want zero result and JSON syntax error", got, err)
			}
			if requests.Load() != 1 {
				t.Errorf("got %d requests; want one", requests.Load())
			}
		})
	}
}
