package llm

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"

	"github.com/quarkloop/pkg/plugin"
	"github.com/quarkloop/runtime/pkg/modelservice"
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

// LoadFromURL fetches a model list from a remote URL and initializes clients.
func (r *Registry) LoadFromURL(url string, providers map[string]Provider) error {
	return r.LoadFromURLWithModelService(url, modelservice.New(providers, nil))
}

// LoadFromURLWithModelService fetches a model list and initializes clients
// through the model service boundary.
func (r *Registry) LoadFromURLWithModelService(url string, service *modelservice.Service) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("fetch model list: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("fetch model list: %d %s", resp.StatusCode, string(body))
	}

	var entries []plugin.ModelEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return fmt.Errorf("parse model list: %w", err)
	}

	return r.LoadEntriesWithModelService(entries, service)
}

// LoadEntries initializes clients from a list of model entries.
func (r *Registry) LoadEntries(entries []plugin.ModelEntry, providers map[string]Provider) error {
	return r.LoadEntriesWithModelService(entries, modelservice.New(providers, nil))
}

// LoadEntriesWithModelService initializes clients from a list of model entries
// and routes every provider call through the model service boundary.
func (r *Registry) LoadEntriesWithModelService(entries []plugin.ModelEntry, service *modelservice.Service) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, entry := range entries {
		if service == nil || !service.HasProvider(entry.Provider) {
			slog.Warn("provider not implemented, skipping model", "provider", entry.Provider, "model", entry.ID)
			continue
		}

		r.models[entry.ID] = NewClient(service.Adapter(entry.Provider), entry.ID, entry.ContextWindow)
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
