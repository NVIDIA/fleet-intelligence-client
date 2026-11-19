// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/NVIDIA/fleet-intelligence-client/internal/generated/fleetapi"
)

func TestValidateStepDurationBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name    string
		step    string
		wantErr string
	}{
		{name: "maximum duration", step: "9223372036854775807ns"},
		{name: "nanosecond overflow", step: "9223372036854775808ns", wantErr: `invalid step "9223372036854775808ns": duration is too large; maximum is 2562047h47m16.854775807s`},
		{name: "compound overflow", step: "2562047h47m16.854775808s", wantErr: `invalid step "2562047h47m16.854775808s": duration is too large; maximum is 2562047h47m16.854775807s`},
		{name: "one minute", step: "1m"},
		{name: "below minimum", step: "59s", wantErr: `invalid step "59s": expected at least 1m`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateStep(tc.step)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateStep(%q) returned %v; want nil", tc.step, err)
				}
			} else if err == nil || err.Error() != tc.wantErr {
				t.Fatalf("ValidateStep(%q) returned %v; want %q", tc.step, err, tc.wantErr)
			}
		})
	}
}

func TestMalformedBaseURLParseErrors(t *testing.T) {
	for _, tc := range []struct {
		name    string
		baseURL string
	}{
		{name: "invalid percent escape", baseURL: "  https://example.com/%zz  "},
		{name: "invalid port", baseURL: "https://example.com:bad"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			validateErr := ValidateBaseURL(tc.baseURL)
			optionCalled := false
			client, clientErr := NewClient(tc.baseURL, "test-key", func(*Client) {
				optionCalled = true
			})
			if client != nil || optionCalled {
				t.Errorf("NewClient returned client %v and called option %t; want nil and false", client, optionCalled)
			}
			for name, err := range map[string]error{"ValidateBaseURL": validateErr, "NewClient": clientErr} {
				var parseErr *url.Error
				if err == nil || !strings.HasPrefix(err.Error(), "invalid API URL:") || !errors.As(err, &parseErr) || errors.Is(err, ErrInsecureBaseURL) {
					t.Errorf("%s returned %v; want wrapped URL parse error with invalid API URL prefix", name, err)
				}
			}
		})
	}
}

type retryBoundaryTimeoutError struct{}

func (*retryBoundaryTimeoutError) Error() string   { return "test network timeout" }
func (*retryBoundaryTimeoutError) Timeout() bool   { return true }
func (*retryBoundaryTimeoutError) Temporary() bool { return true }

func TestRetryingDoerNetworkTimeoutBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name        string
		maxAttempts int
		wantCalls   int
		wantWaits   int
		wantSuccess bool
	}{
		{name: "timeout then success", maxAttempts: 2, wantCalls: 2, wantWaits: 1, wantSuccess: true},
		{name: "zero attempts", maxAttempts: 0, wantCalls: 1},
		{name: "negative attempts", maxAttempts: -1, wantCalls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			timeoutErr := &retryBoundaryTimeoutError{}
			calls := 0
			var waits []time.Duration
			const retryDelay = 123 * time.Millisecond
			inner := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				if calls == 1 || !tc.wantSuccess {
					return nil, timeoutErr
				}
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("success")), Request: req}, nil
			})}
			doer := &retryingDoer{
				inner: inner, maxAttempts: tc.maxAttempts,
				delay: func(int, *http.Response) time.Duration { return retryDelay },
				wait: func(_ context.Context, delay time.Duration) error {
					waits = append(waits, delay)
					return nil
				},
			}
			req, err := http.NewRequest(http.MethodGet, "https://example.com/", nil)
			if err != nil {
				t.Fatal(err)
			}
			response, err := doer.Do(req)
			if response != nil && response.Body != nil {
				defer response.Body.Close()
			}
			if calls != tc.wantCalls || len(waits) != tc.wantWaits {
				t.Fatalf("got %d attempts and %d waits; want %d and %d", calls, len(waits), tc.wantCalls, tc.wantWaits)
			}
			if tc.wantSuccess {
				if err != nil || response == nil || response.StatusCode != http.StatusOK || response.Body == nil {
					t.Fatalf("got response %#v and error %v; want successful response", response, err)
				}
				body, readErr := io.ReadAll(response.Body)
				if readErr != nil || string(body) != "success" || waits[0] != retryDelay {
					t.Fatalf("got body %q, read error %v, and wait %v", body, readErr, waits[0])
				}
			} else if response != nil || !errors.Is(err, timeoutErr) {
				t.Fatalf("got response %#v and error %v; want original timeout", response, err)
			}
		})
	}
}

func TestEventBucketsAbsoluteRangeAndInvalidRange(t *testing.T) {
	const start = "2026-05-01T00:00:00Z"
	const end = "2026-05-01T01:00:00Z"
	const payload = `{"bucketInterval":"1h","buckets":[{"startTime":"2026-05-01T00:00:00Z","endTime":"2026-05-01T01:00:00Z","count":3}]}`
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		q := r.URL.Query()
		if r.Method != http.MethodGet || r.URL.Path != "/v1/events/buckets" || q.Get("timeMode") != "absolute" || q.Get("startTime") != start || q.Get("endTime") != end || q.Get("nodeUUID") != "node-1" || q.Get("component") != "gpu" || q.Has("window") {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(payload))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.GetEventBuckets(context.Background(), EventBucketsOptions{StartTime: start, EndTime: end, NodeUUID: "  node-1  ", Component: "  gpu  "})
	if err != nil {
		t.Fatal(err)
	}
	if got.BucketInterval != "1h" || string(got.RawJSON) != payload || len(got.Buckets) != 1 {
		t.Fatalf("unexpected buckets: %#v", got)
	}
	bucket := got.Buckets[0]
	if bucket.StartTime != start || bucket.EndTime != end || bucket.Count == nil || *bucket.Count != 3 {
		t.Fatalf("unexpected bucket: %#v", bucket)
	}
	if requests.Load() != 1 {
		t.Fatalf("got %d requests; want 1", requests.Load())
	}
	got, err = client.GetEventBuckets(context.Background(), EventBucketsOptions{StartTime: "invalid", EndTime: end})
	if err == nil || err.Error() != "event start time must be RFC3339" || !reflect.DeepEqual(got, EventBuckets{}) {
		t.Fatalf("got buckets %#v and error %v; want zero result and invalid start time error", got, err)
	}
	if requests.Load() != 1 {
		t.Errorf("invalid range made a request; got %d requests", requests.Load())
	}
}

func TestEventResponseBoundaries(t *testing.T) {
	endpoints := []struct {
		name string
		path string
		zero any
		call func(context.Context, *Client) (any, error)
	}{
		{"list", "/v1/events", EventsPage{}, func(ctx context.Context, c *Client) (any, error) {
			return c.ListEvents(ctx, EventListOptions{Window: "24h"})
		}},
		{"buckets", "/v1/events/buckets", EventBuckets{}, func(ctx context.Context, c *Client) (any, error) {
			return c.GetEventBuckets(ctx, EventBucketsOptions{Window: "24h"})
		}},
		{"filter options", "/v1/events/options", EventFilterOptions{}, func(ctx context.Context, c *Client) (any, error) {
			return c.GetEventFilterOptions(ctx)
		}},
	}
	for _, endpoint := range endpoints {
		for _, scenario := range []string{"canceled", "malformed", "forbidden"} {
			if scenario == "forbidden" && endpoint.name == "list" {
				continue
			}
			t.Run(endpoint.name+"/"+scenario, func(t *testing.T) {
				var requests atomic.Int64
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					requests.Add(1)
					if r.Method != http.MethodGet || r.URL.Path != endpoint.path {
						t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
					}
					if scenario == "forbidden" {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusForbidden)
						_, _ = w.Write([]byte(`{"error":"denied","details":"event access forbidden"}`))
					} else {
						w.Header().Set("Content-Type", "text/plain")
						_, _ = w.Write([]byte(`{"broken":`))
					}
				}))
				defer server.Close()
				client, err := NewClient(server.URL, "test-key")
				if err != nil {
					t.Fatal(err)
				}
				ctx := context.Background()
				if scenario == "canceled" {
					var cancel context.CancelFunc
					ctx, cancel = context.WithCancel(ctx)
					cancel()
				}
				got, err := endpoint.call(ctx, client)
				if !reflect.DeepEqual(got, endpoint.zero) {
					t.Errorf("got result %#v; want zero result %#v", got, endpoint.zero)
				}
				switch scenario {
				case "canceled":
					if !errors.Is(err, context.Canceled) || requests.Load() != 0 {
						t.Fatalf("got error %v and %d requests; want cancellation and no request", err, requests.Load())
					}
				case "malformed":
					var syntaxErr *json.SyntaxError
					if !errors.As(err, &syntaxErr) || requests.Load() != 1 {
						t.Fatalf("got error %v and %d requests; want JSON syntax error and one request", err, requests.Load())
					}
				case "forbidden":
					var apiErr *APIError
					if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusForbidden || apiErr.Message != "denied" || apiErr.Details != "event access forbidden" || requests.Load() != 1 {
						t.Fatalf("got error %v and %d requests; want 403 with server details and one request", err, requests.Load())
					}
				}
			})
		}
	}
}

