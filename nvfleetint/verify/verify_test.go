// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package verify

import (
	"crypto/ecdh"
	"crypto/x509"
	"encoding/asn1"
	"encoding/pem"
	"errors"
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

func TestVerifySignedReportPublicKeyProcessingErrors(t *testing.T) {
	report := []byte("customer,issued_at\nacme,2026-06-15T00:00:00Z\n")
	bundleJSON, _ := signTestReport(t, report)

	x25519Key, err := ecdh.X25519().NewPublicKey(make([]byte, 32))
	if err != nil {
		t.Fatalf("create X25519 public key: %v", err)
	}
	x25519DER, err := x509.MarshalPKIXPublicKey(x25519Key)
	if err != nil {
		t.Fatalf("marshal X25519 public key: %v", err)
	}

	tests := []struct {
		name  string
		der   []byte
		check func(*testing.T, error)
	}{
		{
			name: "malformed PKIX",
			der:  []byte{0x01, 0x02, 0x03},
			check: func(t *testing.T, err error) {
				if !errors.Is(err, ErrInvalidKey) {
					t.Fatalf("expected ErrInvalidKey, got: %v", err)
				}
				var parseErr asn1.StructuralError
				if !errors.As(err, &parseErr) {
					t.Fatalf("expected underlying PKIX structural parse error, got: %v", err)
				}
			},
		},
		{
			name: "unsupported X25519 key",
			der:  x25519DER,
			check: func(t *testing.T, err error) {
				if err == nil || !strings.Contains(err.Error(), "load public key verifier") || !strings.Contains(err.Error(), "unsupported public key type") {
					t.Fatalf("expected unsupported verifier key error, got: %v", err)
				}
				if errors.Is(err, ErrInvalidKey) || errors.Is(err, ErrInvalidBundle) {
					t.Fatalf("unsupported key was misclassified: %v", err)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: tt.der})
			tt.check(t, VerifySignedReport(report, bundleJSON, keyPEM))
		})
	}
}
