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
	"sync/atomic"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/generated/fleetapi"
)

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
