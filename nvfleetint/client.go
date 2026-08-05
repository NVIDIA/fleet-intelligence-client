// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/NVIDIA/fleet-intelligence-client/internal/generated/fleetapi"
)

// DefaultTimeout is the per-request timeout applied when none is configured.
const DefaultTimeout = 2 * time.Minute

const (
	defaultRequestAttempts = 3
	initialRetryDelay      = 200 * time.Millisecond
	maximumRetryDelay      = 5 * time.Second
)

// signingKeyPath is the well-known location of the report signing public key.
const signingKeyPath = "/.well-known/signing-key.pub"

// signingKeyAcceptHeader requests the PEM key while still allowing raw file
// responses from deployments that use a generic content type.
const signingKeyAcceptHeader = "application/x-pem-file, text/plain, */*"

var (
	ErrMissingBaseURL = errors.New("base URL is required")
	ErrMissingAPIKey  = errors.New("API key is required")
)

// Calls the Fleet Intelligence customer API
type Client struct {
	baseURL     *url.URL
	apiKey      string
	httpClient  *http.Client
	requestDoer fleetapi.HttpRequestDoer
	timeout     time.Duration
	api         *fleetapi.ClientWithResponses
}

// Customizes client construction behavior
type Option func(*Client)

// Configures the HTTP client used for API requests. The client is used through
// a shallow copy carrying the redirect guard (see guardRedirect), so a client
// shared across callers is never mutated and its transport, cookie jar, and
// timeout are preserved.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		if httpClient != nil {
			c.httpClient = withRedirectGuard(httpClient)
		}
	}
}

// Sets the per-request timeout applied to API calls. The timeout is enforced
// via the request context rather than the HTTP client, so it is unaffected by
// WithHTTPClient (which replaces the HTTP client) and never mutates an
// *http.Client that may be shared across clients. Non-positive values are
// ignored, leaving the existing timeout in place.
func WithTimeout(timeout time.Duration) Option {
	return func(c *Client) {
		if timeout > 0 {
			c.timeout = timeout
		}
	}
}

// Creates a Fleet Intelligence API client
func NewClient(baseURL, apiKey string, opts ...Option) (*Client, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return nil, ErrMissingBaseURL
	}

	// Reject plaintext endpoints before any request is built: every call
	// carries the API key in an Authorization header.
	if err := ValidateBaseURL(baseURL); err != nil {
		return nil, err
	}

	parsedBaseURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}

	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil, ErrMissingAPIKey
	}

	client := &Client{
		baseURL:    parsedBaseURL,
		apiKey:     apiKey,
		httpClient: defaultHTTPClient(),
		timeout:    DefaultTimeout,
	}

	for _, opt := range opts {
		opt(client)
	}
	client.requestDoer = &retryingDoer{
		inner:       client.httpClient,
		maxAttempts: defaultRequestAttempts,
	}

	api, err := fleetapi.NewClientWithResponses(
		client.baseURL.String(),
		fleetapi.WithHTTPClient(&timeoutDoer{
			inner:   client.requestDoer,
			timeout: client.timeout,
		}),
		fleetapi.WithRequestEditorFn(client.authorizeRequest),
	)
	if err != nil {
		return nil, err
	}
	client.api = api

	return client, nil
}

// The hardened transport is built once and shared by every client that does
// not supply its own. Cloning per client would give each one a private
// connection pool and TLS session cache, so an embedder constructing a client
// per request would pay a fresh handshake every time and leak idle sockets.
var (
	sharedTransportOnce sync.Once
	sharedTransport     *http.Transport
)

// Builds the HTTP client used when the caller supplies none. It reuses one
// hardened clone of the standard transport, so connection pooling behaves as it
// would with http.DefaultTransport. This is a safe default rather than a
// guarantee: WithHTTPClient lets a caller substitute their own transport.
func defaultHTTPClient() *http.Client {
	sharedTransportOnce.Do(func() {
		sharedTransport = hardenedTransport(http.DefaultTransport)
	})
	if sharedTransport == nil {
		return withRedirectGuard(&http.Client{})
	}

	return withRedirectGuard(&http.Client{Transport: sharedTransport})
}

// maxRedirects mirrors net/http's own default. Installing a CheckRedirect
// replaces that default, so the cap has to be restated here.
const maxRedirects = 10

