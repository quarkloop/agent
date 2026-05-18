//go:build e2e

package utils

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// PostMessage POSTs a user message to the agent's message SSE endpoint and
// returns the concatenated "text" payload received on the stream.
func PostMessage(t *testing.T, ctx context.Context, env *E2EEnv, sessionID, content string) string {
	t.Helper()
	return PostMessageTrace(t, ctx, env, sessionID, content).Text
}

// MessageTrace is the observable response stream produced by PostMessageTrace.
type MessageTrace struct {
	Text             string
	Space            string
	SessionID        string
	RunID            string
	ToolStarts       []string
	ToolResults      []string
	ToolStartEvents  []ToolEvent
	ToolResultEvents []ToolEvent
	LastEvent        string
	Completed        bool
}

type ToolEvent struct {
	CallID         string `json:"id,omitempty"`
	ServiceCallID  string `json:"service_call_id,omitempty"`
	Name           string `json:"name"`
	Arguments      string `json:"arguments,omitempty"`
	Result         string `json:"result,omitempty"`
	Error          bool   `json:"error,omitempty"`
	SessionID      string `json:"session_id,omitempty"`
	RunID          string `json:"run_id,omitempty"`
	ObservedAt     string `json:"observed_at,omitempty"`
	DurationMillis int64  `json:"duration_millis,omitempty"`
}

// MessageTraceOptions bounds one streamed agent response and controls failure
// diagnostics. OverallTimeout includes the HTTP request and stream read;
// IdleTimeout bounds silence after the last observed SSE line.
type MessageTraceOptions struct {
	Label          string
	Prompt         string
	Space          string
	SessionID      string
	OverallTimeout time.Duration
	IdleTimeout    time.Duration
}

// DefaultMessageTraceOptions returns conservative per-message guards. Tests
// with known longer prompts can override these values explicitly.
func DefaultMessageTraceOptions() MessageTraceOptions {
	return MessageTraceOptions{
		Label:          "agent message",
		OverallTimeout: 3 * time.Minute,
		IdleTimeout:    90 * time.Second,
	}
}

// PostMessageTrace POSTs a user message and returns streamed text plus tool
// progress events emitted by the runtime.
func PostMessageTrace(t *testing.T, ctx context.Context, env *E2EEnv, sessionID, content string) MessageTrace {
	t.Helper()
	return PostMessageTraceWithOptions(t, ctx, env, sessionID, content, DefaultMessageTraceOptions())
}