func TestDefaultRetryDelayNilResponseCapped(t *testing.T) {
	delay := defaultRetryDelay(6, nil)
	if delay < maximumRetryDelay/2 || delay > maximumRetryDelay+maximumRetryDelay/2 {
		t.Fatalf("got delay %v; want between %v and %v", delay, maximumRetryDelay/2, maximumRetryDelay+maximumRetryDelay/2)
	}
}

func TestDescribeNodeAttestationGPUClaims(t *testing.T) {
	const payload = `{"nodeUUID":"node-1","verificationCheckExtraInfo":{"gpuEvidencePresent":false,"overallAttestationResult":true,"nrasCallResult":"complete","gpuResults":{"gpu-1":{"eat_nonce":"nonce","exp":200,"hwmodel":"H100","iat":100,"iss":"issuer","jti":"token","measres":"failed","nbf":90,"sub":"subject","ueid":"device","x-nvidia-attestation-warning":"warning","x-nvidia-gpu-arch-check":false,"x-nvidia-gpu-attestation-report-nonce-match":true,"x-nvidia-gpu-attestation-report-parsed":false,"x-nvidia-gpu-attestation-report-signature-verified":true,"x-nvidia-gpu-vbios-rim-fetched":false,"x-nvidia-gpu-vbios-rim-measurements-available":true,"x-nvidia-gpu-vbios-version":"v1","x-nvidia-overall-att-result":false,"x-nvidia-gpu-attestation-report-cert-chain":{"x-nvidia-cert-expiration-date":"2030-01-01","x-nvidia-cert-ocsp-nonce-matches":false,"x-nvidia-cert-ocsp-response-valid":true,"x-nvidia-cert-ocsp-status":"good","x-nvidia-cert-revocation-reason":"none","x-nvidia-cert-status":"valid"},"x-nvidia-error-details":[{"code":42,"description":"description","fieldName":"measurement","httpStatus":"400","message":"mismatch"}],"x-nvidia-mismatch-measurement-records":[{"index":1,"expected":"abc"},"record"]},"gpu-2":{}}}}`
	const absentPayload = `{"nodeUUID":"node-1","verificationCheckExtraInfo":{"gpuResults":{"gpu-3":{"iss":"issuer"}}}}`
	for _, tc := range []struct {
		name, body string
		populated  bool
	}{{"populated", payload, true}, {"missing chain and errors", absentPayload, false}} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/nodes/node-1" || r.URL.Query().Get("dataSource") != "inband" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			client, err := NewClient(server.URL, "test-key")
			if err != nil {
				t.Fatal(err)
			}
			got, err := client.DescribeNodeWithOptions(context.Background(), "node-1", DescribeNodeOptions{DataSource: DataSourceInband})
			if err != nil {
				t.Fatal(err)
			}
			if got.UUID != "node-1" || string(got.RawJSON) != tc.body || got.VerificationCheckExtraInfo == nil {
				t.Fatalf("unexpected node or raw JSON: %#v", got)
			}
			info := got.VerificationCheckExtraInfo
			if !tc.populated {
				claim, ok := info.GPUResults["gpu-3"]
				if !ok || len(info.GPUResults) != 1 || claim.Issuer != "issuer" || claim.NVIDIAGPUAttestationReportCertChain != nil || claim.NVIDIAErrorDetails != nil {
					t.Fatalf("missing fields changed: %#v", info.GPUResults)
				}
				return
			}
			claim, ok := info.GPUResults["gpu-1"]
			empty, emptyOK := info.GPUResults["gpu-2"]
			if !ok || !emptyOK || len(info.GPUResults) != 2 || !reflect.DeepEqual(empty, JWTClaims{}) {
				t.Fatalf("unexpected GPU claims: %#v", info.GPUResults)
			}
			if info.GPUEvidencePresent == nil || *info.GPUEvidencePresent || info.OverallAttestationResult == nil || !*info.OverallAttestationResult || info.NRASCallResult != "complete" {
				t.Fatalf("unexpected aggregate info: %#v", info)
			}
			if claim.EATNonce != "nonce" || claim.ExpiresAt == nil || *claim.ExpiresAt != 200 || claim.IssuedAt == nil || *claim.IssuedAt != 100 || claim.NotBefore == nil || *claim.NotBefore != 90 || claim.Issuer != "issuer" || claim.JWTID != "token" || claim.Subject != "subject" || claim.HardwareModel != "H100" || claim.UEID != "device" || claim.MeasurementResults != "failed" || claim.NVIDIAAttestationWarning != "warning" || claim.NVIDIAGPUVBIOSVersion != "v1" {
				t.Fatalf("unexpected JWT fields: %#v", claim)
			}
			for _, check := range []struct {
				name  string
				value *bool
				want  bool
			}{
				{"architecture", claim.NVIDIAGPUArchitectureCheck, false}, {"nonce", claim.NVIDIAGPUAttestationReportNonceMatch, true}, {"parsed", claim.NVIDIAGPUAttestationReportParsed, false}, {"signature", claim.NVIDIAGPUAttestationReportSignatureVerified, true}, {"rim fetched", claim.NVIDIAGPUVBIOSRIMFetched, false}, {"rim measurements", claim.NVIDIAGPUVBIOSRIMMeasurementsAvailable, true}, {"overall", claim.NVIDIAOverallAttestationResult, false},
			} {
				if check.value == nil || *check.value != check.want {
					t.Errorf("%s: got %v, want %v", check.name, check.value, check.want)
				}
			}
			chain := claim.NVIDIAGPUAttestationReportCertChain
			if chain == nil || chain.ExpirationDate != "2030-01-01" || chain.OCSPNonceMatches == nil || *chain.OCSPNonceMatches || chain.OCSPResponseValid == nil || !*chain.OCSPResponseValid || chain.OCSPStatus != "good" || chain.RevocationReason != "none" || chain.Status != "valid" {
				t.Fatalf("unexpected certificate chain: %#v", chain)
			}
			if len(claim.NVIDIAErrorDetails) != 1 {
				t.Fatalf("unexpected errors: %#v", claim.NVIDIAErrorDetails)
			}
			detail := claim.NVIDIAErrorDetails[0]
			if detail.Code == nil || *detail.Code != 42 || detail.Description != "description" || detail.FieldName != "measurement" || detail.HTTPStatus != "400" || detail.Message != "mismatch" {
				t.Fatalf("unexpected error detail: %#v", detail)
			}
			wantRecords := []any{map[string]any{"index": float64(1), "expected": "abc"}, "record"}
			if !reflect.DeepEqual(claim.NVIDIAMismatchMeasurementRecords, wantRecords) {
				t.Fatalf("unexpected mismatch records: %#v", claim.NVIDIAMismatchMeasurementRecords)
			}
		})
	}
}

func TestDescribeNodeResourcesWithoutGPUInfo(t *testing.T) {
	const payload = `{"nodeUUID":"node-1","hostname":"host-1","resources":{"cpuInfo":{"manufacturer":"Intel","logicalCores":"96"}}}`
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != "/v1/nodes/node-1" || r.URL.Query().Get("dataSource") != "inband" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(payload))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatal(err)
	}
	node, err := client.DescribeNodeWithOptions(context.Background(), "node-1", DescribeNodeOptions{DataSource: DataSourceInband})
	if err != nil {
		t.Fatal(err)
	}
	if node.UUID != "node-1" || node.Hostname != "host-1" || string(node.RawJSON) != payload || node.Resources == nil || node.Resources.CPUInfo == nil || node.Resources.CPUInfo.Manufacturer != "Intel" || node.Resources.CPUInfo.LogicalCores != "96" || node.Resources.GPUInfo != nil || requests.Load() != 1 {
		t.Fatalf("unexpected node %#v after %d requests", node, requests.Load())
	}
}

func TestVerificationEARClonesTypedNilContainers(t *testing.T) {
	var nilMap map[string]any
	var nilSlice []any
	nestedMap := map[string]any{"nilMap": nilMap, "value": "original"}
	nestedSlice := []any{nilSlice, map[string]any{"value": "nested"}}
	ear := map[string]any{"object": nestedMap, "array": nestedSlice}
	got := verificationCheckExtraInfoFromGenerated(&fleetapi.ModelsVerificationCheckExtraInfo{Ear: &ear})
	if got == nil || got.EAR == nil {
		t.Fatal("missing mapped EAR")
	}
	object, ok := got.EAR["object"].(map[string]any)
	if !ok {
		t.Fatalf("object has unexpected type: %#v", got.EAR["object"])
	}
	mappedNilMap, ok := object["nilMap"].(map[string]any)
	if !ok || mappedNilMap != nil {
		t.Fatalf("typed nil map changed: %#v", object["nilMap"])
	}
	array, ok := got.EAR["array"].([]any)
	if !ok || len(array) != 2 {
		t.Fatalf("array changed: %#v", got.EAR["array"])
	}
	mappedNilSlice, ok := array[0].([]any)
	if !ok || mappedNilSlice != nil {
		t.Fatalf("typed nil slice changed: %#v", array[0])
	}
	nestedMap["value"] = "changed"
	nestedSlice[1].(map[string]any)["value"] = "changed"
	nestedSlice[0] = "changed"
	ear["object"] = "changed"
	want := map[string]any{
		"object": map[string]any{"nilMap": nilMap, "value": "original"},
		"array":  []any{nilSlice, map[string]any{"value": "nested"}},
	}
	if !reflect.DeepEqual(got.EAR, want) {
		t.Fatalf("mapped EAR changed with source: %#v; want %#v", got.EAR, want)
	}
}

