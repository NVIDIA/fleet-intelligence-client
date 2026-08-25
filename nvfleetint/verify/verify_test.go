// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package verify

import (
	"strings"
	"testing"

	"github.com/sigstore/sigstore-go/pkg/bundle"
	"github.com/sigstore/sigstore-go/pkg/sign"
)

// signTestReport signs csv with a fresh ephemeral key and returns the Sigstore
// bundle JSON plus the PEM-encoded public key, mirroring what the API produces
// for a signed inventory report.
func signTestReport(t *testing.T, csv []byte) (bundleJSON, publicKeyPEM []byte) {
	t.Helper()

	keypair, err := sign.NewEphemeralKeypair(nil)
	if err != nil {
		t.Fatalf("new keypair failed: %v", err)
	}
	pem, err := keypair.GetPublicKeyPem()
	if err != nil {
		t.Fatalf("get public key failed: %v", err)
	}

	pb, err := sign.Bundle(&sign.PlainData{Data: csv}, keypair, sign.BundleOptions{})
	if err != nil {
		t.Fatalf("sign bundle failed: %v", err)
	}
	signed, err := bundle.NewBundle(pb)
	if err != nil {
		t.Fatalf("wrap bundle failed: %v", err)
	}
	data, err := signed.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal bundle failed: %v", err)
	}
	return data, []byte(pem)
}

// Verifies a correctly signed report passes verification
func TestVerifySignedReportRoundTrip(t *testing.T) {
	csv := []byte("customer,issued_at\nacme,2026-06-15T00:00:00Z\n")
	bundleJSON, key := signTestReport(t, csv)

	if err := VerifySignedReport(csv, bundleJSON, key); err != nil {
		t.Fatalf("expected verification to succeed, got: %v", err)
	}
}

// Verifies a tampered CSV fails verification
func TestVerifySignedReportTamperedCSV(t *testing.T) {
	csv := []byte("customer,issued_at\nacme,2026-06-15T00:00:00Z\n")
	bundleJSON, key := signTestReport(t, csv)

	tampered := append([]byte(nil), csv...)
	tampered[0] = 'X'

	if err := VerifySignedReport(tampered, bundleJSON, key); err == nil {
		t.Fatal("expected verification to fail for tampered csv")
	}
}

// Verifies a mismatched public key fails verification
func TestVerifySignedReportWrongKey(t *testing.T) {
	csv := []byte("customer,issued_at\nacme,2026-06-15T00:00:00Z\n")
	bundleJSON, _ := signTestReport(t, csv)
	_, otherKey := signTestReport(t, csv)

	if err := VerifySignedReport(csv, bundleJSON, otherKey); err == nil {
		t.Fatal("expected verification to fail with a mismatched key")
	}
}

// Verifies a non-PEM key is rejected with a clear error
func TestVerifySignedReportInvalidKey(t *testing.T) {
	csv := []byte("data\n")
	bundleJSON, _ := signTestReport(t, csv)

	err := VerifySignedReport(csv, bundleJSON, []byte("not-a-pem-key"))
	if err == nil || !strings.Contains(err.Error(), "PEM") {
		t.Fatalf("expected PEM error, got: %v", err)
	}
}

// Verifies a malformed bundle is rejected with a clear error
func TestVerifySignedReportInvalidBundle(t *testing.T) {
	csv := []byte("data\n")
	_, key := signTestReport(t, csv)

	err := VerifySignedReport(csv, []byte("{not valid json"), key)
	if err == nil || !strings.Contains(err.Error(), "bundle") {
		t.Fatalf("expected bundle parse error, got: %v", err)
	}
}
