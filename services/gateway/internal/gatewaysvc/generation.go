package gatewaysvc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/quarkloop/pkg/plugin"
	gatewayv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/gateway/v1"
)

func (s *Server) generate(ctx context.Context, primary string, cmd generateCommand) (string, []toolCall, modelUsage, error) {
	inputTokens := estimateMessagesTokens(cmd.Messages, cmd.Tools)
	attempted := make([]string, 0)
	failures := make([]error, 0)
	chain, providers := s.providerChainSnapshot(primary)
	for _, providerID := range chain {
		attempted = append(attempted, providerID)
		p, ok := providers[providerID]
		if !ok {
			failures = append(failures, plugin.NewProviderError(plugin.ProviderErrorModelUnavailable, providerID, cmd.Model, 0, fmt.Errorf("provider unavailable")))
			continue
		}
		started := time.Now()
		stream, err := p.StreamGenerate(ctx, cmd)
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", providerID, err))
			if !canFallbackAfter(err) {
				return "", nil, modelUsage{}, err
			}
			continue
		}
		var text strings.Builder
		var calls []toolCall
		var providerUsage *modelUsage
		for event := range stream {
			if event.Err != nil {
				failures = append(failures, fmt.Errorf("%s: %w", providerID, event.Err))
				if canFallbackAfter(event.Err) {
					continue
				}
				return "", nil, modelUsage{}, event.Err
			}
			text.WriteString(event.Delta)
			calls = append(calls, event.ToolCalls...)
			if event.Usage != nil {
				usageCopy := *event.Usage
				providerUsage = &usageCopy
			}
		}
		body := text.String()
		if providerUsage != nil {
			if providerUsage.Provider == "" {
				providerUsage.Provider = providerID
			}
			if providerUsage.Model == "" {
				providerUsage.Model = firstNonEmpty(cmd.Model, defaultModel(p))
			}
			if len(providerUsage.FallbackChain) == 0 {
				providerUsage.FallbackChain = append([]string(nil), attempted...)
			}
			if providerUsage.RequestID == "" {
				providerUsage.RequestID = requestID("generate")
			}
			if providerUsage.FinishReason == "" {
				providerUsage.FinishReason = "stop"
			}
			return body, calls, *providerUsage, nil
		}
		return body, calls, modelUsage{
			Provider:      providerID,
			Model:         firstNonEmpty(cmd.Model, defaultModel(p)),
			InputTokens:   inputTokens,
			OutputTokens:  estimateTextTokens(body),
			LatencyMillis: elapsedMillis(started, time.Now()),
			FallbackChain: append([]string(nil), attempted...),
			RequestID:     requestID("generate"),
			FinishReason:  "stop",
		}, nil
	}
	return "", nil, modelUsage{}, plugin.NewProviderError(plugin.ProviderErrorExhausted, primary, cmd.Model, 0, errors.Join(failures...))
}

func (s *Server) usage(providerID, model string, started time.Time, cmd generateCommand, output string, chain []string, reason string) modelUsage {
	if len(chain) == 0 {
		chain = []string{providerID}
	}
	return modelUsage{
		Provider:      providerID,
		Model:         model,
		InputTokens:   estimateMessagesTokens(cmd.Messages, cmd.Tools),
		OutputTokens:  estimateTextTokens(output),
		LatencyMillis: elapsedMillis(started, time.Now()),
		FallbackChain: append([]string(nil), chain...),
		RequestID:     requestID("stream"),
		FinishReason:  reason,
	}
}

func defaultModel(p provider) string {
	models, err := p.ListModels(context.Background())
	if err != nil || len(models) == 0 {
		return ""
	}
	return models[0].GetId()
}

func embeddingUsageModel(cmd embedCommand, embeddings []*gatewayv1.Embedding) string {
	if len(embeddings) > 0 && strings.TrimSpace(embeddings[0].GetModel()) != "" {
		return embeddings[0].GetModel()
	}
	return strings.TrimSpace(cmd.Model)
}
