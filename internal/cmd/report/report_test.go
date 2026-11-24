// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sigstore/sigstore-go/pkg/bundle"
	"github.com/sigstore/sigstore-go/pkg/sign"
	"github.com/spf13/cobra"
)

// newRootCmd builds a root command carrying only this package's commands, so
// the tests drive them through the same argument path a user types.
func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "nvfleetint",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(NewCmd())
	return root
}

// Changes the working directory for the duration of a test
func chdir(t *testing.T, dir string) {
	t.Helper()
	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(original); err != nil {
			t.Fatalf("restore chdir failed: %v", err)
		}
	})
}

// writeSignedFixture signs csv and writes the CSV, bundle, and public key into
// dir, returning their paths.
func writeSignedFixture(t *testing.T, dir string, csv []byte) (csvPath, bundlePath, keyPath string) {
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
	bundleJSON, err := signed.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal bundle failed: %v", err)
	}

	csvPath = filepath.Join(dir, "inventory.csv")
	bundlePath = filepath.Join(dir, "inventory.sig.bundle")
	keyPath = filepath.Join(dir, "signing-key.pub")
	for path, data := range map[string][]byte{csvPath: csv, bundlePath: bundleJSON, keyPath: []byte(pem)} {
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatalf("write fixture %s failed: %v", path, err)
		}
	}
	return csvPath, bundlePath, keyPath
}

type reportFailingWriter struct {
	err   error
	calls int
}

func (w *reportFailingWriter) Write(_ []byte) (int, error) {
	w.calls++
	return 0, w.err
}
