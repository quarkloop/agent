// Package client is the Go SDK for the Quark supervisor HTTP API.
//
// The SDK is split by resource concern:
//   - spaces.go   — space CRUD and latest Quarkfile state
//   - kb.go       — knowledge base operations
//   - plugins.go  — plugin install/uninstall/search
//   - agents.go   — agent runtime start/stop/lookup
//
// All methods are safe for concurrent use.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/quarkloop/pkg/boundary"
	"github.com/quarkloop/supervisor/pkg/api"
)

// DefaultBaseURL is the default supervisor HTTP URL.
const DefaultBaseURL = "http://127.0.0.1:7200"

// Client is the supervisor HTTP client.
type Client struct {
	baseURL string
	http    *http.Client
	route   api.RouteBuilder
}

// Option customises a Client.
type Option func(*Client)

// WithHTTPClient overrides the underlying HTTP client.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) {
		if h != nil {
			c.http = h
		}
	}
}

// WithBaseURL overrides the base URL.
func WithBaseURL(url string) Option {
	return func(c *Client) {
		if url != "" {
			c.baseURL = url
		}
	}
}

// New constructs a Client. The base URL is resolved from (in order):
//  1. the WithBaseURL option
//  2. the QUARK_SUPERVISOR_URL environment variable
//  3. DefaultBaseURL
func New(opts ...Option) *Client {
	c := &Client{
		baseURL: defaultBaseURL(),
		http:    &http.Client{Timeout: 60 * time.Second},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func defaultBaseURL() string {
	if v := os.Getenv("QUARK_SUPERVISOR_URL"); v != "" {
		return v
	}
	return DefaultBaseURL
}

// BaseURL returns the configured supervisor base URL.
func (c *Client) BaseURL() string { return c.baseURL }

// do executes an HTTP request against the supervisor. body and out are
// JSON-marshaled / unmarshaled. Pass nil for either to skip.
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return boundary.Wrap(boundary.Supervisor, boundary.Transport, method+" "+path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(resp.Body)
		return &HTTPError{
			Method:     method,
			Path:       path,
			StatusCode: resp.StatusCode,
			Category:   boundary.FromHTTPStatus(boundary.Supervisor, method+" "+path, resp.StatusCode, string(data)).Category,
			Body:       string(data),
		}
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// HTTPError is returned by Client methods when the supervisor responds
// with a non-2xx status code.
type HTTPError struct {
	Method     string
	Path       string
	StatusCode int
	Category   boundary.Category
	Body       string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("supervisor %s %s returned %d: %s", e.Method, e.Path, e.StatusCode, e.Body)
}

func (e *HTTPError) Unwrap() error {
	return boundary.FromHTTPStatus(boundary.Supervisor, e.Method+" "+e.Path, e.StatusCode, e.Body)
}

// IsNotFound reports whether err is a 404.
func IsNotFound(err error) bool {
	if boundary.IsCategory(err, boundary.NotFound) {
		return true
	}
	var he *HTTPError
	if !errors.As(err, &he) {
		return false
	}
	return he.StatusCode == http.StatusNotFound
}

// IsConflict reports whether err is a 409.
func IsConflict(err error) bool {
	if boundary.IsCategory(err, boundary.Conflict) {
		return true
	}
	var he *HTTPError
	if !errors.As(err, &he) {
		return false
	}
	return he.StatusCode == http.StatusConflict
}
