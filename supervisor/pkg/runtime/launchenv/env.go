package launchenv

import (
	"fmt"
	"os"
	"strings"
)

type Builder struct {
	base          []string
	supervisorURL string
	serviceEnv    []string
}

type Inputs struct {
	RuntimeID  string
	Space      string
	WorkingDir string
	Port       int
	PluginsDir string
	RuntimeEnv []string
	CatalogEnv []string
}

type ProcessSpec struct {
	RuntimeID  string
	WorkingDir string
	Port       int
	Env        []string
}

func New(supervisorURL string, serviceEnv []string) Builder {
	return NewWithBase(os.Environ(), supervisorURL, serviceEnv)
}

func NewWithBase(base []string, supervisorURL string, serviceEnv []string) Builder {
	return Builder{
		base:          cloneStrings(base),
		supervisorURL: strings.TrimSpace(supervisorURL),
		serviceEnv:    cloneStrings(serviceEnv),
	}
}

func (b Builder) Build(in Inputs) (ProcessSpec, error) {
	if strings.TrimSpace(in.RuntimeID) == "" {
		return ProcessSpec{}, fmt.Errorf("runtime id is required")
	}
	if strings.TrimSpace(in.Space) == "" {
		return ProcessSpec{}, fmt.Errorf("space is required")
	}
	if strings.TrimSpace(in.WorkingDir) == "" {
		return ProcessSpec{}, fmt.Errorf("working directory is required")
	}
	if in.Port == 0 {
		return ProcessSpec{}, fmt.Errorf("port is required")
	}

	env := newEnvSet(b.base)
	env.Set("QUARK_RUNTIME_ID", in.RuntimeID)
	env.Set("QUARK_SPACE", in.Space)
	env.Add(in.RuntimeEnv)
	env.Add(in.CatalogEnv)
	env.Add(b.serviceEnv)
	if b.supervisorURL != "" {
		env.Set("QUARK_SUPERVISOR_URL", b.supervisorURL)
	}
	if strings.TrimSpace(in.PluginsDir) != "" {
		env.Set("QUARK_PLUGINS_DIR", in.PluginsDir)
	}
	return ProcessSpec{
		RuntimeID:  in.RuntimeID,
		WorkingDir: in.WorkingDir,
		Port:       in.Port,
		Env:        env.List(),
	}, nil
}

type envSet struct {
	order []string
	value map[string]string
}

func newEnvSet(base []string) *envSet {
	set := &envSet{order: make([]string, 0, len(base)), value: make(map[string]string, len(base))}
	set.Add(base)
	return set
}

func (e *envSet) Add(entries []string) {
	for _, entry := range entries {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || key == "" {
			continue
		}
		e.Set(key, value)
	}
}

func (e *envSet) Set(key, value string) {
	if _, exists := e.value[key]; !exists {
		e.order = append(e.order, key)
	}
	e.value[key] = value
}

func (e *envSet) List() []string {
	out := make([]string, 0, len(e.order))
	for _, key := range e.order {
		out = append(out, key+"="+e.value[key])
	}
	return out
}

func cloneStrings(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}