func TestInventoryReportOptionValidationBoundaries(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name      string
		opts      InventoryReportOptions
		option    string
		value     string
		wantError string
	}{
		{name: "invalid format", opts: InventoryReportOptions{Format: "xml"}, option: "format", value: "xml"},
		{name: "invalid end time", opts: InventoryReportOptions{StartTime: "2026-05-01T00:00:00Z", EndTime: "bad"}, wantError: "inventory report end time must be RFC3339"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			valuesErr := tc.opts.ValidateValues()
			if tc.option == "" {
				if valuesErr != nil {
					t.Fatalf("ValidateValues: %v", valuesErr)
				}
			} else {
				assertReportOptionError(t, valuesErr, tc.option, tc.value)
			}
			validateErr := tc.opts.Validate()
			got, getErr := client.GetInventoryReport(context.Background(), tc.opts)
			if !reflect.DeepEqual(got, InventoryReport{}) {
				t.Errorf("got report %#v; want zero report", got)
			}
			if tc.option != "" {
				assertReportOptionError(t, validateErr, tc.option, tc.value)
				assertReportOptionError(t, getErr, tc.option, tc.value)
			} else {
				for _, err := range []error{validateErr, getErr} {
					if err == nil || err.Error() != tc.wantError {
						t.Errorf("got error %v; want %q", err, tc.wantError)
					}
				}
			}
		})
	}
	if got := requests.Load(); got != 0 {
		t.Errorf("server received %d requests; want none", got)
	}
}

func TestErrorReportOptionValidationBoundaries(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name      string
		opts      ErrorReportOptions
		option    string
		value     string
		wantError string
	}{
		{name: "invalid format", opts: ErrorReportOptions{View: ErrorReportViewList, GroupBy: ErrorReportGroupByError, Format: "xml"}, option: "format", value: "xml"},
		{name: "invalid time mode", opts: ErrorReportOptions{View: ErrorReportViewList, GroupBy: ErrorReportGroupByError, TimeMode: "weekly"}, option: "timeMode", value: "weekly"},
		{name: "overview grouping", opts: ErrorReportOptions{View: ErrorReportViewOverview, GroupBy: ErrorReportGroupByNode}, wantError: "error report overview view does not support group-by"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			valuesErr := tc.opts.ValidateValues()
			if tc.option == "" {
				if valuesErr != nil {
					t.Fatalf("ValidateValues: %v", valuesErr)
				}
			} else {
				assertReportOptionError(t, valuesErr, tc.option, tc.value)
			}
			validateErr := tc.opts.Validate()
			got, getErr := client.GetErrorReport(context.Background(), tc.opts)
			if !reflect.DeepEqual(got, ErrorReport{}) {
				t.Errorf("got report %#v; want zero report", got)
			}
			if tc.option != "" {
				assertReportOptionError(t, validateErr, tc.option, tc.value)
				assertReportOptionError(t, getErr, tc.option, tc.value)
			} else {
				for _, err := range []error{validateErr, getErr} {
					if err == nil || err.Error() != tc.wantError {
						t.Errorf("got error %v; want %q", err, tc.wantError)
					}
				}
			}
		})
	}
	if got := requests.Load(); got != 0 {
		t.Errorf("server received %d requests; want none", got)
	}
}

func assertReportOptionError(t *testing.T, err error, option, value string) {
	t.Helper()
	var optionErr *InvalidOptionError
	if !errors.As(err, &optionErr) || optionErr.Option != option || optionErr.Value != value {
		t.Errorf("got error %v; want InvalidOptionError with option %q and value %q", err, option, value)
	}
}

func TestSignedInventoryResponseMetadataBoundaries(t *testing.T) {
	// An empty ZIP archive, including its end-of-central-directory record.
	bundle := []byte("PK\x05\x06\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00")
	for _, tc := range []struct {
		name, contentType, disposition, filename, wantError string
	}{
		{name: "parameterized type and path", contentType: "application/zip; charset=binary", disposition: `attachment; filename="reports/bundle.zip"`, filename: "bundle.zip"},
		{name: "malformed disposition", contentType: "application/zip", disposition: `attachment; filename="unterminated`, filename: "inventory-report.zip"},
		{name: "dot filename", contentType: "application/zip", disposition: `attachment; filename="."`, filename: "inventory-report.zip"},
		{name: "missing content type", wantError: "signed inventory report response missing content type"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query()
				if r.Method != http.MethodGet || r.URL.Path != "/v1/reports/inventory" || q.Get("dataSource") != "oob" || q.Get("signed") != "true" || q.Get("format") != "csv" || r.Header.Get("Accept") != "application/zip" {
					t.Errorf("unexpected request: %s %s, Accept %q", r.Method, r.URL.String(), r.Header.Get("Accept"))
				}
				if tc.contentType == "" {
					w.Header()["Content-Type"] = nil
				} else {
					w.Header().Set("Content-Type", tc.contentType)
				}
				if tc.disposition != "" {
					w.Header().Set("Content-Disposition", tc.disposition)
				}
				_, _ = w.Write(bundle)
			}))
			defer server.Close()
			client, err := NewClient(server.URL, "test-key")
			if err != nil {
				t.Fatal(err)
			}
			got, err := client.GetInventoryReport(context.Background(), InventoryReportOptions{DataSource: DataSourceOOB, Format: ReportFormatCSV, Signed: true})
			if tc.wantError != "" {
				if err == nil || err.Error() != tc.wantError || !reflect.DeepEqual(got, InventoryReport{}) {
					t.Fatalf("got report %#v and error %v; want zero report and %q", got, err, tc.wantError)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Filename != tc.filename || !reflect.DeepEqual(got.RawSigned, bundle) {
				t.Fatalf("got filename %q and bundle %q; want %q and %q", got.Filename, got.RawSigned, tc.filename, bundle)
			}
		})
	}
}

