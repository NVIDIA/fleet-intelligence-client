// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/NVIDIA/fleet-intelligence-client/internal/generated/fleetapi"
)

// DefaultMaxResponseBytes is the ceiling applied to a single API response body
// when none is configured. The generated client buffers every response whole
// (io.ReadAll) before parsing, so an oversized or endless body would otherwise
// be bounded only by available memory. The value leaves generous headroom over
// the largest responses seen in practice (tens of MiB for a wide `alert
// describe` or a signed inventory report); raise it with WithMaxResponseBytes
// if a deployment legitimately returns more.
const DefaultMaxResponseBytes int64 = 64 << 20

// DefaultMaxJSONDepth is the nesting depth allowed in a JSON response body when
// none is configured. Real payloads from the Fleet Intelligence API nest a
// handful of levels; the limit exists to reject adversarially nested documents
// long before encoding/json's own 10000-level ceiling turns into wasted CPU.
const DefaultMaxJSONDepth = 64

// ErrResponseTooLarge reports that a response body exceeded the configured size
// limit and was not parsed. See WithMaxResponseBytes.
var ErrResponseTooLarge = errors.New("response body too large")

// ErrResponseTooDeep reports that a JSON response body nested more deeply than
// the configured limit and was not parsed. See WithMaxJSONDepth.
var ErrResponseTooDeep = errors.New("response JSON nested too deeply")

// WithMaxResponseBytes sets the maximum number of bytes read from a single API
// response body. Reading past the limit fails the call with
// ErrResponseTooLarge rather than buffering the remainder. Non-positive values
// are ignored, leaving the existing limit in place.
func WithMaxResponseBytes(maxBytes int64) Option {
	return func(c *Client) {
		if maxBytes > 0 {
			c.maxResponseBytes = maxBytes
		}
	}
}

// WithMaxJSONDepth sets the maximum object and array nesting allowed in a JSON
// response body. Exceeding it fails the call with ErrResponseTooDeep.
// Non-positive values are ignored, leaving the existing limit in place.
func WithMaxJSONDepth(maxDepth int) Option {
	return func(c *Client) {
		if maxDepth > 0 {
			c.maxJSONDepth = maxDepth
		}
	}
}

// Wraps a Doer so every response body it hands back is bounded before anything
// downstream buffers or parses it.
//
// The limits live here, at the transport seam, because the code that actually
// calls io.ReadAll is generated (internal/generated/fleetapi) and must not be
// hand-edited. Guarding the body instead of the parser also covers the SDK's
// own json.Unmarshal calls and the raw CSV and ZIP report payloads, which never
// reach a JSON decoder at all.
type limitingDoer struct {
	inner    fleetapi.HttpRequestDoer
	maxBytes int64
	maxDepth int
}

// Performs the request and returns it with a guarded body.
func (d *limitingDoer) Do(req *http.Request) (*http.Response, error) {
	response, err := d.inner.Do(req)
	if err != nil || response == nil || response.Body == nil {
		return response, err
	}

	body := newGuardedBody(response.Body, d.maxBytes, responseDepthLimit(response, d.maxDepth))
	body.rejectDeclaredLength(response.ContentLength)
	response.Body = body

	return response, nil
}

// Returns the depth limit to apply to this response, or 0 to skip the check.
//
// Depth is checked only for bodies the server labels as JSON — the same
// condition the generated parser uses to decide whether to unmarshal. Scanning
// a ZIP or CSV report payload for brace nesting would be meaningless and could
// reject a legitimate download on arbitrary binary content. A body that is
// unmarshaled despite a non-JSON content type is still covered by the byte
// limit and by encoding/json's own nesting ceiling.
func responseDepthLimit(response *http.Response, maxDepth int) int {
	if maxDepth <= 0 || !strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "json") {
		return 0
	}

	return maxDepth
}

// Enforces a byte ceiling, and optionally a JSON nesting ceiling, as a response
// body is read. Both are checked incrementally so an oversized body is rejected
// while it streams rather than after it has been buffered.
type guardedBody struct {
	inner     io.ReadCloser
	remaining int64
	limit     int64
	scanner   *jsonDepthScanner
	err       error
}

// Builds a guarded body
func newGuardedBody(inner io.ReadCloser, maxBytes int64, maxDepth int) *guardedBody {
	body := &guardedBody{inner: inner, remaining: maxBytes, limit: maxBytes}
	if maxDepth > 0 {
		body.scanner = &jsonDepthScanner{max: maxDepth}
	}

	return body
}

// Fails the body up front when the server declares a length above the limit, so
// an oversized payload is refused instead of streamed only to be rejected at
// the end. A chunked response declares -1 and is caught while reading instead;
// a declared length that understates the real body is caught the same way.
func (b *guardedBody) rejectDeclaredLength(declared int64) {
	if declared > b.limit {
		b.err = fmt.Errorf("%w: declared %d bytes, limit is %d", ErrResponseTooLarge, declared, b.limit)
	}
}

// Reads from the underlying body, failing once either limit is crossed. The
// error is sticky: a caller that ignores it and reads again gets the same
// failure instead of a partial, silently truncated document.
func (b *guardedBody) Read(p []byte) (int, error) {
	if b.err != nil {
		return 0, b.err
	}

	n, err := b.inner.Read(p)
	if n > 0 {
		if int64(n) > b.remaining {
			b.err = fmt.Errorf("%w: exceeds limit of %d bytes", ErrResponseTooLarge, b.limit)
			return 0, b.err
		}
		b.remaining -= int64(n)

		if b.scanner != nil {
			if scanErr := b.scanner.scan(p[:n]); scanErr != nil {
				b.err = scanErr
				return 0, b.err
			}
		}
	}
	if err != nil {
		b.err = err
	}

	return n, err
}

// Closes the underlying body
func (b *guardedBody) Close() error {
	return b.inner.Close()
}

// Tracks JSON object and array nesting across successive chunks of a streamed
// body. It deliberately does not validate the document: malformed input is the
// decoder's business, and this only has to bound how deep a well-formed one may
// go before the decoder recurses into it.
type jsonDepthScanner struct {
	max      int
	depth    int
	inString bool
	escaped  bool
}

// Scans one chunk, reporting ErrResponseTooDeep as soon as the limit is passed.
func (s *jsonDepthScanner) scan(chunk []byte) error {
	for _, c := range chunk {
		if s.inString {
			switch {
			case s.escaped:
				s.escaped = false
			case c == '\\':
				s.escaped = true
			case c == '"':
				s.inString = false
			}
			continue
		}

		switch c {
		case '"':
			s.inString = true
		case '{', '[':
			s.depth++
			if s.depth > s.max {
				return fmt.Errorf("%w: exceeds limit of %d levels", ErrResponseTooDeep, s.max)
			}
		case '}', ']':
			s.depth--
		}
	}

	return nil
}
