// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package output

import (
	"bytes"
	"errors"
	"strings"
)

// Returns an error for every write
type failingWriter struct{}

// Always fails
func (failingWriter) Write(_ []byte) (int, error) {
	return 0, errors.New("write failed")
}

type outputErrorWriter struct {
	accepted bytes.Buffer
	err      error
	failOn   string
	failed   bool
}

func (w *outputErrorWriter) Write(p []byte) (int, error) {
	if w.failOn == "" || strings.Contains(string(p), w.failOn) {
		w.failed = true
		return 0, w.err
	}
	return w.accepted.Write(p)
}

type firstLineWriter struct {
	accepted bytes.Buffer
	err      error
	failed   bool
}

func (w *firstLineWriter) Write(p []byte) (int, error) {
	if w.accepted.Len() > 0 {
		w.failed = true
		return 0, w.err
	}
	return w.accepted.Write(p)
}
