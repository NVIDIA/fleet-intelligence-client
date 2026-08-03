// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"bytes"
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"time"

	"github.com/sigstore/sigstore-go/pkg/bundle"
	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/verify"
	"github.com/sigstore/sigstore/pkg/signature"
)

// Errors returned by VerifySignedReport. They are sentinels so callers can
// distinguish the failure mode (e.g. with errors.Is) and present their own
// messages instead of the underlying sigstore/proto details.
var (
	// ErrInvalidKey means the supplied data is not a PEM-encoded public key.
	ErrInvalidKey = errors.New("invalid public key: expected a PEM-encoded public key")
	// ErrInvalidBundle means the supplied data is not a Sigstore .sig.bundle.
	ErrInvalidBundle = errors.New("invalid signature bundle: expected a Sigstore .sig.bundle file")
	// ErrVerificationFailed means the report does not match the signature.
	ErrVerificationFailed = errors.New("signature verification failed: the report does not match the signature")
)

// VerifySignedReport verifies that csv was signed with the public key in
// publicKeyPEM, using the Sigstore bundle in bundleJSON. It mirrors
// `cosign verify-blob --key <key> --bundle <bundle> --insecure-ignore-tlog <csv>`:
// the signature is checked against an out-of-band public key while the
// transparency log and observer timestamps are intentionally not required.
// It returns nil when verification succeeds and a descriptive error otherwise.
func VerifySignedReport(csv, bundleJSON, publicKeyPEM []byte) error {
	publicKey, err := parsePublicKeyPEM(publicKeyPEM)
	if err != nil {
		return err
	}

	verifier, err := signature.LoadVerifier(publicKey, crypto.SHA256)
	if err != nil {
		return fmt.Errorf("load public key verifier: %w", err)
	}

	// A zero validity window means the key is accepted at any time, so the
	// WithCurrentTime check below never rejects it. This matches cosign's
	// --insecure-ignore-tlog behavior for a long-lived signing key.
	key := root.NewExpiringKey(verifier, time.Time{}, time.Time{})
	trustedMaterial := root.NewTrustedPublicKeyMaterial(func(string) (root.TimeConstrainedVerifier, error) {
		return key, nil
	})

	var signedBundle bundle.Bundle
	if err := signedBundle.UnmarshalJSON(bundleJSON); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidBundle, err)
	}

	// WithCurrentTime: do not require a Timestamp Authority or transparency-log
	// timestamp. WithKey: the artifact was signed with a key, not a Fulcio
	// certificate identity.
	sev, err := verify.NewVerifier(trustedMaterial, verify.WithCurrentTime())
	if err != nil {
		return fmt.Errorf("build verifier: %w", err)
	}

	policy := verify.NewPolicy(verify.WithArtifact(bytes.NewReader(csv)), verify.WithKey())
	if _, err := sev.Verify(&signedBundle, policy); err != nil {
		return fmt.Errorf("%w: %w", ErrVerificationFailed, err)
	}
	return nil
}

// parsePublicKeyPEM decodes a PEM-encoded PKIX public key.
func parsePublicKeyPEM(data []byte) (crypto.PublicKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, ErrInvalidKey
	}

	publicKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidKey, err)
	}
	return publicKey, nil
}
