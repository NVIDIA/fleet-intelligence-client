package fleetintelligence

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultTimeout = 30 * time.Second

var (
	ErrMissingBaseURL    = errors.New("base URL is required")
	ErrMissingServiceKey = errors.New("service key is required")
)

type Client struct {
	baseURL    *url.URL
	serviceKey string
	httpClient *http.Client
}

type Option func(*Client)

func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		if httpClient != nil {
			c.httpClient = httpClient
		}
	}
}

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
		httpClient: &http.Client{Timeout: defaultTimeout},
	}

	for _, opt := range opts {
		opt(client)
	}

	return client, nil
}

func (c *Client) BaseURL() string {
	if c == nil || c.baseURL == nil {
		return ""
	}

	return c.baseURL.String()
}

func (c *Client) ServiceKeyConfigured() bool {
	return c != nil && strings.TrimSpace(c.serviceKey) != ""
}
