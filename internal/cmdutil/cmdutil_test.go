// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package cmdutil

import (
	"bytes"
	"strings"
)

type optionsFailingWriter struct {
	out         bytes.Buffer
	marker      string
	err         error
	encountered bool
}

func (w *optionsFailingWriter) Write(p []byte) (int, error) {
	if strings.Contains(w.out.String()+string(p), w.marker) {
		w.encountered = true
		return 0, w.err
	}
	return w.out.Write(p)
}

type promptErrorReader struct {
	input string
	err   error
}

func (r *promptErrorReader) Read(p []byte) (int, error) {
	n := copy(p, r.input)
	r.input = r.input[n:]
	return n, r.err
}

type errorFirstWriteFails struct {
	out      bytes.Buffer
	attempts int
	err      error
}

func (w *errorFirstWriteFails) Write(p []byte) (int, error) {
	w.attempts++
	if w.attempts == 1 {
		return 0, w.err
	}
	return w.out.Write(p)
}

type sourceTableFailingWriter struct {
	out      bytes.Buffer
	attempts int
	failAt   int
	err      error
}

func (w *sourceTableFailingWriter) Write(p []byte) (int, error) {
	w.attempts++
	if w.attempts == w.failAt {
		return 0, w.err
	}
	return w.out.Write(p)
}
