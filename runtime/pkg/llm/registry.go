package llm

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/quarkloop/pkg/plugin"
)

// Registry manages available LLM models and their clients.
type Registry struct {
	mu      sync.RWMutex
	models  map[string]*Client // keyed by model ID
	Default string             // default model ID
}

// NewRegistry creates a new empty model Registry.
func NewRegistry() *Registry {
	return &Registry{
		models: make(map[string]*Client),
	}
}

// LoadEntries initializes clients from models and their Gateway client
// adapters. Provider routing policy is owned by Gateway, not this registry.
func (r *Registry) LoadEntries(entries []plugin.ModelEntry, providers map[string]Provider) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, entry := range entries {
		provider, ok := providers[entry.Provider]
		if !ok || provider == nil {
			slog.Warn("gateway adapter not available, skipping model", "provider", entry.Provider, "model", entry.ID)
			continue
		}

		r.models[entry.ID] = NewClient(provider, entry.ID, entry.ContextWindow)
		slog.Info("model registered", "model", entry.ID, "provider", entry.Provider, "context_window", entry.ContextWindow)

		if entry.Default || r.Default == "" {
			r.Default = entry.ID
		}
	}

	if len(r.models) == 0 {
		return fmt.Errorf("no models loaded — no matching providers")
	}

	slog.Info("models loaded", "count", len(r.models), "default", r.Default)
	return nil
}

// Get returns a client for the given model ID.
func (r *Registry) Get(modelID string) *Client {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.models[modelID]
}

// GetDefault returns the default model client.
func (r *Registry) GetDefault() *Client {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.models[r.Default]
}

// SetDefault changes the default model.
func (r *Registry) SetDefault(modelID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.models[modelID]; !ok {
		return false
	}
	r.Default = modelID
	return true
}

// List returns all registered model IDs.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.models))
	for id := range r.models {
		out = append(out, id)
	}
	return out
}

// AddModel dynamically adds a model to the registry.
func (r *Registry) AddModel(modelID string, p Provider, contextWindow int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.models[modelID] = NewClient(p, modelID, contextWindow)
}

// RemoveModel dynamically removes a model from the registry.
func (r *Registry) RemoveModel(modelID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.models, modelID)
	if r.Default == modelID {
		r.Default = ""
		for id := range r.models {
			r.Default = id
			break
		}
	}
}
