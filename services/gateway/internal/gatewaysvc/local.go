package gatewaysvc

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"strings"

	gatewayv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/gateway/v1"
)

type localProvider struct {
	id    string
	model string
}

func newLocalProvider(cfg ProviderConfig) provider {
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = "local/noop"
	}
	id := strings.TrimSpace(cfg.ID)
	if id == "" {
		id = "local"
	}
	return &localProvider{id: id, model: model}
}

func (p *localProvider) ID() string { return p.id }

func (p *localProvider) ListModels(context.Context) ([]*gatewayv1.ModelInfo, error) {
	return []*gatewayv1.ModelInfo{{
		Id:            p.model,
		Provider:      p.id,
		Name:          p.model,
		ContextWindow: 32768,
		DefaultModel:  true,
	}}, nil
}

func (p *localProvider) StreamGenerate(ctx context.Context, cmd generateCommand) (<-chan streamEvent, error) {
	out := make(chan streamEvent, 4)
	go func() {
		defer close(out)
		text := "Local deterministic model response."
		if last := lastUserMessage(cmd.Messages); last != "" {
			text = fmt.Sprintf("Local deterministic model response for: %s", truncate(last, 160))
		}
		select {
		case <-ctx.Done():
			out <- streamEvent{Err: ctx.Err()}
		case out <- streamEvent{Delta: text}:
		}
		out <- streamEvent{Done: true}
	}()
	return out, nil
}

func (p *localProvider) Embed(_ context.Context, cmd embedCommand) ([]*gatewayv1.Embedding, error) {
	dimensions := int(cmd.Dimensions)
	if dimensions <= 0 {
		dimensions = 32
	}
	model := cmd.Model
	if model == "" {
		model = p.model
	}
	out := make([]*gatewayv1.Embedding, 0, len(cmd.Input))
	for _, input := range cmd.Input {
		vector := deterministicVector(input, dimensions)
		out = append(out, &gatewayv1.Embedding{
			Vector:      vector,
			Provider:    p.id,
			Model:       model,
			Dimensions:  int32(len(vector)),
			ContentHash: contentHash(input),
		})
	}
	return out, nil
}

func (p *localProvider) Health(context.Context) providerHealth {
	return providerHealth{Healthy: true, Status: "ready"}
}

func rerankLocal(query string, documents []string) []*gatewayv1.RerankResult {
	queryTerms := termSet(query)
	results := make([]*gatewayv1.RerankResult, 0, len(documents))
	for i, doc := range documents {
		score := float32(0)
		for term := range termSet(doc) {
			if _, ok := queryTerms[term]; ok {
				score++
			}
		}
		results = append(results, &gatewayv1.RerankResult{Index: int32(i), Score: score})
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			return results[i].Index < results[j].Index
		}
		return results[i].Score > results[j].Score
	})
	return results
}

func deterministicVector(input string, dimensions int) []float32 {
	out := make([]float32, dimensions)
	seed := sha256.Sum256([]byte(input))
	for i := range out {
		offset := (i * 4) % len(seed)
		value := binary.BigEndian.Uint32(seed[offset : offset+4])
		out[i] = (float32(value%2000) / 1000) - 1
	}
	var norm float32
	for _, value := range out {
		norm += value * value
	}
	if norm == 0 {
		return out
	}
	norm = float32(math.Sqrt(float64(norm)))
	for i := range out {
		out[i] = out[i] / norm
	}
	return out
}

func contentHash(input string) string {
	sum := sha256.Sum256([]byte(input))
	return fmt.Sprintf("%x", sum[:])
}

func lastUserMessage(messages []message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return strings.TrimSpace(messages[i].Content)
		}
	}
	return ""
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max]
}

func termSet(value string) map[string]struct{} {
	out := make(map[string]struct{})
	for _, field := range strings.Fields(strings.ToLower(value)) {
		field = strings.Trim(field, ".,:;!?()[]{}\"'")
		if field != "" {
			out[field] = struct{}{}
		}
	}
	return out
}
