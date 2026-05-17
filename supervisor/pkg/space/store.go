package space

import (
	"github.com/quarkloop/supervisor/pkg/api"
	"github.com/quarkloop/supervisor/pkg/kb"
	"github.com/quarkloop/supervisor/pkg/pluginmanager"
	"github.com/quarkloop/supervisor/pkg/sessions"
)

// Store is the supervisor's persistent space registry. All spaces are
// identified by Name (meta.name from the Quarkfile). The store keeps only the
// latest Quarkfile state; Quarkfile history is user-owned.
//
// Implementations must be safe for concurrent use.
type Store interface {
	// Create registers a new space with the given name and initial Quarkfile
	// contents. The Quarkfile is written to workingDir.
	// Returns ErrAlreadyExists if a space with that name is already registered.
	Create(name string, quarkfile []byte, workingDir string) (*Space, error)

	// UpdateQuarkfile replaces the latest Quarkfile for the named space.
	UpdateQuarkfile(name string, quarkfile []byte) (*Space, error)

	// Get returns the metadata for the named space.
	Get(name string) (*Space, error)

	// List returns every registered space.
	List() ([]*Space, error)

	// Delete permanently removes the named space and all of its data.
	Delete(name string) error

	// Quarkfile returns the latest stored Quarkfile contents.
	Quarkfile(name string) (contents []byte, err error)

	// AgentEnvironment returns concrete environment entries derived from the
	// latest Quarkfile model declaration.
	AgentEnvironment(name string) ([]string, error)

	// KB opens the knowledge-base store scoped to the named space.
	KB(name string) (kb.Store, error)

	// Plugins returns the plugin manager scoped to the named space.
	Plugins(name string) (*pluginmanager.Installer, error)

	// Sessions returns the session store scoped to the named space.
	Sessions(name string) (*sessions.Store, error)

	// ServiceStateDir returns a supervisor-owned state directory for a service
	// process scoped to the named space.
	ServiceStateDir(name, service string) (string, error)

	// Doctor runs health checks against the named space's Quarkfile and
	// installed plugins.
	Doctor(name string) (api.DoctorResponse, error)
}