// PostMessageTraceWithOptions POSTs a user message with explicit stream guards.
func PostMessageTraceWithOptions(t *testing.T, ctx context.Context, env *E2EEnv, sessionID, content string, opts MessageTraceOptions) MessageTrace {
	t.Helper()

	opts = normalizeMessageTraceOptions(opts)
	opts.Prompt = content
	opts.Space = env.Space
	opts.SessionID = sessionID
	reqCtx := ctx
	var cancel context.CancelFunc
	if opts.OverallTimeout > 0 {
		reqCtx, cancel = context.WithTimeout(ctx, opts.OverallTimeout)
	} else {
		reqCtx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	body, err := json.Marshal(map[string]string{"content": content})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	url := fmt.Sprintf("%s/v1/sessions/%s/messages", env.AgentURL, sessionID)
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	httpc := &http.Client{} // no timeout — SSE holds the connection open
	resp, err := httpc.Do(req)
	if err != nil {
		t.Fatalf("post message %s: %v\n%s", opts.Label, err, messageTraceDiagnostics(MessageTrace{}, opts))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST %s: status=%d body=%s", url, resp.StatusCode, string(data))
	}

	trace, err := readMessageTrace(reqCtx, resp.Body, opts)
	if err != nil {
		var rl rateLimitError
		if AsRateLimitError(err, &rl) {
			t.Skipf("provider rate limited the e2e run: %s", rl.message)
		}
		t.Fatalf("read message stream %s: %v", opts.Label, err)
	}
	trace.Space = opts.Space
	trace.SessionID = opts.SessionID
	trace.RunID = traceRunID(trace)
	return trace
}

func traceRunID(trace MessageTrace) string {
	for _, event := range trace.ToolStartEvents {
		if event.RunID != "" {
			return event.RunID
		}
	}
	for _, event := range trace.ToolResultEvents {
		if event.RunID != "" {
			return event.RunID
		}
	}
	return ""
}

func normalizeMessageTraceOptions(opts MessageTraceOptions) MessageTraceOptions {
	defaults := DefaultMessageTraceOptions()
	if opts.Label == "" {
		opts.Label = defaults.Label
	}
	if opts.OverallTimeout == 0 {
		opts.OverallTimeout = defaults.OverallTimeout
	}
	if opts.IdleTimeout == 0 {
		opts.IdleTimeout = defaults.IdleTimeout
	}
	return opts
}

func readMessageTrace(ctx context.Context, stream io.Reader, opts MessageTraceOptions) (MessageTrace, error) {
	var trace MessageTrace
	var full strings.Builder
	reader := bufio.NewReader(stream)
	var dataBuf bytes.Buffer
	var currentEvent string

	lines := make(chan streamLine, 1)
	go func() {
		for {
			line, err := reader.ReadBytes('\n')
			select {
			case lines <- streamLine{line: line, err: err}:
			case <-ctx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()

	var idleTimer *time.Timer
	var idle <-chan time.Time
	if opts.IdleTimeout > 0 {
		idleTimer = time.NewTimer(opts.IdleTimeout)
		idle = idleTimer.C
		defer idleTimer.Stop()
	}
	resetIdle := func() {
		if idleTimer == nil {
			return
		}
		if !idleTimer.Stop() {
			select {
			case <-idleTimer.C:
			default:
			}
		}
		idleTimer.Reset(opts.IdleTimeout)
	}

	for {
		select {
		case item := <-lines:
			if len(item.line) > 0 {
				resetIdle()
				if err := consumeMessageTraceLine(&trace, &full, &dataBuf, &currentEvent, item.line); err != nil {
					return trace, fmt.Errorf("%w\n%s", err, messageTraceDiagnostics(trace, opts))
				}
			}
			if item.err != nil {
				if dataBuf.Len() > 0 {
					if err := consumeMessageTraceEvent(&trace, &full, currentEvent, dataBuf.Bytes()); err != nil {
						return trace, fmt.Errorf("%w\n%s", err, messageTraceDiagnostics(trace, opts))
					}
				}
				if item.err == io.EOF || item.err == io.ErrUnexpectedEOF {
					trace.Text = full.String()
					trace.Completed = true
					return trace, nil
				}
				return trace, fmt.Errorf("read stream: %w\n%s", item.err, messageTraceDiagnostics(trace, opts))
			}
		case <-idle:
			return trace, fmt.Errorf("message stream idle timeout after %s\n%s", opts.IdleTimeout, messageTraceDiagnostics(trace, opts))
		case <-ctx.Done():
			return trace, fmt.Errorf("message stream context ended: %w\n%s", ctx.Err(), messageTraceDiagnostics(trace, opts))
		}
	}
}

func toolEvent(data []byte) ToolEvent {
	var payload ToolEvent
	if err := json.Unmarshal(data, &payload); err != nil {
		return ToolEvent{}
	}
	return payload
}

type streamLine struct {
	line []byte
	err  error
}

func consumeMessageTraceLine(trace *MessageTrace, full *strings.Builder, dataBuf *bytes.Buffer, currentEvent *string, raw []byte) error {
	line := bytes.TrimRight(raw, "\r\n")

	if len(line) == 0 {
		if dataBuf.Len() > 0 {
			if err := consumeMessageTraceEvent(trace, full, *currentEvent, dataBuf.Bytes()); err != nil {
				return err
			}
		}
		dataBuf.Reset()
		*currentEvent = ""
		return nil
	}
	if bytes.HasPrefix(line, []byte(":")) {
		return nil
	}
	if rest, ok := bytes.CutPrefix(line, []byte("event:")); ok {
		*currentEvent = strings.TrimSpace(string(rest))
		trace.LastEvent = *currentEvent
		return nil
	}
	if rest, ok := bytes.CutPrefix(line, []byte("data:")); ok {
		if dataBuf.Len() > 0 {
			dataBuf.WriteByte('\n')
		}
		dataBuf.Write(bytes.TrimPrefix(rest, []byte(" ")))
	}
	return nil
}

func consumeMessageTraceEvent(trace *MessageTrace, full *strings.Builder, event string, data []byte) error {
	switch event {
	case "text", "token":
		var payload string
		if err := json.Unmarshal(data, &payload); err == nil {
			full.WriteString(payload)
			trace.Text = full.String()
		}
	case "tool_start":
		if event := toolEvent(data); event.Name != "" {
			trace.ToolStarts = append(trace.ToolStarts, event.Name)
			trace.ToolStartEvents = append(trace.ToolStartEvents, event)
		}
	case "tool_result":
		if event := toolEvent(data); event.Name != "" {
			trace.ToolResults = append(trace.ToolResults, event.Name)
			trace.ToolResultEvents = append(trace.ToolResultEvents, event)
		}
	case "error":
		message := string(data)
		if IsRateLimitText(message) {
			return rateLimitError{message: message}
		}
		return fmt.Errorf("agent returned error event: %s", message)
	}
	return nil
}

type rateLimitError struct {
	message string
}

func (e rateLimitError) Error() string {
	return "provider rate limited the e2e run: " + e.message
}

func AsRateLimitError(err error, target *rateLimitError) bool {
	return errors.As(err, target)
}

func messageTraceDiagnostics(trace MessageTrace, opts MessageTraceOptions) string {
	return fmt.Sprintf(
		"label=%s space=%s session=%s last_event=%s completed=%t tool_starts=%v tool_results=%v text_preview=%q prompt_preview=%q",
		opts.Label,
		opts.Space,
		opts.SessionID,
		trace.LastEvent,
		trace.Completed,
		trace.ToolStarts,
		trace.ToolResults,
		previewString(trace.Text, 500),
		previewString(opts.Prompt, 500),
	)
}

func previewString(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "...(truncated)"
}

// AgentSessionsCount reads GET /v1/info on the agent and returns the session
// count from the response.
func AgentSessionsCount(t *testing.T, env *E2EEnv) int {
	t.Helper()
	resp, err := env.HTTPC.Get(env.AgentURL + "/v1/info")
	if err != nil {
		t.Fatalf("get /v1/info: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET /v1/info: status=%d body=%s", resp.StatusCode, string(body))
	}
	var payload struct {
		Sessions int `json:"sessions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode /v1/info: %v", err)
	}
	return payload.Sessions
}

// WaitForAgentSession polls the agent's /v1/info until the session count
// reflects that a session has been mirrored from the supervisor.
func WaitForAgentSession(t *testing.T, env *E2EEnv, id string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	url := fmt.Sprintf("%s/v1/sessions/%s/messages", env.AgentURL, id)
	for time.Now().Before(deadline) {
		resp, err := env.HTTPC.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("agent never mirrored session %s", id)
}
