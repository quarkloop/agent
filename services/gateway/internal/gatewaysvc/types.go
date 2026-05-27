package gatewaysvc

import (
	"context"

	gatewayv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/gateway/v1"
)

type Config struct {
	Providers           []ProviderConfig
	Fallbacks           map[string][]string
	EmbeddingProvider   string
	MaxExternalRequests int64
	Logger              logger
}

type ProviderConfig struct {
	ID             string
	Kind           string
	APIKey         string
	BaseURL        string
	Model          string
	EmbeddingModel string
	Enabled        bool
}

type logger interface {
	Warn(msg string, args ...any)
}

type provider interface {
	ID() string
	ListModels(context.Context) ([]*gatewayv1.ModelInfo, error)
	StreamGenerate(context.Context, generateCommand) (<-chan streamEvent, error)
	Embed(context.Context, embedCommand) ([]*gatewayv1.Embedding, error)
	Health(context.Context) providerHealth
}

type generateCommand struct {
	Model    string
	Messages []message
	Tools    []toolSchema
	Options  map[string]string
}

type embedCommand struct {
	Model      string
	Inputs     []multimodalInput
	Dimensions int32
	Options    map[string]string
}

type message struct {
	Role       string
	Content    []contentPart
	ToolCalls  []toolCall
	ToolCallID string
}

type contentKind uint8

const (
	contentText contentKind = iota + 1
	contentImageURL
	contentImageData
	contentContentRef
	contentImageRef
	contentPageRef
	contentArtifactRef
	contentFileRef
)

type contentPart struct {
	Kind      contentKind
	Text      string
	ImageURL  string
	ImageData []byte
	MIMEType  string
	Ref       string
	Metadata  map[string]string
}

type multimodalInput struct {
	Content  []contentPart
	Metadata map[string]string
}

type toolSchema struct {
	Name           string
	Description    string
	ParametersJSON string
}

type toolCall struct {
	Index         int32
	ID            string
	Type          string
	Name          string
	ArgumentsJSON string
}

type streamEvent struct {
	Delta     string
	ToolCalls []toolCall
	Done      bool
	Usage     *modelUsage
	Err       error
}

type providerHealth struct {
	Healthy bool
	Status  string
}

type modelUsage struct {
	Provider        string
	Model           string
	InputTokens     int64
	OutputTokens    int64
	EmbeddingTokens int64
	LatencyMillis   int64
	CostEstimate    float64
	FallbackChain   []string
	RequestID       string
	FinishReason    string
}

type StreamGenerateEvent struct {
	Response *gatewayv1.StreamGenerateResponse
	Err      error
}

type closableProvider interface {
	Close() error
}
