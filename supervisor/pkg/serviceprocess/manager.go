// Package serviceprocess owns supervisor-managed local service processes.
package serviceprocess

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/quarkloop/supervisor/pkg/api"
)

type ProcessSpec struct {
	Space         string
	Name          string
	Binary        string
	Args          []string
	Env           []string
	WorkingDir    string
	Endpoint      string
	HealthService string
	LogPath       string
}

type State struct {
	Space         string
	Name          string
	Status        api.ServiceStatus
	PID           int
	Endpoint      string
	HealthService string
	StartedAt     time.Time
	StoppedAt     time.Time
	LogPath       string
	Diagnostics   []string
}

type Manager struct {
	mu        sync.RWMutex
	processes map[string]*processEntry
}

type processEntry struct {
	spec  ProcessSpec
	state State
	cmd   *exec.Cmd
	log   *os.File
	done  chan struct{}
}

func NewManager() *Manager {
	return &Manager{processes: make(map[string]*processEntry)}
}

func (m *Manager) Start(ctx context.Context, spec ProcessSpec) (State, error) {
	if err := spec.validate(); err != nil {
		return State{}, err
	}
	key := processKey(spec.Space, spec.Name)

	m.mu.Lock()
	if existing, ok := m.processes[key]; ok && existing.state.Status != api.ServiceStatusStopped && existing.state.Status != api.ServiceStatusUnavailable {
		state := cloneState(existing.state)
		m.mu.Unlock()
		return state, fmt.Errorf("service %s/%s is already %s", spec.Space, spec.Name, state.Status)
	}

	logFile, err := openLogFile(spec.LogPath)
	if err != nil {
		m.mu.Unlock()
		return State{}, err
	}
	cmd := exec.CommandContext(context.WithoutCancel(ctx), spec.Binary, spec.Args...)
	cmd.Dir = spec.WorkingDir
	cmd.Env = append([]string(nil), spec.Env...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		m.mu.Unlock()
		return State{}, fmt.Errorf("start service %s/%s: %w", spec.Space, spec.Name, err)
	}

	entry := &processEntry{
		spec: spec,
		state: State{
			Space:         spec.Space,
			Name:          spec.Name,
			Status:        api.ServiceStatusStarting,
			PID:           cmd.Process.Pid,
			Endpoint:      spec.Endpoint,
			HealthService: spec.HealthService,
			StartedAt:     time.Now().UTC(),
			LogPath:       spec.LogPath,
		},
		cmd:  cmd,
		log:  logFile,
		done: make(chan struct{}),
	}
	m.processes[key] = entry
	state := cloneState(entry.state)
	m.mu.Unlock()

	go m.wait(key, entry)
	return state, nil
}

func (m *Manager) MarkReady(space, name string) {
	m.setStatus(space, name, api.ServiceStatusReady, nil)
}

func (m *Manager) MarkUnavailable(space, name string, diagnostics ...string) {
	m.setStatus(space, name, api.ServiceStatusUnavailable, diagnostics)
}

func (m *Manager) Stop(ctx context.Context, space, name string) (State, error) {
	key := processKey(space, name)
	m.mu.RLock()
	entry, ok := m.processes[key]
	if !ok {
		m.mu.RUnlock()
		return State{}, fmt.Errorf("service %s/%s is not managed", space, name)
	}
	if entry.cmd == nil || entry.cmd.Process == nil {
		state := cloneState(entry.state)
		m.mu.RUnlock()
		return state, fmt.Errorf("service %s/%s is not running", space, name)
	}
	done := entry.done
	process := entry.cmd.Process
	m.mu.RUnlock()

	if err := process.Signal(syscall.SIGTERM); err != nil {
		return State{}, fmt.Errorf("signal service %s/%s: %w", space, name, err)
	}
	m.setStatus(space, name, api.ServiceStatusStopping, nil)
	select {
	case <-done:
	case <-ctx.Done():
		_ = process.Kill()
		return State{}, ctx.Err()
	case <-time.After(5 * time.Second):
		_ = process.Kill()
		<-done
	}
	state, _ := m.Inspect(space, name)
	return state, nil
}

func (m *Manager) Restart(ctx context.Context, spec ProcessSpec) (State, error) {
	if _, ok := m.Inspect(spec.Space, spec.Name); ok {
		_, _ = m.Stop(ctx, spec.Space, spec.Name)
	}
	return m.Start(ctx, spec)
}

func (m *Manager) Inspect(space, name string) (State, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	entry, ok := m.processes[processKey(space, name)]
	if !ok {
		return State{}, false
	}
	return cloneState(entry.state), true
}

func (m *Manager) Logs(space, name string, maxBytes int64) (string, State, bool, error) {
	state, ok := m.Inspect(space, name)
	if !ok {
		return "", State{}, false, nil
	}
	if maxBytes <= 0 {
		maxBytes = 64 * 1024
	}
	data, err := tailFile(state.LogPath, maxBytes)
	if err != nil {
		return "", state, true, err
	}
	return string(data), state, true, nil
}

func (m *Manager) setStatus(space, name string, status api.ServiceStatus, diagnostics []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if entry, ok := m.processes[processKey(space, name)]; ok {
		entry.state.Status = status
		entry.state.Diagnostics = append([]string(nil), diagnostics...)
	}
}

func (m *Manager) wait(key string, entry *processEntry) {
	err := entry.cmd.Wait()
	_ = entry.log.Close()
	m.mu.Lock()
	if current, ok := m.processes[key]; ok && current == entry {
		current.state.Status = api.ServiceStatusStopped
		current.state.PID = 0
		current.state.StoppedAt = time.Now().UTC()
		if err != nil {
			current.state.Diagnostics = []string{err.Error()}
		}
		current.cmd = nil
		current.log = nil
	}
	close(entry.done)
	m.mu.Unlock()
}

func (s ProcessSpec) validate() error {
	if s.Space == "" {
		return fmt.Errorf("service process space is required")
	}
	if s.Name == "" {
		return fmt.Errorf("service process name is required")
	}
	if s.Binary == "" {
		return fmt.Errorf("service process binary is required")
	}
	if s.Endpoint == "" {
		return fmt.Errorf("service process endpoint is required")
	}
	if s.WorkingDir == "" {
		return fmt.Errorf("service process working directory is required")
	}
	if s.LogPath == "" {
		return fmt.Errorf("service process log path is required")
	}
	return nil
}

func processKey(space, name string) string {
	return space + "/" + name
}

func cloneState(state State) State {
	state.Diagnostics = append([]string(nil), state.Diagnostics...)
	return state
}

func openLogFile(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create service log dir: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open service log %s: %w", path, err)
	}
	return file, nil
}

func tailFile(path string, maxBytes int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	offset := info.Size() - maxBytes
	if offset < 0 {
		offset = 0
	}
	if _, err := file.Seek(offset, 0); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}
	return data, nil
}
