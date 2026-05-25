package agent

import (
	"context"
	"log/slog"

	"github.com/quarkloop/pkg/plugin"
	"github.com/quarkloop/runtime/pkg/channel"
	"github.com/quarkloop/runtime/pkg/loop"
	"github.com/quarkloop/runtime/pkg/modelservice"
)

// sendInitMessages queues initialization messages after plugin discovery.
func (a *Agent) sendInitMessages() {
	providers := a.Plugins.GetProviders()
	if len(providers) == 0 {
		slog.Warn("no Gateway-backed model adapters registered")
	}
	for id := range providers {
		slog.Info("Gateway model adapter available", "provider", id)
	}
	fallback := []plugin.ModelEntry{}
	if a.config.Model != "" {
		fallback = append(fallback, plugin.ModelEntry{
			ID:       a.config.Model,
			Provider: a.config.ModelProvider,
			Name:     a.config.Model,
			Default:  true,
		})
	}
	msg := NewInitLLMMsg()
	msg.ModelListURL = a.config.ModelListURL
	msg.Providers = providers
	msg.Fallback = fallback
	a.loop.Send(msg)
}

func (a *Agent) handleInitLLM(_ context.Context, msg loop.Message) error {
	payload := msg.(InitLLMMsg)
	slog.Info("initializing LLM models")
	models := modelservice.New(payload.Providers, a.recordModelUsage)
	if payload.ModelListURL != "" {
		if err := a.Models.LoadFromURLWithGatewayService(payload.ModelListURL, models); err != nil {
			slog.Warn("remote model list failed, using fallback", "error", err)
		}
	}
	if a.Models.GetDefault() == nil && len(payload.Fallback) > 0 {
		if err := a.Models.LoadEntriesWithGatewayService(payload.Fallback, models); err != nil {
			slog.Error("fallback model init failed", "error", err)
		}
	}
	if a.Models.GetDefault() != nil {
		slog.Info("LLM ready", "default_model", a.Models.Default)
	} else {
		slog.Warn("no LLM models available")
	}
	return nil
}

func (a *Agent) handleInitChannel(_ context.Context, msg loop.Message) error {
	payload := msg.(InitChannelMsg)
	if bus, ok := payload.Bus.(*channel.ChannelBus); ok {
		a.Bus = bus
		slog.Info("channel bus registered", "active_channels", len(a.Bus.ActiveChannels()))
	}
	return nil
}

func (a *Agent) handleSetModel(_ context.Context, msg loop.Message) error {
	payload := msg.(SetModelMsg)
	if a.Models.SetDefault(payload.ModelID) {
		slog.Info("switched default model", "model_id", payload.ModelID)
	} else {
		slog.Warn("model not found in registry", "model_id", payload.ModelID)
	}
	return nil
}
