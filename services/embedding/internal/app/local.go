package app

import (
	"context"
	"crypto/sha256"
	"math"
	"strings"
	"unicode"
)

type localEmbedder struct {
	model      string
	dimensions int
}

func (e localEmbedder) Embed(ctx context.Context, input, model string, dimensions int) (embeddingResult, error) {
	_ = ctx
	if strings.TrimSpace(model) == "" {
		model = e.model
	}
	if dimensions <= 0 {
		dimensions = e.dimensions
	}
	return embeddingResult{
		Vector:   deterministicVector(input, dimensions),
		Model:    model,
		Provider: e.Provider(),
	}, nil
}

func (e localEmbedder) Provider() string { return "local" }
func (e localEmbedder) Model() string    { return e.model }
func (e localEmbedder) Dimensions() int  { return e.dimensions }
func (e localEmbedder) Description() string {
	return "Create a deterministic local embedding vector for text."
}

func deterministicVector(text string, dimensions int) []float32 {
	vector := make([]float32, dimensions)
	for _, token := range tokenize(text) {
		sum := sha256.Sum256([]byte(token))
		idx := int(sum[0]) % dimensions
		sign := float32(1)
		if sum[1]%2 == 1 {
			sign = -1
		}
		vector[idx] += sign * (1 + float32(len(token)%7)/10)
	}
	var norm float64
	for _, value := range vector {
		norm += float64(value * value)
	}
	if norm == 0 {
		vector[0] = 1
		return vector
	}
	scale := float32(1 / math.Sqrt(norm))
	for i := range vector {
		vector[i] *= scale
	}
	return vector
}

func cloneVector(in []float32) []float32 {
	out := make([]float32, len(in))
	copy(out, in)
	return out
}

func tokenize(text string) []string {
	fields := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	out := fields[:0]
	for _, field := range fields {
		if field != "" {
			out = append(out, field)
		}
	}
	return out
}
