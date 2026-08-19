// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/generated/fleetapi"
)

// Records what a write actually put on the wire
type issuedRequest struct {
	method string
	url    string
	body   string
}

// Verifies every dry-run preview describes the request the write really sends.
// Add a case here for each new write endpoint.
func TestPreviewsMatchIssuedRequests(t *testing.T) {
	// Base URLs ValidateBaseURL accepts but that resolve differently under a
	// hand-rolled URL join than under the generated client.
	baseSuffixes := []string{"", "/", "/api", "/api/", "/api?x=1", "/api#frag", "/a%2Fb"}

	writes := []struct {
		name  string
		issue func(context.Context, *Client) error
		//nolint:revive // preview mirrors issue; both take the same arguments
		preview func(context.Context, *Client) (RequestPreview, error)
	}{
		{
			name: "computezone update",
			issue: func(ctx context.Context, client *Client) error {
				_, err := client.UpdateComputeZone(ctx, updateComputeZonePreviewOptions())
				return err
			},
			preview: func(ctx context.Context, client *Client) (RequestPreview, error) {
				return client.PreviewUpdateComputeZone(ctx, updateComputeZonePreviewOptions())
			},
		},
	}

	for _, write := range writes {
		for _, suffix := range baseSuffixes {
			t.Run(write.name+" base "+suffix, func(t *testing.T) {
				var issued *issuedRequest
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method == http.MethodGet {
						w.Header().Set("Content-Type", "application/json")
						_, _ = w.Write([]byte(`{"computezones":[{"id":"cz-1","type":"datacenter","contact":{"email":"old@example.com","pic":"Ada"},"geoLocation":{"city":"Santa Clara","country":"US","region":"us-west","latitude":37.774929,"longitude":-122.419416}}],"hasMore":false,"page":0,"pageSize":20,"total":1}`))
						return
					}

					data, err := io.ReadAll(r.Body)
					if err != nil {
						t.Fatalf("read body failed: %v", err)
					}
					issued = &issuedRequest{
						method: r.Method,
						// r.URL is request-target only; rebuild the absolute
						// URL the preview reports.
						url:  "http://" + r.Host + r.URL.RequestURI(),
						body: string(data),
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"id":"cz-1"}`))
				}))
				defer server.Close()

				client, err := NewClient(server.URL+suffix, "test-key")
				if err != nil {
					t.Fatalf("new client failed: %v", err)
				}

				if err := write.issue(context.Background(), client); err != nil {
					t.Fatalf("write failed: %v", err)
				}
				if issued == nil {
					t.Fatal("server never saw the write")
				}

				preview, err := write.preview(context.Background(), client)
				if err != nil {
					t.Fatalf("preview failed: %v", err)
				}

				if preview.Method != issued.method {
					t.Fatalf("preview method %q does not match issued %q", preview.Method, issued.method)
				}
				if preview.URL != issued.url {
					t.Fatalf("preview URL %q does not match issued %q", preview.URL, issued.url)
				}
				if string(preview.Body) != issued.body {
					t.Fatalf("preview body %q does not match issued %q", string(preview.Body), issued.body)
				}
			})
		}
	}
}

// Builds options that exercise merged and caller-supplied fields alike
func updateComputeZonePreviewOptions() UpdateComputeZoneOptions {
	email := "new@example.com"
	city := "Austin"
	return UpdateComputeZoneOptions{ID: "cz-1", ContactEmail: &email, GeoCity: &city}
}

// Verifies the preview normalizes a base URL exactly the way the generated
// client does, since that is what makes the previewed URL trustworthy
func TestGeneratedServerURLNormalization(t *testing.T) {
	servers := []string{
		"https://fleet.example.com",
		"https://fleet.example.com/",
		"https://fleet.example.com/api",
		"https://fleet.example.com/api/",
		"https://fleet.example.com/api?x=1",
		"https://fleet.example.com/api#frag",
		"http://127.0.0.1:8080",
	}

	for _, server := range servers {
		t.Run(server, func(t *testing.T) {
			generated, err := fleetapi.NewClient(server)
			if err != nil {
				t.Fatalf("new generated client failed: %v", err)
			}
			if got := generatedServerURL(server); got != generated.Server {
				t.Fatalf("normalized %q, generated client uses %q", got, generated.Server)
			}
		})
	}
}
