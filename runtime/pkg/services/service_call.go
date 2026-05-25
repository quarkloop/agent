package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/quarkloop/pkg/boundary"
	"github.com/quarkloop/pkg/natskit"
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

var serviceToolUnsafeChars = regexp.MustCompile(`[^A-Za-z0-9_]+`)

func FunctionNameFor(serviceName string, rpc *servicev1.RpcDescriptor) string {
	if rpc != nil && strings.TrimSpace(rpc.GetFunctionName()) != "" {
		return strings.TrimSpace(rpc.GetFunctionName())
	}
	if rpc == nil {
		return ToolNameFor(serviceName, "")
	}
	return ToolNameFor(serviceName, rpc.GetMethod())
}

func ToolNameFor(serviceName, method string) string {
	serviceName = strings.TrimSpace(serviceName)
	method = strings.TrimSpace(method)
	if serviceName == "" && method == "" {
		return "service_call"
	}
	if serviceName == "" {
		serviceName = "service"
	}
	name := serviceToolUnsafeChars.ReplaceAllString(serviceName+"_"+method, "_")
	name = strings.Trim(name, "_")
	if name == "" {
		return "service_call"
	}
	return name
}

func serviceFunctionContext(ctx context.Context, rpc *servicev1.RpcDescriptor) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	timeout := time.Duration(0)
	if rpc != nil && rpc.GetTimeoutMillis() > 0 {
		timeout = time.Duration(rpc.GetTimeoutMillis()) * time.Millisecond
	}
	if timeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, timeout)
}

func (e *Executor) invokeNATSServiceFunction(ctx context.Context, resolved resolvedRPC, payload json.RawMessage, response protoreflect.MessageDescriptor) (*dynamicpb.Message, natskit.ResponseEnvelope, error) {
	if e == nil || e.caller == nil {
		return nil, natskit.ResponseEnvelope{}, boundary.New(boundary.Runtime, boundary.Unavailable, "service function", "NATS service function caller is not configured")
	}
	operation, err := serviceFunctionOperation(resolved)
	if err != nil {
		return nil, natskit.ResponseEnvelope{}, boundary.Wrap(boundary.Service, boundary.InvalidArgument, "service function operation", err)
	}
	attempts := serviceFunctionMaxAttempts(resolved.rpc)
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		envelope, err := e.caller.Call(ctx, serviceFunctionCall{
			Operation: operation,
			Payload:   payload,
			RPC:       resolved.rpc,
		})
		if err == nil && envelope.Status == natskit.StatusOK {
			out := dynamicpb.NewMessage(response)
			if len(envelope.Payload) > 0 {
				if err := protojson.Unmarshal(envelope.Payload, out); err != nil {
					return nil, envelope, boundary.Wrap(boundary.Service, boundary.InvalidArgument, operation.Subject, err)
				}
			}
			return out, envelope, nil
		}
		if err == nil && envelope.Error != nil {
			err = boundary.New(envelope.Error.Boundary, envelope.Error.Category, envelope.Error.Operation, envelope.Error.Message)
		}
		if err == nil {
			err = boundary.New(boundary.Service, boundary.Unknown, operation.Subject, "service function returned non-ok response without an error payload")
		}
		lastErr = err
		if attempt == attempts || !serviceFunctionRetryable(resolved.rpc, err) {
			return nil, envelope, boundary.FromError(boundary.Service, operation.Subject, err)
		}
		if err := waitServiceFunctionRetry(ctx, resolved.rpc, attempt); err != nil {
			return nil, envelope, err
		}
	}
	return nil, natskit.ResponseEnvelope{}, lastErr
}

func serviceFunctionOperation(resolved resolvedRPC) (natskit.Operation, error) {
	rpc := resolved.rpc
	if rpc == nil {
		return natskit.Operation{}, fmt.Errorf("rpc descriptor is required")
	}
	subject := strings.TrimSpace(rpc.GetSubject())
	if subject == "" {
		return natskit.Operation{}, fmt.Errorf("service function subject is required for %s/%s", rpc.GetService(), rpc.GetMethod())
	}
	return natskit.ParseServiceOperation(subject)
}

func serviceFunctionMaxAttempts(rpc *servicev1.RpcDescriptor) int {
	if rpc == nil || rpc.GetRetryPolicy() == nil || rpc.GetRetryPolicy().GetMaxAttempts() <= 0 {
		return 1
	}
	return int(rpc.GetRetryPolicy().GetMaxAttempts())
}

func serviceFunctionRetryable(rpc *servicev1.RpcDescriptor, err error) bool {
	if rpc == nil || rpc.GetRetryPolicy() == nil {
		return false
	}
	code := normalizeRetryCode(serviceFunctionErrorCode(err))
	for _, retryable := range rpc.GetRetryPolicy().GetRetryableCodes() {
		if normalizeRetryCode(retryable) == code {
			return true
		}
	}
	return false
}

func serviceFunctionErrorCode(err error) string {
	var boundaryErr *boundary.Error
	if errors.As(err, &boundaryErr) {
		return string(boundaryErr.Category)
	}
	return string(boundary.Unknown)
}

func normalizeRetryCode(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "_", "")
	value = strings.ReplaceAll(value, "-", "")
	value = strings.ReplaceAll(value, " ", "")
	return strings.ToLower(value)
}

func waitServiceFunctionRetry(ctx context.Context, rpc *servicev1.RpcDescriptor, attempt int) error {
	backoff := serviceFunctionBackoff(rpc, attempt)
	if backoff <= 0 {
		return nil
	}
	timer := time.NewTimer(backoff)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func serviceFunctionBackoff(rpc *servicev1.RpcDescriptor, attempt int) time.Duration {
	if rpc == nil || rpc.GetRetryPolicy() == nil {
		return 0
	}
	initial := time.Duration(rpc.GetRetryPolicy().GetInitialBackoffMillis()) * time.Millisecond
	if initial <= 0 {
		return 0
	}
	backoff := initial
	for i := 1; i < attempt; i++ {
		backoff *= 2
	}
	maxBackoff := time.Duration(rpc.GetRetryPolicy().GetMaxBackoffMillis()) * time.Millisecond
	if maxBackoff > 0 && backoff > maxBackoff {
		return maxBackoff
	}
	return backoff
}
