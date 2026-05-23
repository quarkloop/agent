package llm

import "github.com/quarkloop/pkg/plugin"

// Provider is the runtime inference boundary. Production providers are
// Gateway-backed adapters; runtime does not load provider plugins directly.
type Provider = plugin.Provider