func TestErrorReportGraphFlexibleValues(t *testing.T) {
	const payload = `{"result":[{"label":{"error":"absent"}},{"label":{"error":"null"},"values":null},{"label":{"error":"string"},"values":"line\nnext"},{"label":{"error":"array"},"values": [ [ 1, 2 ], { "count": 3 } ]}]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/reports/error" || r.URL.Query().Get("view") != "graph" || r.URL.Query().Get("groupBy") != "error" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(payload))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.GetErrorReport(context.Background(), ErrorReportOptions{View: ErrorReportViewGraph})
	if err != nil {
		t.Fatal(err)
	}
	want := []ErrorTimeSeries{{Error: "absent", Values: ""}, {Error: "null", Values: ""}, {Error: "string", Values: "line\nnext"}, {Error: "array", Values: `[[1,2],{"count":3}]`}}
	if got.View != ErrorReportViewGraph || got.GroupBy != ErrorReportGroupByError || got.Graph == nil || string(got.RawJSON) != payload {
		t.Fatalf("unexpected graph report: %#v", got)
	}
	if len(got.Graph.Result) != len(want) {
		t.Fatalf("got %d series; want %d", len(got.Graph.Result), len(want))
	}
	for i, expected := range want {
		t.Run(expected.Error, func(t *testing.T) {
			if got.Graph.Result[i] != expected {
				t.Errorf("got series %#v; want %#v", got.Graph.Result[i], expected)
			}
		})
	}
}

func TestReportOptionValuesValid(t *testing.T) {
	for _, tc := range []struct {
		value ReportFormat
		want  bool
	}{{ReportFormatJSON, true}, {ReportFormatCSV, true}, {"xml", false}} {
		t.Run("format/"+string(tc.value), func(t *testing.T) {
			if got := tc.value.Valid(); got != tc.want {
				t.Errorf("Valid() = %v; want %v", got, tc.want)
			}
		})
	}
	for _, tc := range []struct {
		value ErrorReportTimeMode
		want  bool
	}{{ErrorReportTimeModeAbsolute, true}, {ErrorReportTimeModeRelative, true}, {"weekly", false}} {
		t.Run("time mode/"+string(tc.value), func(t *testing.T) {
			if got := tc.value.Valid(); got != tc.want {
				t.Errorf("Valid() = %v; want %v", got, tc.want)
			}
		})
	}
}

func TestNodeResponseDecodingErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		view   NodeView
		source DataSource
		detail bool
	}{
		{"basic list", NodeViewBasic, DataSourceInband, false},
		{"inband detail list", NodeViewDetail, DataSourceInband, false},
		{"oob detail list", NodeViewDetail, DataSourceOOB, false},
		{"inband describe", "", DataSourceInband, true},
		{"oob describe", "", DataSourceOOB, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				path := "/v1/nodes"
				if tc.detail {
					path += "/node-1"
				}
				if r.Method != http.MethodGet || r.URL.Path != path || r.URL.Query().Get("view") != string(tc.view) || r.URL.Query().Get("dataSource") != string(tc.source) {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
				}
				w.Header().Set("Content-Type", "text/plain")
				if tc.detail {
					_, _ = w.Write([]byte(`{"gpuCount":"not-an-integer"}`))
				} else {
					_, _ = w.Write([]byte(`{"page":"not-an-integer"}`))
				}
			}))
			defer server.Close()
			client, err := NewClient(server.URL, "test-key")
			if err != nil {
				t.Fatal(err)
			}
			if tc.detail {
				got, err := client.DescribeNodeWithOptions(context.Background(), "node-1", DescribeNodeOptions{DataSource: tc.source})
				var decodeErr *json.UnmarshalTypeError
				if !errors.As(err, &decodeErr) || !reflect.DeepEqual(got, NodeDetails{}) {
					t.Fatalf("got result %#v and error %v; want zero result and decoding error", got, err)
				}
			} else {
				got, err := client.ListNodes(context.Background(), ListNodesOptions{View: tc.view, DataSource: tc.source})
				var decodeErr *json.UnmarshalTypeError
				if !errors.As(err, &decodeErr) || !reflect.DeepEqual(got, NodesPage{}) {
					t.Fatalf("got result %#v and error %v; want zero result and decoding error", got, err)
				}
			}
		})
	}
}

func TestDescribeNodeCanceledContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("canceled request reached server")
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err := client.DescribeNodeWithOptions(ctx, "node-1", DescribeNodeOptions{DataSource: DataSourceInband})
	if !errors.Is(err, context.Canceled) || !reflect.DeepEqual(got, NodeDetails{}) {
		t.Fatalf("got result %#v and error %v; want zero result and context cancellation", got, err)
	}
}

func TestReportResponseDecodingErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		path   string
		source DataSource
		view   ErrorReportView
		group  ErrorReportGroupBy
		body   string
	}{
		{"inventory inband", "/v1/reports/inventory", DataSourceInband, "", "", `{"page":"not-an-integer"}`},
		{"inventory oob", "/v1/reports/inventory", DataSourceOOB, "", "", `{"page":"not-an-integer"}`},
		{"errors list by error", "/v1/reports/error", DataSourceInband, ErrorReportViewList, ErrorReportGroupByError, `{"page":"not-an-integer"}`},
		{"errors list by node", "/v1/reports/error", DataSourceOOB, ErrorReportViewList, ErrorReportGroupByNode, `{"page":"not-an-integer"}`},
		{"errors graph", "/v1/reports/error", DataSourceInband, ErrorReportViewGraph, ErrorReportGroupByError, `{"result":"not-an-array"}`},
		{"errors overview", "/v1/reports/error", DataSourceOOB, ErrorReportViewOverview, "", `{"totalErrors":"not-an-integer"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := make(chan struct{}, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				select {
				case requests <- struct{}{}:
				default:
					t.Error("unexpected extra API request while recorder is full")
				}
				if r.Method != http.MethodGet || r.URL.Path != tc.path || r.URL.Query().Get("dataSource") != string(tc.source) || r.URL.Query().Get("view") != string(tc.view) || r.URL.Query().Get("groupBy") != string(tc.group) {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
				}
				w.Header().Set("Content-Type", "text/plain")
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			client, err := NewClient(server.URL, "test-key")
			if err != nil {
				t.Fatal(err)
			}
			var decodeErr *json.UnmarshalTypeError
			if tc.path == "/v1/reports/inventory" {
				got, err := client.GetInventoryReport(context.Background(), InventoryReportOptions{DataSource: tc.source})
				if !errors.As(err, &decodeErr) || !reflect.DeepEqual(got, InventoryReport{DataSource: tc.source}) {
					t.Fatalf("got result %#v and error %v; want empty payload and decoding error", got, err)
				}
			} else {
				got, err := client.GetErrorReport(context.Background(), ErrorReportOptions{DataSource: tc.source, View: tc.view, GroupBy: tc.group})
				if !errors.As(err, &decodeErr) || !reflect.DeepEqual(got, ErrorReport{DataSource: tc.source}) {
					t.Fatalf("got result %#v and error %v; want empty payload and decoding error", got, err)
				}
			}
			select {
			case <-requests:
			default:
				t.Fatal("request did not reach server")
			}
		})
	}
}

