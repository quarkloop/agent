package rtclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Transport struct {
	baseURL    string
	httpClient *http.Client
}

type TransportOption func(*Transport)

func NewTransport(baseURL string, opts ...TransportOption) *Transport {
	t := &Transport{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

func WithHTTPClient(httpClient *http.Client) TransportOption {
	return func(t *Transport) {
		if httpClient != nil {
			t.httpClient = httpClient
		}
	}
}

func WithTimeout(timeout time.Duration) TransportOption {
	return func(t *Transport) {
		if timeout > 0 {
			t.httpClient.Timeout = timeout
		}
	}
}

func (t *Transport) BaseURL() string {
	return t.baseURL
}

func (t *Transport) Get(ctx context.Context, path string, out any) error {
	return t.doRequest(ctx, http.MethodGet, path, nil, out)
}

func (t *Transport) Post(ctx context.Context, path string, in, out any) error {
	return t.doRequest(ctx, http.MethodPost, path, in, out)
}

func (t *Transport) Put(ctx context.Context, path string, in, out any) error {
	return t.doRequest(ctx, http.MethodPut, path, in, out)
}

func (t *Transport) Patch(ctx context.Context, path string, in, out any) error {
	return t.doRequest(ctx, http.MethodPatch, path, in, out)
}

func (t *Transport) Delete(ctx context.Context, path string, in, out any) error {
	return t.doRequest(ctx, http.MethodDelete, path, in, out)
}

func (t *Transport) doRequest(ctx context.Context, method, path string, in, out any) error {
	fullURL := fmt.Sprintf("%s/%s", t.baseURL, strings.TrimLeft(path, "/"))

	var body io.Reader
	if in != nil {
		data, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, body)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if err := checkStatus(resp); err != nil {
		return err
	}

	if out != nil && resp.StatusCode != http.StatusNoContent {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

func checkStatus(resp *http.Response) error {
	if resp.StatusCode < 400 {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	return &HTTPError{StatusCode: resp.StatusCode, Body: strings.TrimSpace(string(body))}
}

// HTTPError describes a non-2xx runtime API response.
type HTTPError struct {
	StatusCode int
	Body       string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("api error %d: %s", e.StatusCode, e.Body)
}

// IsNotFound reports whether err is a runtime API 404.
func IsNotFound(err error) bool {
	var httpErr *HTTPError
	return errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound
}
