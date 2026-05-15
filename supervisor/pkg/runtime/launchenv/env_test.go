package launchenv

import (
	"slices"
	"testing"
)

func TestBuilderBuildsDeterministicEnvironmentWithPrecedence(t *testing.T) {
	builder := NewWithBase(
		[]string{"PATH=/bin", "QUARK_SPACE=old", "QUARK_MODEL_PROVIDER=old-provider"},
		"http://127.0.0.1:7200",
		[]string{"QUARK_SPACE_SERVICE_ADDR=127.0.0.1:7400"},
	)

	spec, err := builder.Build(Inputs{
		RuntimeID:  "rt-1",
		Space:      "space-1",
		WorkingDir: "/tmp/space",
		Port:       7777,
		PluginsDir: "/tmp/plugins",
		RuntimeEnv: []string{"QUARK_MODEL_PROVIDER=openrouter", "QUARK_MODEL_NAME=test-model"},
		CatalogEnv: []string{"QUARK_RUNTIME_PLUGIN_CATALOG={}", "QUARK_AGENT_PROFILE=quark-knowledge"},
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	for _, want := range []string{
		"PATH=/bin",
		"QUARK_SPACE=space-1",
		"QUARK_RUNTIME_ID=rt-1",
		"QUARK_MODEL_PROVIDER=openrouter",
		"QUARK_MODEL_NAME=test-model",
		"QUARK_RUNTIME_PLUGIN_CATALOG={}",
		"QUARK_AGENT_PROFILE=quark-knowledge",
		"QUARK_SPACE_SERVICE_ADDR=127.0.0.1:7400",
		"QUARK_SUPERVISOR_URL=http://127.0.0.1:7200",
		"QUARK_PLUGINS_DIR=/tmp/plugins",
	} {
		if !slices.Contains(spec.Env, want) {
			t.Fatalf("env missing %q: %v", want, spec.Env)
		}
	}
}

func TestBuilderCopiesInputs(t *testing.T) {
	base := []string{"PATH=/bin"}
	service := []string{"A=1"}
	runtimeEnv := []string{"B=1"}
	builder := NewWithBase(base, "", service)
	base[0] = "PATH=/mutated"
	service[0] = "A=mutated"

	spec, err := builder.Build(Inputs{
		RuntimeID:  "rt-1",
		Space:      "space-1",
		WorkingDir: "/tmp/space",
		Port:       7777,
		RuntimeEnv: runtimeEnv,
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	runtimeEnv[0] = "B=mutated"
	if !slices.Contains(spec.Env, "PATH=/bin") || !slices.Contains(spec.Env, "A=1") || !slices.Contains(spec.Env, "B=1") {
		t.Fatalf("env was not copied: %v", spec.Env)
	}
}

func TestBuilderRequiresProcessFields(t *testing.T) {
	if _, err := NewWithBase(nil, "", nil).Build(Inputs{}); err == nil {
		t.Fatal("expected validation error")
	}
}