// Returns a shallow copy of client with the redirect guard installed, leaving
// the caller's client untouched — it may be shared across clients. A
// CheckRedirect the caller already set still runs, after the guard, so their
// own policy (including a different redirect cap) is preserved.
func withRedirectGuard(client *http.Client) *http.Client {
	guarded := *client
	inner := client.CheckRedirect
	guarded.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if err := guardRedirect(req, via); err != nil {
			return err
		}
		if inner != nil {
			return inner(req, via)
		}
		if len(via) >= maxRedirects {
			return fmt.Errorf("stopped after %d redirects", maxRedirects)
		}
		return nil
	}

	return &guarded
}

// Refuses to follow a redirect that downgrades https to plaintext, and drops
// the Authorization header before any cross-origin hop.
//
// net/http already withholds Authorization when a redirect leaves the original
// domain, but that check compares hosts only: it ignores the scheme and port,
// so an https://api -> http://api redirect would hand the bearer token to a
// cleartext connection, and it forwards the token to subdomains and to other
// ports on the same host. Same-origin https redirects are unaffected.
func guardRedirect(req *http.Request, via []*http.Request) error {
	if len(via) == 0 {
		return nil
	}

	previous := via[len(via)-1].URL
	if strings.EqualFold(previous.Scheme, "https") && !strings.EqualFold(req.URL.Scheme, "https") {
		// Report the scheme and host only; the path and query may carry
		// identifiers that do not belong in an error string.
		return fmt.Errorf(
			"refusing redirect from https to %s://%s: %w",
			req.URL.Scheme, req.URL.Host, ErrInsecureBaseURL,
		)
	}
	if !sameOrigin(via[0].URL, req.URL) {
		req.Header.Del("Authorization")
	}

	return nil
}

// Reports whether two URLs share a scheme, host, and port
func sameOrigin(a, b *url.URL) bool {
	return strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(originHostPort(a), originHostPort(b))
}

// Renders the host and port of u, filling in the scheme's default port so that
// https://host and https://host:443 compare equal.
func originHostPort(u *url.URL) string {
	port := u.Port()
	if port == "" {
		switch strings.ToLower(u.Scheme) {
		case "https":
			port = "443"
		case "http":
			port = "80"
		}
	}

	return net.JoinHostPort(u.Hostname(), port)
}

// Clones base (preserving proxy, keep-alive, and HTTP/2 defaults) and pins at
// least a TLS 1.2 floor, since Go's default has none. An existing stricter
// floor is left alone. Returns nil when base is not an *http.Transport, leaving
// the caller to fall back to net/http's own default.
func hardenedTransport(base http.RoundTripper) *http.Transport {
	transport, ok := base.(*http.Transport)
	if !ok {
		return nil
	}

	cloned := transport.Clone()
	if cloned.TLSClientConfig == nil {
		cloned.TLSClientConfig = &tls.Config{}
	}
	if cloned.TLSClientConfig.MinVersion < tls.VersionTLS12 {
		cloned.TLSClientConfig.MinVersion = tls.VersionTLS12
	}

	return cloned
}

// Wraps the HTTP Doer to translate the configured timeout firing into a
// concise error instead of the transport-level "context deadline exceeded"
// message, which also leaks the full request URL.
type timeoutDoer struct {
	inner   fleetapi.HttpRequestDoer
	timeout time.Duration
}

// Retries idempotent reads when a transient transport or backend failure makes
// an individual request fail. Paginated callers therefore retry only the page
// that failed; successfully returned pages are not requested again.
type retryingDoer struct {
	inner       fleetapi.HttpRequestDoer
	maxAttempts int
	delay       func(attempt int, response *http.Response) time.Duration
	wait        func(context.Context, time.Duration) error
}

// Performs one request, retrying bounded transient failures within the request
// context's existing timeout.
func (d *retryingDoer) Do(req *http.Request) (*http.Response, error) {
	maxAttempts := d.maxAttempts
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		response, err := d.inner.Do(req)
		if !canRetryRequest(req, response, err) || attempt == maxAttempts {
			return response, err
		}

		if response != nil && response.Body != nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
			_ = response.Body.Close()
		}

		delay := defaultRetryDelay(attempt, response)
		if d.delay != nil {
			delay = d.delay(attempt, response)
		}
		wait := waitForRetry
		if d.wait != nil {
			wait = d.wait
		}
		if err := wait(req.Context(), delay); err != nil {
			return nil, err
		}
	}

	return nil, errors.New("retry attempts exhausted")
}

