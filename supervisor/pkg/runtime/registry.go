// Package runtime tracks running processes managed by the supervisor.
// The registry is purely in-memory: it does not persist state. Durable data
// (spaces, Quarkfile, KB) lives in the space.Store.
package runtime

import (
	"fmt"
	"os/exec"
	"sync"
	"time"

	"github.com/quarkloop/supervisor/pkg/api"
)

// Runtime is the record for a running process.
type Runtime struct {
	id         string
	space      string
	workingDir string
	pluginsDir string
	status     api.RuntimeStatus
	pid        int
	port       int
	startedAt  time.Time
	cmd        *exec.Cmd
}

// NewRuntime creates a new Runtime record.
func NewRuntime(id, space, workingDir, pluginsDir string) *Runtime {
	return &Runtime{
		id:         id,
		space:      space,
		workingDir: workingDir,
		pluginsDir: pluginsDir,
		status:     api.RuntimeStarting,
	}
}

// Getter methods

func (r *Runtime) ID() string                { return r.id }
func (r *Runtime) Space() string             { return r.space }
func (r *Runtime) WorkingDir() string        { return r.workingDir }
func (r *Runtime) PluginsDir() string        { return r.pluginsDir }
func (r *Runtime) Status() api.RuntimeStatus { return r.status }
func (r *Runtime) PID() int                  { return r.pid }
func (r *Runtime) Port() int                 { return r.port }
func (r *Runtime) StartedAt() time.Time      { return r.startedAt }

// Mutator methods

func (r *Runtime) SetPort(port int) { r.port = port }

// Registry tracks running processes keyed by runtime ID.
type Registry struct {
	mu       sync.RWMutex
	runtimes map[string]*Runtime
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{runtimes: make(map[string]*Runtime)}
}

// Register adds a runtime entry.
func (r *Registry) Register(rt *Runtime) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runtimes[rt.ID()] = rt
}

// Get returns the runtime with the given ID or an error if it does not exist.
func (r *Registry) Get(id string) (*Runtime, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rt, ok := r.runtimes[id]
	if !ok {
		return nil, fmt.Errorf("runtime %q not found", id)
	}
	return rt, nil
}

// GetBySpace returns the first runtime running for the given space name, or an
// error if no runtime is running for it.
func (r *Registry) GetBySpace(space string) (*Runtime, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, rt := range r.runtimes {
		if rt.Space() == space {
			return rt, nil
		}
	}
	return nil, fmt.Errorf("no runtime running for space %q", space)
}

// List returns a snapshot of all tracked runtimes.
func (r *Registry) List() []*Runtime {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Runtime, 0, len(r.runtimes))
	for _, rt := range r.runtimes {
		out = append(out, rt)
	}
	return out
}

// Remove drops a runtime from the registry. It does not stop the process.
func (r *Registry) Remove(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.runtimes, id)
}

// SetStatus updates the status of a runtime.
func (r *Registry) MarkRunning(id string, handle ProcessHandle) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	rt, ok := r.runtimes[id]
	if !ok {
		return fmt.Errorf("runtime %q not found", id)
	}
	rt.cmd = handle.Cmd
	rt.pid = handle.PID
	rt.startedAt = handle.StartedAt
	rt.status = api.RuntimeRunning
	return nil
}

func (r *Registry) MarkStopping(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	rt, ok := r.runtimes[id]
	if !ok {
		return fmt.Errorf("runtime %q not found", id)
	}
	if rt.status != api.RuntimeRunning && rt.status != api.RuntimeStarting {
		return fmt.Errorf("runtime %s is not running (status: %s)", id, rt.status)
	}
	rt.status = api.RuntimeStopping
	return nil
}

// SetStopped marks a stopped runtime's fields under the registry lock.
// This is called from the goroutine that waits on cmd.Wait() to avoid
// data races with concurrent Get/GetBySpace readers.
func (r *Registry) SetStopped(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if rt, ok := r.runtimes[id]; ok {
		rt.status = api.RuntimeStopped
		rt.pid = 0
		rt.cmd = nil
	}
}
