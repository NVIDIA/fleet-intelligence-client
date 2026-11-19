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
)

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