func TestReportForbiddenResponses(t *testing.T) {
	for _, tc := range []struct {
		name string
		path string
	}{
		{"inventory", "/v1/reports/inventory"},
		{"errors", "/v1/reports/error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := make(chan struct{}, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				select {
				case requests <- struct{}{}:
				default:
					t.Error("unexpected extra API request while recorder is full")
				}
				if r.Method != http.MethodGet || r.URL.Path != tc.path || r.URL.Query().Get("dataSource") != string(DataSourceInband) || (tc.name == "errors" && (r.URL.Query().Get("view") != string(ErrorReportViewList) || r.URL.Query().Get("groupBy") != string(ErrorReportGroupByError))) {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"permission denied"}`))
			}))
			defer server.Close()
			client, err := NewClient(server.URL, "test-key")
			if err != nil {
				t.Fatal(err)
			}
			if tc.name == "inventory" {
				got, callErr := client.GetInventoryReport(context.Background(), InventoryReportOptions{DataSource: DataSourceInband})
				err = callErr
				if !reflect.DeepEqual(got, InventoryReport{}) {
					t.Fatalf("unexpected report: %#v", got)
				}
			} else {
				got, callErr := client.GetErrorReport(context.Background(), ErrorReportOptions{DataSource: DataSourceInband, View: ErrorReportViewList, GroupBy: ErrorReportGroupByError})
				err = callErr
				if !reflect.DeepEqual(got, ErrorReport{}) {
					t.Fatalf("unexpected report: %#v", got)
				}
			}
			var apiErr *APIError
			if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusForbidden || apiErr.Message != "permission denied" {
				t.Fatalf("got error %v; want 403 APIError with permission denied message", err)
			}
			select {
			case <-requests:
			default:
				t.Fatal("request did not reach server")
			}
		})
	}
}

func TestReportCanceledContexts(t *testing.T) {
	for _, endpoint := range []string{"inventory", "errors"} {
		t.Run(endpoint, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
				t.Error("canceled request reached server")
			}))
			defer server.Close()
			client, err := NewClient(server.URL, "test-key")
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if endpoint == "inventory" {
				got, err := client.GetInventoryReport(ctx, InventoryReportOptions{DataSource: DataSourceInband})
				if !errors.Is(err, context.Canceled) || !reflect.DeepEqual(got, InventoryReport{}) {
					t.Fatalf("got result %#v and error %v; want zero result and context cancellation", got, err)
				}
			} else {
				got, err := client.GetErrorReport(ctx, ErrorReportOptions{DataSource: DataSourceInband, View: ErrorReportViewList, GroupBy: ErrorReportGroupByError})
				if !errors.Is(err, context.Canceled) || !reflect.DeepEqual(got, ErrorReport{}) {
					t.Fatalf("got result %#v and error %v; want zero result and context cancellation", got, err)
				}
			}
		})
	}
}

func TestFetchSigningKeyRetryAfterInterrupted(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != signingKeyPath {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatal(err)
	}
	retryer, ok := client.requestDoer.(*retryingDoer)
	if !ok {
		t.Fatalf("request doer has type %T; want *retryingDoer", client.requestDoer)
	}
	var delays []time.Duration
	retryer.wait = func(_ context.Context, delay time.Duration) error {
		delays = append(delays, delay)
		return context.Canceled
	}
	key, err := client.FetchSigningKey(context.Background())
	if !errors.Is(err, context.Canceled) || key != nil {
		t.Fatalf("got key %q and error %v; want cancellation and no key", key, err)
	}
	if len(delays) != 1 || delays[0] != 7*time.Second {
		t.Errorf("requested delays %v; want one delay of 7s", delays)
	}
	if got := requests.Load(); got != 1 {
		t.Errorf("server received %d requests; want 1", got)
	}
}

func TestRetryingDoerDoesNotReplayGETBody(t *testing.T) {
	calls := 0
	original := &http.Response{StatusCode: http.StatusServiceUnavailable, Body: io.NopCloser(strings.NewReader("unavailable")), Header: make(http.Header)}
	transport := roundTripperFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return original, nil
	})
	doer := &retryingDoer{inner: &http.Client{Transport: transport}, maxAttempts: defaultRequestAttempts}
	req, err := http.NewRequest(http.MethodGet, "https://example.com", strings.NewReader("body"))
	if err != nil {
		t.Fatal(err)
	}
	response, err := doer.Do(req)
	if response != nil && response.Body != nil {
		defer response.Body.Close()
	}
	if err != nil || response != original {
		t.Fatalf("got response %p and error %v; want original response %p", response, err, original)
	}
	if calls != 1 {
		t.Errorf("transport called %d times; want 1", calls)
	}
}

func TestFetchSigningKeyTransportDeadlineReportsConfiguredTimeout(t *testing.T) {
	calls := 0
	transport := roundTripperFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, context.DeadlineExceeded
	})
	const timeout = 7 * time.Second
	client, err := NewClient("https://example.com", "test-key", WithHTTPClient(&http.Client{Transport: transport}), WithTimeout(timeout))
	if err != nil {
		t.Fatal(err)
	}
	key, err := client.FetchSigningKey(context.Background())
	if key != nil || err == nil || err.Error() != "request timed out after 7s" {
		t.Fatalf("got key %q and error %v; want no key and configured timeout error", key, err)
	}
	if calls != 1 {
		t.Errorf("transport called %d times; want 1", calls)
	}
}

func alertBoundaryCalls() []struct {
	name string
	path string
	zero any
	call func(context.Context, *Client) (any, error)
} {
	return []struct {
		name string
		path string
		zero any
		call func(context.Context, *Client) (any, error)
	}{
		{"alerts", "/v1/alerts", AlertsPage{}, func(ctx context.Context, c *Client) (any, error) {
			return c.ListAlerts(ctx, ListAlertsOptions{})
		}},
		{"timeline nodes", "/v1/alert_timeline/nodes", AlertTimelineNodesPage{}, func(ctx context.Context, c *Client) (any, error) {
			return c.ListAlertTimelineNodes(ctx, ListAlertTimelineNodesOptions{})
		}},
		{"node timeline", "/v1/alert_timeline/nodes/node-1/alerts", NodeAlertTimelinePage{}, func(ctx context.Context, c *Client) (any, error) {
			return c.ListNodeAlertTimeline(ctx, ListNodeAlertTimelineOptions{NodeUUID: "node-1"})
		}},
		{"timeline detail", "/v1/alert_timeline/nodes/node-1/alerts/alert-1", AlertTimelineDetails{}, func(ctx context.Context, c *Client) (any, error) {
			return c.DescribeAlertTimelineWithOptions(ctx, "node-1", "alert-1", DescribeAlertTimelineOptions{})
		}},
	}
}

func TestAlertResponseErrors(t *testing.T) {
	for _, endpoint := range alertBoundaryCalls() {
		for _, response := range []struct {
			name, contentType, body string
			status                  int
		}{
			{"forbidden", "application/json", `{"message":"permission denied"}`, http.StatusForbidden},
			{"malformed", "text/plain", `{`, http.StatusOK},
		} {
			t.Run(endpoint.name+"/"+response.name, func(t *testing.T) {
				var requests atomic.Int64
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					requests.Add(1)
					if r.Method != http.MethodGet || r.URL.Path != endpoint.path {
						t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
					}
					w.Header().Set("Content-Type", response.contentType)
					w.WriteHeader(response.status)
					_, _ = w.Write([]byte(response.body))
				}))
				defer server.Close()
				client, err := NewClient(server.URL, "test-key")
				if err != nil {
					t.Fatal(err)
				}
				got, err := endpoint.call(context.Background(), client)
				if !reflect.DeepEqual(got, endpoint.zero) {
					t.Errorf("got result %#v; want zero value %#v", got, endpoint.zero)
				}
				if response.status == http.StatusForbidden {
					var apiErr *APIError
					if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusForbidden || apiErr.Message != response.body || !strings.Contains(apiErr.Error(), "permission denied") {
						t.Errorf("got error %v; want 403 APIError preserving the permission denied response body", err)
					}
				} else {
					var syntaxErr *json.SyntaxError
					if !errors.As(err, &syntaxErr) {
						t.Errorf("got error %v; want JSON syntax error", err)
					}
				}
				if got := requests.Load(); got != 1 {
					t.Errorf("server received %d requests; want 1", got)
				}
			})
		}
	}
}

func TestAlertCanceledContexts(t *testing.T) {
	calls := alertBoundaryCalls()
	calls = append(calls, struct {
		name string
		path string
		zero any
		call func(context.Context, *Client) (any, error)
	}{"filter options", "", AlertTimelineFilterOptions{}, func(ctx context.Context, c *Client) (any, error) {
		return c.GetAlertTimelineFilterOptions(ctx, false)
	}})
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range calls {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			got, err := tc.call(ctx, client)
			if !errors.Is(err, context.Canceled) || !reflect.DeepEqual(got, tc.zero) {
				t.Errorf("got result %#v and error %v; want zero value and context cancellation", got, err)
			}
			if got := requests.Load(); got != 0 {
				t.Errorf("server received %d requests; want none", got)
			}
		})
	}
}

func TestAlertTimelineValidationBoundaries(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, identifierError string
		zero                  any
		call                  func() (any, error)
	}{
		{"node timeline missing node", "node UUID is required", NodeAlertTimelinePage{}, func() (any, error) {
			return client.ListNodeAlertTimeline(context.Background(), ListNodeAlertTimelineOptions{})
		}},
		{"detail missing node", "node UUID is required", AlertTimelineDetails{}, func() (any, error) {
			return client.DescribeAlertTimeline(context.Background(), "", "alert-1")
		}},
		{"detail missing alert", "alert UUID is required", AlertTimelineDetails{}, func() (any, error) {
			return client.DescribeAlertTimeline(context.Background(), "node-1", "")
		}},
		{"nodes invalid order", "", AlertTimelineNodesPage{}, func() (any, error) {
			return client.ListAlertTimelineNodes(context.Background(), ListAlertTimelineNodesOptions{Order: "sideways"})
		}},
		{"node timeline invalid order", "", NodeAlertTimelinePage{}, func() (any, error) {
			return client.ListNodeAlertTimeline(context.Background(), ListNodeAlertTimelineOptions{NodeUUID: "node-1", Order: "sideways"})
		}},
		{"detail invalid order", "", AlertTimelineDetails{}, func() (any, error) {
			return client.DescribeAlertTimelineWithOptions(context.Background(), "node-1", "alert-1", DescribeAlertTimelineOptions{Order: "sideways"})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.call()
			if !reflect.DeepEqual(got, tc.zero) {
				t.Errorf("got result %#v; want zero value %#v", got, tc.zero)
			}
			if tc.identifierError != "" {
				if err == nil || err.Error() != tc.identifierError {
					t.Errorf("got error %v; want %q", err, tc.identifierError)
				}
			} else {
				assertReportOptionError(t, err, "order", "sideways")
			}
			if got := requests.Load(); got != 0 {
				t.Errorf("server received %d requests; want none", got)
			}
		})
	}
}

func TestDescribeAlertTimelineConvenience(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != "/v1/alert_timeline/nodes/node-1/alerts/alert-1" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		for _, key := range []string{"page", "pageSize", "order"} {
			if _, ok := r.URL.Query()[key]; ok {
				t.Errorf("unexpected query parameter %s: %v", key, r.URL.Query())
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"alertUuid":"alert-1","nodeUuid":"node-1","timeline":[]}`))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.DescribeAlertTimeline(context.Background(), "node-1", "alert-1")
	if err != nil || got.AlertUUID != "alert-1" {
		t.Fatalf("got detail %#v and error %v; want alert-1", got, err)
	}
	if got := requests.Load(); got != 1 {
		t.Errorf("server received %d requests; want 1", got)
	}
}

func TestFetchSigningKeyCustomRedirectPolicy(t *testing.T) {
	const key = "known signing key"
	policyErr := errors.New("redirect rejected by caller")
	for _, tc := range []struct {
		name        string
		sameOrigin  bool
		tls         bool
		policyError error
		wantError   error
		wantCalls   int64
		wantTargets int64
	}{
		{name: "same origin policy rejects", sameOrigin: true, policyError: policyErr, wantError: policyErr, wantCalls: 1},
		{name: "cross port policy allows", wantCalls: 1, wantTargets: 1},
		{name: "https downgrade rejected before policy", tls: true, wantError: ErrInsecureBaseURL},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var origins, targets, callbacks atomic.Int64
			callbackAuth := make(chan string, 1)
			targetAuth := make(chan string, 1)
			targetHandler := func(w http.ResponseWriter, r *http.Request) {
				targets.Add(1)
				select {
				case targetAuth <- r.Header.Get("Authorization"):
				default:
				}
				_, _ = w.Write([]byte(key))
			}
			destination := httptest.NewServer(http.HandlerFunc(targetHandler))
			defer destination.Close()

			originHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/redirected" {
					targetHandler(w, r)
					return
				}
				origins.Add(1)
				if r.Method != http.MethodGet || r.URL.Path != signingKeyPath || r.Header.Get("Authorization") != "Bearer test-key" {
					t.Errorf("unexpected origin request: %s %s, authorization %q", r.Method, r.URL.Path, r.Header.Get("Authorization"))
				}
				location := destination.URL + "/redirected"
				if tc.sameOrigin {
					location = "/redirected"
				}
				http.Redirect(w, r, location, http.StatusFound)
			})
			var origin *httptest.Server
			if tc.tls {
				origin = httptest.NewTLSServer(originHandler)
			} else {
				origin = httptest.NewServer(originHandler)
			}
			defer origin.Close()

			supplied := origin.Client()
			supplied.CheckRedirect = func(req *http.Request, _ []*http.Request) error {
				callbacks.Add(1)
				select {
				case callbackAuth <- req.Header.Get("Authorization"):
				default:
				}
				return tc.policyError
			}
			client, err := NewClient(origin.URL, "test-key", WithHTTPClient(supplied))
			if err != nil {
				t.Fatal(err)
			}
			if client.httpClient == supplied {
				t.Fatal("client retained the supplied HTTP client instead of a shallow copy")
			}
			got, err := client.FetchSigningKey(context.Background())
			if tc.wantError != nil {
				if !errors.Is(err, tc.wantError) || got != nil {
					t.Fatalf("got key %q and error %v; want no key and %v", got, err, tc.wantError)
				}
			} else if err != nil || string(got) != key {
				t.Fatalf("got key %q and error %v; want %q", got, err, key)
			}
			if origins.Load() != 1 || callbacks.Load() != tc.wantCalls || targets.Load() != tc.wantTargets {
				t.Fatalf("origin requests %d, callbacks %d, destination requests %d; want 1, %d, %d", origins.Load(), callbacks.Load(), targets.Load(), tc.wantCalls, tc.wantTargets)
			}
			if tc.wantTargets == 1 {
				if auth := <-callbackAuth; auth != "" {
					t.Errorf("callback saw authorization %q; want none", auth)
				}
				if auth := <-targetAuth; auth != "" {
					t.Errorf("destination saw authorization %q; want none", auth)
				}
			}

			// Calling the supplied policy directly must still invoke the original
			// callback, without the SDK's guard or changes to its result.
			before := callbacks.Load()
			probe, err := http.NewRequest(http.MethodGet, destination.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			probe.Header.Set("Authorization", "probe")
			// Drain the earlier callback observation before probing the policy.
			if tc.wantCalls == 1 && tc.wantTargets == 0 {
				<-callbackAuth
			}
			if supplied.CheckRedirect == nil {
				t.Fatal("supplied redirect policy was cleared")
			}
			if err := supplied.CheckRedirect(probe, nil); err != tc.policyError || callbacks.Load() != before+1 {
				t.Errorf("supplied policy returned %v and invoked %d callbacks; want %v and one callback", err, callbacks.Load()-before, tc.policyError)
			}
			select {
			case auth := <-callbackAuth:
				if auth != "probe" {
					t.Errorf("supplied policy saw authorization %q; want probe", auth)
				}
			default:
				t.Error("supplied policy did not record the probe")
			}
		})
	}
}

