package buildrelease

import (
	"context"
	"os"
	"time"

	"github.com/quarkloop/pkg/plugin"
	"github.com/quarkloop/pkg/toolkit"
	core "github.com/quarkloop/services/build-release/pkg/buildrelease"
)

// Tool is the backwards-compatible plugin adapter for the build-release
// service pipeline.
type Tool struct {
	manifest *plugin.Manifest
	runner   *core.Runner
}

func (t *Tool) SetManifest(m *plugin.Manifest) {
	t.manifest = m
}

func (t *Tool) Name() string {
	if t.manifest == nil {
		return "build-release"
	}
	return t.manifest.Name
}

func (t *Tool) Version() string {
	if t.manifest == nil {
		return "1.0.0"
	}
	return t.manifest.Version
}

func (t *Tool) Description() string {
	if t.manifest == nil {
		return "Compatibility adapter for build-release service functions"
	}
	return t.manifest.Description
}

func (t *Tool) Schema() plugin.ToolSchema {
	if t.manifest != nil && t.manifest.Tool != nil {
		return t.manifest.Tool.Schema
	}
	return plugin.ToolSchema{
		Name:        "build-release",
		Description: "Compatibility adapter for build-release service functions",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type": "string",
					"enum": []string{"release", "dryrun", "init", "schema", "health", "skill"},
				},
				"config": map[string]any{
					"type":        "string",
					"description": "Path to build_release.json config file",
				},
				"version": map[string]any{
					"type":        "string",
					"description": "Override version string",
				},
				"parallel": map[string]any{
					"type":        "integer",
					"description": "Number of parallel builds",
				},
				"skip_tests": map[string]any{
					"type":        "boolean",
					"description": "Skip running tests before build",
				},
			},
			"required": []string{"command"},
		},
	}
}

func (t *Tool) Commands() []toolkit.Command {
	t.ensureRunner()
	return []toolkit.Command{
		{
			Name:        "release",
			Description: "Run the full release pipeline",
			Args: []toolkit.Arg{
				{Name: "config", Description: "Config file path", Required: false, Default: "build_release.json"},
			},
			Flags: []toolkit.Flag{
				{Name: "version", Type: "string", Description: "Override version", Default: ""},
				{Name: "parallel", Type: "int", Description: "Parallel builds", Default: 0},
				{Name: "skip-tests", Type: "bool", Description: "Skip tests", Default: false},
			},
			Handler: t.handleRelease,
		},
		{
			Name:        "dryrun",
			Description: "Preview what would be built without compiling",
			Args: []toolkit.Arg{
				{Name: "config", Description: "Config file path", Required: false, Default: "build_release.json"},
			},
			Flags: []toolkit.Flag{
				{Name: "version", Type: "string", Description: "Override version", Default: ""},
			},
			Handler: t.handleDryRun,
		},
		{
			Name:        "init",
			Description: "Scaffold a new project with build_release files",
			Handler:     t.handleInit,
		},
	}
}

func (t *Tool) handleRelease(ctx context.Context, input toolkit.Input) (toolkit.Output, error) {
	t.ensureRunner()
	req, err := releaseRequestFromInput(input)
	if err != nil {
		return toolkit.Output{Error: err.Error()}, nil
	}
	result, err := t.runner.Release(ctx, req)
	if err != nil {
		return toolkit.Output{Error: err.Error()}, nil
	}
	return toolkit.Output{Data: map[string]any{
		"version":     result.Version,
		"artifacts":   artifactsForOutput(result.Artifacts),
		"release_dir": result.ReleaseDir,
	}}, nil
}

func (t *Tool) handleDryRun(ctx context.Context, input toolkit.Input) (toolkit.Output, error) {
	t.ensureRunner()
	req, err := dryRunRequestFromInput(input)
	if err != nil {
		return toolkit.Output{Error: err.Error()}, nil
	}
	result, err := t.runner.DryRun(ctx, req)
	if err != nil {
		return toolkit.Output{Error: err.Error()}, nil
	}
	return toolkit.Output{Data: map[string]any{
		"version": result.Version,
		"planned": artifactsForOutput(result.Planned),
	}}, nil
}

func (t *Tool) handleInit(ctx context.Context, input toolkit.Input) (toolkit.Output, error) {
	t.ensureRunner()
	workingDir, err := os.Getwd()
	if err != nil {
		return toolkit.Output{Error: err.Error()}, nil
	}
	result, err := t.runner.Init(ctx, core.InitRequest{WorkingDir: workingDir})
	if err != nil {
		return toolkit.Output{Error: err.Error()}, nil
	}
	return toolkit.Output{Data: map[string]any{
		"config_path": result.ConfigPath,
		"created":     result.Created,
	}}, nil
}

func (t *Tool) ensureRunner() {
	if t.runner == nil {
		t.runner = core.NewRunner()
	}
}

func releaseRequestFromInput(input toolkit.Input) (core.ReleaseRequest, error) {
	workingDir, err := os.Getwd()
	if err != nil {
		return core.ReleaseRequest{}, err
	}
	req := core.ReleaseRequest{
		WorkingDir:  workingDir,
		ConfigPath:  configFromInput(input),
		Parallelism: 4,
	}
	if v, ok := input.Flags["version"].(string); ok && v != "" {
		req.Version = v
	}
	if p, ok := input.Flags["parallel"].(int); ok && p > 0 {
		req.Parallelism = p
	}
	if s, ok := input.Flags["skip-tests"].(bool); ok {
		req.SkipTests = s
	}
	return req, nil
}

func dryRunRequestFromInput(input toolkit.Input) (core.DryRunRequest, error) {
	workingDir, err := os.Getwd()
	if err != nil {
		return core.DryRunRequest{}, err
	}
	req := core.DryRunRequest{
		WorkingDir:  workingDir,
		ConfigPath:  configFromInput(input),
		Parallelism: 4,
	}
	if v, ok := input.Flags["version"].(string); ok && v != "" {
		req.Version = v
	}
	if p, ok := input.Flags["parallel"].(int); ok && p > 0 {
		req.Parallelism = p
	}
	return req, nil
}

func configFromInput(input toolkit.Input) string {
	cfgFile := input.Args["config"]
	if cfgFile == "" {
		return "build_release.json"
	}
	return cfgFile
}

func artifactsForOutput(artifacts []core.Artifact) []map[string]any {
	out := make([]map[string]any, 0, len(artifacts))
	for _, artifact := range artifacts {
		out = append(out, map[string]any{
			"build":           artifact.BuildName,
			"os":              artifact.Target.OS,
			"arch":            artifact.Target.Arch,
			"arm":             artifact.Target.ARM,
			"archive":         artifact.ArchiveName,
			"filename":        artifact.Filename,
			"checksum":        artifact.Checksum,
			"size":            artifact.Size,
			"duration_millis": artifact.Duration.Round(time.Millisecond).Milliseconds(),
			"error":           artifact.Error,
		})
	}
	return out
}
