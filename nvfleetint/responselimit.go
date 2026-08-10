// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/NVIDIA/fleet-intelligence-client/internal/generated/fleetapi"
)

// maxResponseBytes caps how much of a response body the SDK buffers. Every
// generated parser reads its response with an unbounded io.ReadAll, so without
// this ceiling a hostile or malfunctioning endpoint could stream until the
// process is out of memory. The cap sits far above any real payload — the
// largest observed response, an `alert describe` carrying per-GPU detail, is
// roughly 33 MB — so it only trips on data the CLI could not have rendered
// anyway.
const maxResponseBytes int64 = 256 << 20

// maxSigningKeyBytes caps the report signing key download separately. The
// document is a PEM public key of a few kilobytes, so the general response
// ceiling would be pointlessly generous for it.
const maxSigningKeyBytes int64 = 1 << 20

// ErrResponseTooLarge reports that an API response outgrew the size the SDK is
// willing to buffer and was refused before being parsed.
var ErrResponseTooLarge = errors.New("API response exceeded the maximum allowed size")

// Bounds the body of every response before it reaches a parser. It wraps the
// innermost Doer, which places the limit beneath retryingDoer: each attempt is
// bounded on its own, and FetchSigningKey — which uses requestDoer directly,
// bypassing the generated client — is covered by the same wrapper.
type limitingDoer struct {
	inner    fleetapi.HttpRequestDoer
	maxBytes int64
}

// Performs the request and hands back the response with a bounded body
func (d *limitingDoer) Do(req *http.Request) (*http.Response, error) {
	resp, err := d.inner.Do(req)
	if err != nil || resp == nil || resp.Body == nil {
		return resp, err
	}
	resp.Body = newLimitedBody(resp.Body, d.maxBytes)

	return resp, nil
}

// Reads at most limit bytes from the underlying body and then fails every
// subsequent read with ErrResponseTooLarge. Close closes the underlying body,
// so the response still behaves like any other http.Response.
type limitedBody struct {
	inner io.ReadCloser
	limit int64
	// remaining is the byte budget left. It goes negative once the body runs
	// past the cap, which latches the failure for all later reads.
	remaining int64
}

// Wraps body so that no more than maxBytes can be read out of it
func newLimitedBody(body io.ReadCloser, maxBytes int64) io.ReadCloser {
	return &limitedBody{inner: body, limit: maxBytes, remaining: maxBytes}
}

// Reads the next chunk unless the body has already overrun its budget
func (b *limitedBody) Read(p []byte) (int, error) {
	if b.remaining < 0 {
		return 0, b.tooLargeError()
	}
	// Allow reading one byte past the budget: consuming that byte is what
	// separates a body ending exactly at the cap from one running past it.
	if int64(len(p)) > b.remaining+1 {
		p = p[:b.remaining+1]
	}

	n, err := b.inner.Read(p)
	b.remaining -= int64(n)
	if b.remaining < 0 {
		return n, b.tooLargeError()
	}

	return n, err
}

// Closes the underlying body
func (b *limitedBody) Close() error {
	return b.inner.Close()
}

// Names the limit that was exceeded, since the signing key fetch uses a
// tighter cap than the general response ceiling.
func (b *limitedBody) tooLargeError() error {
	return fmt.Errorf("%w of %d bytes", ErrResponseTooLarge, b.limit)
}