// Reports whether retrying this request is safe and potentially useful.
func canRetryRequest(req *http.Request, response *http.Response, err error) bool {
	if req == nil || (req.Method != http.MethodGet && req.Method != http.MethodHead) {
		return false
	}
	// The generated read requests have no body. Avoid replaying an unusual GET
	// body because net/http may already have consumed or closed it.
	if req.Body != nil {
		return false
	}
	if req.Context().Err() != nil {
		return false
	}
	if err != nil {
		return isRetryableNetworkError(err)
	}
	if response == nil {
		return false
	}

	switch response.StatusCode {
	case http.StatusRequestTimeout,
		http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

// Identifies transport failures that commonly succeed when retried.
func isRetryableNetworkError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	return errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.EPIPE)
}

// Computes Retry-After or an exponential delay with bounded jitter.
func defaultRetryDelay(attempt int, response *http.Response) time.Duration {
	if delay, ok := responseRetryAfter(response, time.Now()); ok {
		return delay
	}

	delay := initialRetryDelay << (attempt - 1)
	if delay > maximumRetryDelay {
		delay = maximumRetryDelay
	}
	// Randomize to 50%-150% so many clients do not retry in lockstep.
	half := delay / 2
	return half + time.Duration(rand.Int64N(int64(delay)+1))
}

// Parses Retry-After as either seconds or an HTTP date.
func responseRetryAfter(response *http.Response, now time.Time) (time.Duration, bool) {
	if response == nil {
		return 0, false
	}
	raw := strings.TrimSpace(response.Header.Get("Retry-After"))
	if raw == "" {
		return 0, false
	}
	if seconds, err := strconv.Atoi(raw); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second, true
	}
	retryAt, err := http.ParseTime(raw)
	if err != nil {
		return 0, false
	}
	if !retryAt.After(now) {
		return 0, true
	}
	return retryAt.Sub(now), true
}

// Waits for a retry delay without ignoring request cancellation.
func waitForRetry(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Performs the request and rewrites timeout errors into a friendly message
func (d *timeoutDoer) Do(req *http.Request) (*http.Response, error) {
	resp, err := d.inner.Do(req)
	if err != nil && errors.Is(err, context.DeadlineExceeded) {
		return nil, fmt.Errorf("request timed out after %s", d.timeout)
	}
	return resp, err
}

// Derives a request context that enforces the configured timeout. Applying it
// here keeps the timeout independent of httpClient, which WithHTTPClient may
// replace and which callers may share across clients. The returned cancel
// func must always be called to release context resources.
func (c *Client) requestContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if c.timeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, c.timeout)
}

// Attaches authentication and response format headers
func (c *Client) authorizeRequest(_ context.Context, req *http.Request) error {
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	return nil
}

// FetchSigningKey downloads the PEM-encoded public key used to sign inventory
// reports from the configured API's well-known endpoint. It is used by
// `report verify` when no local key is supplied.
func (c *Client) FetchSigningKey(ctx context.Context) ([]byte, error) {
	ref := c.baseURL.ResolveReference(&url.URL{Path: signingKeyPath})

	ctx, cancel := c.requestContext(ctx)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ref.String(), nil)
	if err != nil {
		return nil, err
	}
	if err := c.authorizeRequest(ctx, req); err != nil {
		return nil, err
	}
	req.Header.Set("Accept", signingKeyAcceptHeader)

	resp, err := c.requestDoer.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("request timed out after %s", c.timeout)
		}
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch signing key: unexpected status %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read signing key: %w", err)
	}
	return body, nil
}

// Returns the configured API base URL
func (c *Client) BaseURL() string {
	if c == nil || c.baseURL == nil {
		return ""
	}

	return c.baseURL.String()
}

// Reports whether a non-empty API key is configured
func (c *Client) APIKeyConfigured() bool {
	return c != nil && strings.TrimSpace(c.apiKey) != ""
}
