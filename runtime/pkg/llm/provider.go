package llm

import "github.com/quarkloop/pkg/plugin"

// Provider is the runtime inference boundary. Production providers are
// Gateway-backed adapters.
type Provider = plugin.Provider
