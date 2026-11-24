// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestFilterOptionsTransportAndReadFailures(t *testing.T) {
	for _, endpoint := range []struct {
		name        string
		path        string
		readContext string
		call        func(*Client) (FilterOptions, error)
	}{
		{name: "node", path: "/v1/nodes/options", readContext: "read node filter options", call: func(c *Client) (FilterOptions, error) {
			return c.GetNodeFilterOptionsWithOptions(context.Background(), NodeFilterOptionsRequest{View: NodeViewBasic, DataSource: DataSourceOOB})
		}},
		{name: "node group", path: "/v1/nodegroups/options", readContext: "read node group filter options", call: func(c *Client) (FilterOptions, error) {
			return c.GetNodeGroupFilterOptions(context.Background())
		}},
	} {
		for _, scenario := range []string{"transport", "read"} {
			t.Run(endpoint.name+"/"+scenario, func(t *testing.T) {
				sentinel := errors.New(scenario + " failure")
				body := &xidOptionsFailingBody{err: sentinel}
				calls := 0
				transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
					calls++
					if req.Method != http.MethodGet || req.URL.Path != endpoint.path {
						t.Errorf("got request %s %s; want GET %s", req.Method, req.URL.String(), endpoint.path)
					}
					if endpoint.name == "node" && (req.URL.Query().Get("view") != "basic" || req.URL.Query().Get("dataSource") != "oob") {
						t.Errorf("got query %s; want view=basic and dataSource=oob", req.URL.RawQuery)
					}
					if scenario == "transport" {
						return nil, sentinel
					}
					return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: body, Request: req}, nil
				})
				client, err := NewClient("https://example.com", "test-key", WithHTTPClient(&http.Client{Transport: transport}))
				if err != nil {
					t.Fatal(err)
				}
				got, err := endpoint.call(client)
				if !errors.Is(err, sentinel) || !reflect.DeepEqual(got, FilterOptions{}) || calls != 1 {
					t.Fatalf("got options %#v, error %v, and %d requests; want zero options, sentinel error, and one request", got, err, calls)
				}
				if scenario == "read" && (!strings.Contains(err.Error(), endpoint.readContext) || !body.closed) {
					t.Errorf("got error %v and body closed %t; want %q context and closed body", err, body.closed, endpoint.readContext)
				}
			})
		}
	}
}