func TestGuardRedirectHTTPLoopbackOriginBoundaries(t *testing.T) {
	const authorization = "Bearer secret"
	for _, tc := range []struct {
		name     string
		via      []string
		target   string
		wantAuth string
	}{
		{name: "implicit to explicit default port", via: []string{"http://localhost/path"}, target: "http://localhost:80/next", wantAuth: authorization},
		{name: "explicit to implicit default port", via: []string{"http://localhost:80/path"}, target: "http://localhost/next", wantAuth: authorization},
		{name: "different port", via: []string{"http://localhost/path"}, target: "http://localhost:8080/next"},
		{name: "multi hop compares original origin", via: []string{"http://localhost", "http://localhost:8080"}, target: "http://localhost:8080/next"},
		{name: "empty via", target: "http://localhost:8080/next", wantAuth: authorization},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := &http.Request{URL: mustParseURL(t, tc.target), Header: make(http.Header)}
			req.Header.Set("Authorization", authorization)
			var via []*http.Request
			for _, raw := range tc.via {
				via = append(via, &http.Request{URL: mustParseURL(t, raw)})
			}
			if err := guardRedirect(req, via); err != nil {
				t.Fatalf("guardRedirect() error = %v; want nil", err)
			}
			if got := req.Header.Get("Authorization"); got != tc.wantAuth {
				t.Errorf("Authorization = %q; want %q", got, tc.wantAuth)
			}
		})
	}
}

func TestComputeZoneMalformedResponses(t *testing.T) {
	for _, view := range []ComputeZoneView{ComputeZoneViewBasic, ComputeZoneViewDetail} {
		t.Run(string(view), func(t *testing.T) {
			var requests atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.Method != http.MethodGet || r.URL.Path != "/v1/computezones" || r.URL.Query().Get("view") != string(view) {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
				}
				w.Header().Set("Content-Type", "text/plain")
				_, _ = w.Write([]byte(`{invalid`))
			}))
			defer server.Close()
			client, err := NewClient(server.URL, "test-key")
			if err != nil {
				t.Fatal(err)
			}
			got, err := client.ListComputeZones(context.Background(), ListComputeZonesOptions{View: view})
			var syntaxErr *json.SyntaxError
			if !errors.As(err, &syntaxErr) || !reflect.DeepEqual(got, ComputeZonesPage{}) {
				t.Fatalf("got page %#v and error %v; want zero page and JSON syntax error", got, err)
			}
			if requests.Load() != 1 {
				t.Errorf("got %d requests; want 1", requests.Load())
			}
		})
	}

	t.Run("update", func(t *testing.T) {
		methods := make(chan string, 10)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			methods <- r.Method
			if r.URL.Path != "/v1/computezones" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}
			if r.Method == http.MethodGet {
				if r.URL.Query().Get("view") != "basic" {
					t.Errorf("unexpected view: %s", r.URL.RawQuery)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"computezones":[` + storedComputeZone + `]}`))
				return
			}
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte(`{invalid`))
		}))
		defer server.Close()
		client, err := NewClient(server.URL, "test-key")
		if err != nil {
			t.Fatal(err)
		}
		email := "updated@example.com"
		got, err := client.UpdateComputeZone(context.Background(), "cz-1", UpdateComputeZoneOptions{ContactEmail: &email})
		var syntaxErr *json.SyntaxError
		if !errors.As(err, &syntaxErr) || !reflect.DeepEqual(got, ComputeZoneUpdate{}) {
			t.Fatalf("got update %#v and error %v; want zero update and JSON syntax error", got, err)
		}
		close(methods)
		var order []string
		for method := range methods {
			order = append(order, method)
		}
		if !reflect.DeepEqual(order, []string{http.MethodGet, http.MethodPut}) {
			t.Errorf("request order %v; want GET then PUT", order)
		}
	})
}

func TestComputeZoneInvalidUpdateDoesNotRequest(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		call func() (any, error)
		zero any
	}{
		{"update", func() (any, error) {
			zoneType := "bogus"
			return client.UpdateComputeZone(context.Background(), "cz-1", UpdateComputeZoneOptions{Type: &zoneType})
		}, ComputeZoneUpdate{}},
		{"preview", func() (any, error) {
			zoneType := "bogus"
			return client.PreviewUpdateComputeZone(context.Background(), "cz-1", UpdateComputeZoneOptions{Type: &zoneType})
		}, RequestPreview{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.call()
			if err == nil || !strings.Contains(err.Error(), `invalid compute zone type "bogus"`) || !reflect.DeepEqual(got, tc.zero) {
				t.Fatalf("got %#v and error %v; want zero result and invalid type error", got, err)
			}
			if requests.Load() != 0 {
				t.Errorf("got %d requests; want none", requests.Load())
			}
		})
	}
}

func TestComputeZoneTransportFailures(t *testing.T) {
	for _, tc := range []struct {
		name       string
		update     bool
		failMethod string
		wantOrder  []string
	}{
		{"list", false, http.MethodGet, []string{http.MethodGet}},
		{"update GET", true, http.MethodGet, []string{http.MethodGet}},
		{"update PUT", true, http.MethodPut, []string{http.MethodGet, http.MethodPut}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/computezones" || r.URL.Query().Get("view") != "basic" {
					t.Errorf("unexpected request reaching server: %s %s", r.Method, r.URL.String())
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"computezones":[` + storedComputeZone + `]}`))
			}))
			defer server.Close()
			sentinel := errors.New("transport failure")
			var methods []string
			transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				methods = append(methods, req.Method)
				if req.Method == tc.failMethod {
					return nil, sentinel
				}
				return server.Client().Transport.RoundTrip(req)
			})
			client, err := NewClient(server.URL, "test-key", WithHTTPClient(&http.Client{Transport: transport}))
			if err != nil {
				t.Fatal(err)
			}
			var got, zero any
			if tc.update {
				email := "updated@example.com"
				got, err = client.UpdateComputeZone(context.Background(), "cz-1", UpdateComputeZoneOptions{ContactEmail: &email})
				zero = ComputeZoneUpdate{}
			} else {
				got, err = client.ListComputeZones(context.Background(), ListComputeZonesOptions{View: ComputeZoneViewBasic})
				zero = ComputeZonesPage{}
			}
			if !errors.Is(err, sentinel) || !reflect.DeepEqual(got, zero) {
				t.Fatalf("got %#v and error %v; want zero result and sentinel error", got, err)
			}
			if !reflect.DeepEqual(methods, tc.wantOrder) {
				t.Errorf("request order %v; want %v", methods, tc.wantOrder)
			}
		})
	}
}

