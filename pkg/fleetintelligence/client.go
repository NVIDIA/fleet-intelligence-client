package fleetintelligence

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gitlab-master.nvidia.com/gpu-health/fleet-intelligence-client-go/internal/generated/fleetapi"
)

// DefaultTimeout is the per-request timeout applied when none is configured.
const DefaultTimeout = 2 * time.Minute

var (
	ErrMissingBaseURL    = errors.New("base URL is required")
	ErrMissingServiceKey = errors.New("service key is required")
)

// Calls the Fleet Intelligence customer API
type Client struct {
	baseURL    *url.URL
	serviceKey string
	httpClient *http.Client
	timeout    time.Duration
	api        *fleetapi.ClientWithResponses
}

// Customizes client construction behavior
type Option func(*Client)

// Configures the HTTP client used for API requests
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		if httpClient != nil {
			c.httpClient = httpClient
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
func NewClient(baseURL, serviceKey string, opts ...Option) (*Client, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return nil, ErrMissingBaseURL
	}

	parsedBaseURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}

	serviceKey = strings.TrimSpace(serviceKey)
	if serviceKey == "" {
		return nil, ErrMissingServiceKey
	}

	client := &Client{
		baseURL:    parsedBaseURL,
		serviceKey: serviceKey,
		httpClient: &http.Client{},
		timeout:    DefaultTimeout,
	}

	for _, opt := range opts {
		opt(client)
	}

	api, err := fleetapi.NewClientWithResponses(
		client.baseURL.String(),
		fleetapi.WithHTTPClient(&timeoutDoer{inner: client.httpClient, timeout: client.timeout}),
		fleetapi.WithRequestEditorFn(client.authorizeRequest),
	)
	if err != nil {
		return nil, err
	}
	client.api = api

	return client, nil
}

// Wraps the HTTP Doer to translate the configured timeout firing into a
// concise error instead of the transport-level "context deadline exceeded"
// message, which also leaks the full request URL.
type timeoutDoer struct {
	inner   fleetapi.HttpRequestDoer
	timeout time.Duration
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
	req.Header.Set("Authorization", "Bearer "+c.serviceKey)
	req.Header.Set("Accept", "application/json")
	return nil
}

// Returns the configured API base URL
func (c *Client) BaseURL() string {
	if c == nil || c.baseURL == nil {
		return ""
	}

	return c.baseURL.String()
}

// Reports whether a non-empty service key is configured
func (c *Client) ServiceKeyConfigured() bool {
	return c != nil && strings.TrimSpace(c.serviceKey) != ""
}
