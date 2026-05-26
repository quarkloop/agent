// Package harnessclient provides the runtime's NATS boundary to Harness.
package harnessclient

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/quarkloop/pkg/natskit"
	"github.com/quarkloop/pkg/plugin"
	"github.com/quarkloop/runtime/pkg/runcontext"

	harnessv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/harness/v1"
)

type Material struct {
	SourceID   string
	SourceKind string
	Content    string
	Required   bool
}

type Input struct {
	Materials     []Material
	RuntimeFacts  []Material
	History       []plugin.Message
	ContextWindow int
}

type Composer interface {
	Compose(context.Context, Input) ([]plugin.Message, error)
}

type Config struct {
	URL      string
	Username string
	Password string
	Name     string
	SpaceID  string
	Timeout  time.Duration
}

type Client struct {
	cfg Config
	mu  sync.Mutex
	nc  *natskit.Client
}

func New(cfg Config) *Client {
	if cfg.Name == "" {
		cfg.Name = "quark-runtime-harness"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	return &Client{cfg: cfg}
}

func (c *Client) Compose(ctx context.Context, input Input) ([]plugin.Message, error) {
	if c == nil {
		return nil, fmt.Errorf("harness client is not configured")
	}
	spaceID := firstNonEmpty(runcontext.SpaceID(ctx), c.cfg.SpaceID)
	if spaceID == "" {
		return nil, fmt.Errorf("space id is required for harness context composition")
	}
	request := &harnessv1.ComposeContextRequest{
		Space:           spaceID,
		SessionId:       runcontext.SessionID(ctx),
		RunId:           runcontext.RunID(ctx),
		ContextWindow:   int32(input.ContextWindow),
		SystemMaterials: materialsToProto(input.Materials),
		RuntimeFacts:    materialsToProto(input.RuntimeFacts),
		History:         messagesToProto(input.History),
	}
	payload, err := protojson.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("encode harness context request: %w", err)
	}
	envelope, err := natskit.NewRequest(natskit.NewServiceCallID(), spaceID, natskit.ActorRuntime, json.RawMessage(payload))
	if err != nil {
		return nil, err
	}
	envelope.SessionID = runcontext.SessionID(ctx)
	envelope.RunID = runcontext.RunID(ctx)
	operation, err := natskit.ServiceOperation("harness", "compose_context")
	if err != nil {
		return nil, err
	}
	callCtx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()
	client, err := c.connection(callCtx)
	if err != nil {
		return nil, err
	}
	response, err := client.Call(callCtx, operation, envelope)
	if err != nil {
		return nil, fmt.Errorf("compose harness context: %w", err)
	}
	if response.Status != natskit.StatusOK {
		if response.Error != nil {
			return nil, fmt.Errorf("compose harness context: %s", response.Error.Message)
		}
		return nil, fmt.Errorf("compose harness context failed")
	}
	var output harnessv1.ComposeContextResponse
	if err := protojson.Unmarshal(response.Payload, &output); err != nil {
		return nil, fmt.Errorf("decode harness context response: %w", err)
	}
	return messagesFromProto(output.GetMessages()), nil
}

func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.nc != nil {
		c.nc.Close()
		c.nc = nil
	}
}

func (c *Client) connection(ctx context.Context) (*natskit.Client, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.nc != nil {
		return c.nc, nil
	}
	if strings.TrimSpace(c.cfg.URL) == "" {
		return nil, fmt.Errorf("NATS URL is required for harness context composition")
	}
	client, err := natskit.Connect(ctx, natskit.Config{
		URL: c.cfg.URL, Username: c.cfg.Username, Password: c.cfg.Password,
		Name: c.cfg.Name, Timeout: c.cfg.Timeout,
	})
	if err != nil {
		return nil, err
	}
	c.nc = client
	return client, nil
}

func materialsToProto(materials []Material) []*harnessv1.PromptMaterial {
	out := make([]*harnessv1.PromptMaterial, 0, len(materials))
	for _, material := range materials {
		if strings.TrimSpace(material.Content) == "" {
			continue
		}
		out = append(out, &harnessv1.PromptMaterial{
			SourceId: material.SourceID, SourceKind: material.SourceKind,
			Content: material.Content, Required: material.Required,
		})
	}
	return out
}

func messagesToProto(messages []plugin.Message) []*harnessv1.ContextMessage {
	out := make([]*harnessv1.ContextMessage, 0, len(messages))
	for i, message := range messages {
		out = append(out, &harnessv1.ContextMessage{
			Role: message.Role, Content: message.Content,
			SourceId: fmt.Sprintf("session.message.%d", i+1),
		})
	}
	return out
}

func messagesFromProto(messages []*harnessv1.ContextMessage) []plugin.Message {
	out := make([]plugin.Message, 0, len(messages))
	for _, message := range messages {
		out = append(out, plugin.Message{Role: message.GetRole(), Content: message.GetContent()})
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
