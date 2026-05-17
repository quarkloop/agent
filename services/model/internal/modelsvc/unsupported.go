package modelsvc

import (
	"context"
	"fmt"

	"github.com/quarkloop/pkg/plugin"
	modelv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/model/v1"
)

type unsupportedProvider struct {
	id    string
	model string
}

func newUnsupportedProvider(cfg ProviderConfig) provider {
	return &unsupportedProvider{id: cfg.ID, model: cfg.Model}
}

func (p *unsupportedProvider) ID() string { return p.id }

func (p *unsupportedProvider) ListModels(context.Context) ([]*modelv1.ModelInfo, error) {
	return []*modelv1.ModelInfo{{
		Id:       p.model,
		Provider: p.id,
		Name:     p.model,
	}}, nil
}

func (p *unsupportedProvider) StreamGenerate(context.Context, generateCommand) (<-chan streamEvent, error) {
	return nil, plugin.NewProviderError(plugin.ProviderErrorModelUnavailable, p.id, p.model, 0, fmt.Errorf("provider adapter is not implemented yet"))
}

func (p *unsupportedProvider) Embed(context.Context, embedCommand) ([]*modelv1.Embedding, error) {
	return nil, plugin.NewProviderError(plugin.ProviderErrorModelUnavailable, p.id, p.model, 0, fmt.Errorf("provider adapter is not implemented yet"))
}

func (p *unsupportedProvider) Health(context.Context) providerHealth {
	return providerHealth{Healthy: false, Status: "provider adapter is not implemented yet"}
}