func TestListTagsResourceScopes(t *testing.T) {
	for _, tc := range []struct {
		name  string
		opts  TagListOptions
		field string
	}{
		{name: "node", opts: TagListOptions{Prefix: "  rack  ", NodeUUID: "  node-1  "}, field: "nodeUUID"},
		{name: "compute zone", opts: TagListOptions{Prefix: "  rack  ", ComputeZoneID: "  zone-1  "}, field: "computeZoneId"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const payload = `{"tags":["rack-blue","rack-red"]}`
			var requests atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				q := r.URL.Query()
				wantID := "node-1"
				if tc.field == "computeZoneId" {
					wantID = "zone-1"
				}
				if r.Method != http.MethodGet || r.URL.Path != "/v1/tags" || q.Get("prefix") != "rack" || q.Get(tc.field) != wantID {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
				}
				for _, field := range []string{"nodeUUID", "nodeGroupId", "computeZoneId"} {
					if field != tc.field && q.Has(field) {
						t.Errorf("unexpected resource filter %q in %s", field, r.URL.RawQuery)
					}
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(payload))
			}))
			defer server.Close()
			client, err := NewClient(server.URL, "test-key")
			if err != nil {
				t.Fatal(err)
			}
			got, err := client.ListTags(context.Background(), tc.opts)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got.Tags, []string{"rack-blue", "rack-red"}) || string(got.RawJSON) != payload || requests.Load() != 1 {
				t.Fatalf("got %#v after %d requests", got, requests.Load())
			}
		})
	}
}

func TestTagMethodsMalformedSuccessResponses(t *testing.T) {
	for _, tc := range []struct {
		name        string
		contentType string
		set         bool
	}{
		{name: "list text", contentType: "text/plain"},
		{name: "list json", contentType: "application/json"},
		{name: "set text", contentType: "text/plain", set: true},
		{name: "set json", contentType: "application/json", set: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				w.Header().Set("Content-Type", tc.contentType)
				_, _ = w.Write([]byte(`{"tags":`))
			}))
			defer server.Close()
			client, err := NewClient(server.URL, "test-key")
			if err != nil {
				t.Fatal(err)
			}
			if tc.set {
				got, err := client.SetNodeTags(context.Background(), "node-1", SetNodeTagsOptions{Tags: []string{"rack-blue"}})
				var syntaxErr *json.SyntaxError
				if !errors.As(err, &syntaxErr) || !reflect.DeepEqual(got, NodeTags{}) {
					t.Fatalf("got result %#v and error %v; want zero result and JSON syntax error", got, err)
				}
			} else {
				got, err := client.ListTags(context.Background(), TagListOptions{NodeUUID: "node-1"})
				var syntaxErr *json.SyntaxError
				if !errors.As(err, &syntaxErr) || !reflect.DeepEqual(got, TagList{}) {
					t.Fatalf("got result %#v and error %v; want zero result and JSON syntax error", got, err)
				}
			}
			if requests.Load() != 1 {
				t.Fatalf("got %d requests; want 1", requests.Load())
			}
		})
	}
}

func TestTagMethodsCanceledContext(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  bool
	}{
		{name: "list"},
		{name: "set", set: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
			}))
			defer server.Close()
			client, err := NewClient(server.URL, "test-key")
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if tc.set {
				got, err := client.SetNodeTags(ctx, "node-1", SetNodeTagsOptions{Tags: []string{"rack-blue"}})
				if !errors.Is(err, context.Canceled) || !reflect.DeepEqual(got, NodeTags{}) {
					t.Fatalf("got result %#v and error %v; want zero result and context cancellation", got, err)
				}
			} else {
				got, err := client.ListTags(ctx, TagListOptions{NodeUUID: "node-1"})
				if !errors.Is(err, context.Canceled) || !reflect.DeepEqual(got, TagList{}) {
					t.Fatalf("got result %#v and error %v; want zero result and context cancellation", got, err)
				}
			}
			if requests.Load() != 0 {
				t.Fatalf("got %d requests; want none", requests.Load())
			}
		})
	}
}

type interruptedTagBody struct {
	partial *strings.Reader
	err     error
	closed  bool
}

func (b *interruptedTagBody) Read(p []byte) (int, error) {
	n, err := b.partial.Read(p)
	if err == io.EOF {
		return n, b.err
	}
	return n, err
}

func (b *interruptedTagBody) Close() error {
	b.closed = true
	return nil
}

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

func TestTagMethodsInterruptedResponseBody(t *testing.T) {
	for _, tc := range []struct {
		name   string
		method string
		path   string
		zero   any
		call   func(*Client) (any, error)
	}{
		{"list", http.MethodGet, "/v1/tags", TagList{}, func(c *Client) (any, error) {
			return c.ListTags(context.Background(), TagListOptions{NodeGroupID: "group-1"})
		}},
		{"set", http.MethodPut, "/v1/nodes/node-1/tags", NodeTags{}, func(c *Client) (any, error) {
			return c.SetNodeTags(context.Background(), "node-1", SetNodeTagsOptions{Tags: []string{"rack-blue"}})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sentinel := errors.New("interrupted response body")
			body := &interruptedTagBody{partial: strings.NewReader(`{"tags":["rack-blue"`), err: sentinel}
			requests := 0
			transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				requests++
				if req.Method != tc.method || req.URL.Path != tc.path {
					t.Errorf("got request %s %s; want %s %s", req.Method, req.URL.String(), tc.method, tc.path)
				}
				if tc.method == http.MethodGet {
					if req.URL.Query().Get("nodeGroupId") != "group-1" {
						t.Errorf("got query %s; want nodeGroupId=group-1", req.URL.RawQuery)
					}
				} else {
					var payload struct {
						Tags []string `json:"tags"`
					}
					if err := json.NewDecoder(req.Body).Decode(&payload); err != nil || !reflect.DeepEqual(payload.Tags, []string{"rack-blue"}) {
						t.Errorf("got request body %#v and error %v; want replacement tags", payload, err)
					}
				}
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: body, Request: req}, nil
			})
			client, err := NewClient("https://example.com", "test-key", WithHTTPClient(&http.Client{Transport: transport}))
			if err != nil {
				t.Fatal(err)
			}
			got, err := tc.call(client)
			if !errors.Is(err, sentinel) || !reflect.DeepEqual(got, tc.zero) {
				t.Errorf("got result %#v and error %v; want zero result and sentinel error", got, err)
			}
			if requests != 1 || !body.closed {
				t.Errorf("got %d requests and body closed %t; want one request and closed body", requests, body.closed)
			}
		})
	}
}

type xidFailureEndpoint struct {
	name string
	path string
	call func(*Client) (any, error)
	zero any
}

func xidFailureEndpoints() []xidFailureEndpoint {
	return []xidFailureEndpoint{
		{name: "list", path: "/v1/xid/bursts", zero: XIDBurstsPage{}, call: func(c *Client) (any, error) {
			return c.ListXIDBursts(context.Background(), ListXIDBurstsOptions{Window: "24h"})
		}},
		{name: "detail", path: "/v1/xid/bursts/burst-1", zero: XIDBurstDetails{}, call: func(c *Client) (any, error) {
			return c.DescribeXIDBurst(context.Background(), "burst-1")
		}},
		{name: "options", path: "/v1/xid/bursts/options", zero: XIDBurstFilterOptions{}, call: func(c *Client) (any, error) {
			return c.GetXIDBurstFilterOptions(context.Background(), XIDBurstFilterOptionsScope{})
		}},
	}
}

func TestXIDBurstTransportFailures(t *testing.T) {
	for _, tc := range xidFailureEndpoints() {
		t.Run(tc.name, func(t *testing.T) {
			sentinel := errors.New("transport failure")
			calls := 0
			transport := roundTripperFunc(func(*http.Request) (*http.Response, error) {
				calls++
				return nil, sentinel
			})
			client, err := NewClient("https://example.com", "test-key", WithHTTPClient(&http.Client{Transport: transport}))
			if err != nil {
				t.Fatal(err)
			}
			got, err := tc.call(client)
			if !errors.Is(err, sentinel) || !reflect.DeepEqual(got, tc.zero) || calls != 1 {
				t.Fatalf("got result %#v, error %v, and %d calls; want zero result and transport error", got, err, calls)
			}
		})
	}
}

func TestXIDBurstMalformedSuccessfulResponses(t *testing.T) {
	for _, tc := range xidFailureEndpoints() {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != http.MethodGet || r.URL.Path != tc.path {
					t.Errorf("got request %s %s; want GET %s", r.Method, r.URL.Path, tc.path)
				}
				w.Header().Set("Content-Type", "text/plain")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("{!"))
			}))
			defer server.Close()
			client, err := NewClient(server.URL, "test-key")
			if err != nil {
				t.Fatal(err)
			}
			got, err := tc.call(client)
			var syntaxErr *json.SyntaxError
			if !errors.As(err, &syntaxErr) || !reflect.DeepEqual(got, tc.zero) || calls.Load() != 1 {
				t.Fatalf("got result %#v, error %v, and %d calls; want zero result and JSON syntax error", got, err, calls.Load())
			}
			if tc.name == "options" && !strings.Contains(err.Error(), "decode XID burst filter options") {
				t.Fatalf("options error lacks decode context: %v", err)
			}
		})
	}
}

type xidOptionsFailingBody struct {
	err    error
	closed bool
}

func (b *xidOptionsFailingBody) Read([]byte) (int, error) { return 0, b.err }
func (b *xidOptionsFailingBody) Close() error {
	b.closed = true
	return nil
}

