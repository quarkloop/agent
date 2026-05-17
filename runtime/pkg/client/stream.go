package rtclient

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// SSEEvent is one server-sent event read from a runtime streaming endpoint.
type SSEEvent struct {
	Type string
	Data json.RawMessage
}

func (t *Transport) StreamSSE(ctx context.Context, path string, fn func(string)) error {
	return t.StreamSSEEvents(ctx, path, func(event SSEEvent) error {
		fn(string(event.Data))
		return nil
	})
}

// StreamSSEEvents opens a GET event stream and passes each complete SSE event
// to fn in order.
func (t *Transport) StreamSSEEvents(ctx context.Context, path string, fn func(SSEEvent) error) error {
	fullURL := fmt.Sprintf("%s/%s", t.baseURL, strings.TrimLeft(path, "/"))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return fmt.Errorf("build stream request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Connection", "keep-alive")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("open stream: %w", err)
	}
	defer resp.Body.Close()

	if err := checkStatus(resp); err != nil {
		return err
	}

	return readSSE(ctx, resp.Body, fn)
}

// PostSSE opens a POST event stream with a JSON request body.
func (t *Transport) PostSSE(ctx context.Context, path string, in any, fn func(SSEEvent) error) error {
	fullURL := fmt.Sprintf("%s/%s", t.baseURL, strings.TrimLeft(path, "/"))

	var body io.Reader
	if in != nil {
		data, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("marshal stream request: %w", err)
		}
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, body)
	if err != nil {
		return fmt.Errorf("build stream request: %w", err)
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("open stream: %w", err)
	}
	defer resp.Body.Close()

	if err := checkStatus(resp); err != nil {
		return err
	}

	return readSSE(ctx, resp.Body, fn)
}

func readSSE(ctx context.Context, body io.Reader, fn func(SSEEvent) error) error {
	reader := bufio.NewReader(body)
	var data bytes.Buffer
	eventType := "message"

	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			line = bytes.TrimRight(line, "\r\n")
			if len(line) == 0 {
				if data.Len() > 0 {
					if err := fn(SSEEvent{Type: eventType, Data: append(json.RawMessage(nil), data.Bytes()...)}); err != nil {
						return err
					}
				}
				data.Reset()
				eventType = "message"
			} else if !bytes.HasPrefix(line, []byte(":")) {
				if rest, ok := bytes.CutPrefix(line, []byte("event:")); ok {
					eventType = strings.TrimSpace(string(rest))
				} else if rest, ok := bytes.CutPrefix(line, []byte("data:")); ok {
					if data.Len() > 0 {
						data.WriteByte('\n')
					}
					data.Write(bytes.TrimPrefix(rest, []byte(" ")))
				}
			}
		}
		if err != nil {
			if data.Len() > 0 {
				if fnErr := fn(SSEEvent{Type: eventType, Data: append(json.RawMessage(nil), data.Bytes()...)}); fnErr != nil {
					return fnErr
				}
			}
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				return nil
			}
			return fmt.Errorf("read stream: %w", err)
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
}