func TestXIDBurstFilterOptionsReadFailure(t *testing.T) {
	sentinel := errors.New("body read failure")
	body := &xidOptionsFailingBody{err: sentinel}
	transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: body, Request: req}, nil
	})
	client, err := NewClient("https://example.com", "test-key", WithHTTPClient(&http.Client{Transport: transport}))
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.GetXIDBurstFilterOptions(context.Background(), XIDBurstFilterOptionsScope{})
	if !errors.Is(err, sentinel) || !strings.Contains(err.Error(), "read XID burst filter options") || !reflect.DeepEqual(got, XIDBurstFilterOptions{}) || !body.closed {
		t.Fatalf("got options %#v, error %v, body closed %t; want zero options, wrapped read error, and closed body", got, err, body.closed)
	}
}

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

func TestNodeHealthHistoryTransportFailure(t *testing.T) {
	sentinel := errors.New("health history transport failure")
	calls := 0
	transport := roundTripperFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, sentinel
	})
	client, err := NewClient("https://example.com", "test-key", WithHTTPClient(&http.Client{Transport: transport}))
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.NodeHealthHistory(context.Background(), "node-1", NodeHealthHistoryOptions{
		StartTime: "2026-04-07T00:00:00Z", EndTime: "2026-04-14T00:00:00Z",
	})
	if !errors.Is(err, sentinel) || !reflect.DeepEqual(got, NodeHealthHistory{}) || calls != 1 {
		t.Fatalf("got history %#v, error %v, and %d calls; want zero history and transport error", got, err, calls)
	}
}

func TestNodeHealthHistoryMalformedResponse(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != "/v1/nodes/node-1/health_history" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{!"))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.NodeHealthHistory(context.Background(), "node-1", NodeHealthHistoryOptions{
		StartTime: "2026-04-07T00:00:00Z", EndTime: "2026-04-14T00:00:00Z",
	})
	var syntaxErr *json.SyntaxError
	if !errors.As(err, &syntaxErr) || !reflect.DeepEqual(got, NodeHealthHistory{}) || calls.Load() != 1 {
		t.Fatalf("got history %#v, error %v, and %d calls; want zero history and JSON syntax error", got, err, calls.Load())
	}
}

func TestNodeHealthHistoryWithoutSummary(t *testing.T) {
	const payload = `{"enrolledAt":"2026-01-01T00:00:00Z","machineStatus":[{"status":"Degraded","startTime":"2026-04-07T00:00:00Z","endTime":"2026-04-14T00:00:00Z"}]}`
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != "/v1/nodes/node-1/health_history" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(payload))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.NodeHealthHistory(context.Background(), "node-1", NodeHealthHistoryOptions{
		StartTime: "2026-04-07T00:00:00Z", EndTime: "2026-04-14T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	wantSegments := []NodeHealthSegment{{Status: "Degraded", StartTime: "2026-04-07T00:00:00Z", EndTime: "2026-04-14T00:00:00Z"}}
	if got.HealthSummary != nil || got.EnrolledAt != "2026-01-01T00:00:00Z" || !reflect.DeepEqual(got.Segments, wantSegments) || string(got.RawJSON) != payload || calls.Load() != 1 {
		t.Fatalf("unexpected history %#v after %d calls", got, calls.Load())
	}
}

func TestGetAuthStatusResponseFailures(t *testing.T) {
	for _, tc := range []struct {
		name        string
		contentType string
		transport   bool
	}{
		{name: "transport error", transport: true},
		{name: "malformed JSON response", contentType: "application/json"},
		{name: "malformed text response", contentType: "text/plain"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int64
			sentinel := errors.New("auth status transport failure")
			var client *Client
			var err error
			if tc.transport {
				transport := roundTripperFunc(func(*http.Request) (*http.Response, error) {
					calls.Add(1)
					return nil, sentinel
				})
				client, err = NewClient("https://example.com", "test-key", WithHTTPClient(&http.Client{Transport: transport}))
			} else {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					if r.Method != http.MethodGet || r.URL.Path != "/v1/auth/status" || r.Header.Get("Authorization") != "Bearer test-key" {
						t.Errorf("unexpected request: %s %s, authorization %q", r.Method, r.URL.Path, r.Header.Get("Authorization"))
					}
					w.Header().Set("Content-Type", tc.contentType)
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte("{!"))
				}))
				defer server.Close()
				client, err = NewClient(server.URL, "test-key")
			}
			if err != nil {
				t.Fatal(err)
			}
			got, err := client.GetAuthStatus(context.Background())
			if !reflect.DeepEqual(got, AuthStatus{}) || calls.Load() != 1 {
				t.Errorf("got status %#v and %d requests; want zero status and one request", got, calls.Load())
			}
			if tc.transport {
				if !errors.Is(err, sentinel) {
					t.Fatalf("got error %v; want transport sentinel", err)
				}
			} else {
				var syntaxErr *json.SyntaxError
				if !errors.As(err, &syntaxErr) {
					t.Fatalf("got error %v; want JSON syntax error", err)
				}
			}
		})
	}
}

func TestOOBInventoryOptionalTopologyAndLinks(t *testing.T) {
	const payload = `{"source":{"address":"192.0.2.10","mac":"02:00:00:00:00:01","sourceType":"redfish"},"systems":[{"id":"system-1","links":{"missing":null,"empty":[],"related":[{"odataId":"/redfish/v1/Chassis/1"}]},"processors":[{"id":"gpu-1","nvidiaGpu":{}},{"id":"gpu-2","nvidiaGpu":{"nvlinkTopology":{"chassisSerialNumber":"chassis-1","ibGuid":"ib-1","lsbIbGuid":"lsb-1","systemGuid":"system-1","traySerialNumber":"tray-1","hostId":0}}}]}]}`
	var input fleetapi.ModelsOobInventory
	if err := json.Unmarshal([]byte(payload), &input); err != nil {
		t.Fatal(err)
	}
	got := oobInventoryFromGenerated(&input)
	if got == nil || len(got.Systems) != 1 || len(got.Systems[0].Processors) != 2 {
		t.Fatalf("unexpected inventory: %#v", got)
	}
	system := got.Systems[0]
	if system.Processors[0].NvidiaGPU == nil || system.Processors[0].NvidiaGPU.NVLinkTopology != nil {
		t.Fatalf("GPU without topology changed: %#v", system.Processors[0].NvidiaGPU)
	}
	gpu := system.Processors[1].NvidiaGPU
	if gpu == nil || gpu.NVLinkTopology == nil {
		t.Fatalf("missing GPU topology: %#v", gpu)
	}
	topology := gpu.NVLinkTopology
	if topology.ChassisSerialNumber != "chassis-1" || topology.IBGUID != "ib-1" || topology.LSBIBGUID != "lsb-1" || topology.SystemGUID != "system-1" || topology.TraySerialNumber != "tray-1" || topology.HostID == nil || *topology.HostID != 0 {
		t.Fatalf("unexpected topology: %#v", topology)
	}
	missing, missingOK := system.Links["missing"]
	empty, emptyOK := system.Links["empty"]
	related, relatedOK := system.Links["related"]
	if !missingOK || missing != nil || !emptyOK || empty == nil || len(empty) != 0 || !relatedOK || len(related) != 1 || related[0].ODataID != "/redfish/v1/Chassis/1" {
		t.Fatalf("unexpected links: %#v", system.Links)
	}
	wantJSON := oobJSONValue(t, input)
	if actual := oobJSONValue(t, got); !reflect.DeepEqual(actual, wantJSON) {
		t.Fatalf("SDK inventory changed JSON representation\ngot: %#v\nwant: %#v", actual, wantJSON)
	}
	generatedSystem := &(*input.Systems)[0]
	(*generatedSystem.Links)["related"][0].OdataId = "/changed"
	*(*generatedSystem.Processors)[1].NvidiaGpu.NvlinkTopology.HostId = 42
	if related[0].ODataID != "/redfish/v1/Chassis/1" || *topology.HostID != 0 || !reflect.DeepEqual(oobJSONValue(t, got), wantJSON) {
		t.Fatal("SDK inventory aliases generated reference or topology")
	}
}

func TestOverviewInvalidSuccessfulResponses(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "malformed JSON", body: `{`},
		{name: "incompatible field type", body: `{"nodesCount":"bad"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.Method != http.MethodGet || r.URL.Path != "/v1/overview" {
					t.Errorf("got request %s %s; want GET /v1/overview", r.Method, r.URL.Path)
				}
				w.Header().Set("Content-Type", "text/plain")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()

			client, err := NewClient(server.URL, "test-key")
			if err != nil {
				t.Fatal(err)
			}
			got, err := client.GetOverview(context.Background(), OverviewOptions{})
			if !reflect.DeepEqual(got, Overview{}) || requests.Load() != 1 {
				t.Errorf("got overview %#v and %d requests; want zero overview and one request", got, requests.Load())
			}
			if tc.name == "malformed JSON" {
				var syntaxErr *json.SyntaxError
				if !errors.As(err, &syntaxErr) {
					t.Fatalf("got error %v; want JSON syntax error", err)
				}
			} else {
				var typeErr *json.UnmarshalTypeError
				if !errors.As(err, &typeErr) {
					t.Fatalf("got error %v; want JSON unmarshal type error", err)
				}
			}
		})
	}
}
