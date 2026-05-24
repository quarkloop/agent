package buildrelease

import (
	"context"
	"time"
)

type BuildTarget struct {
	Name         string   `json:"name"`
	SourcePath   string   `json:"source_path"`
	BinaryName   string   `json:"binary_name"`
	SourceDir    string   `json:"source_dir"`
	LDFlags      string   `json:"ldflags"`
	ExtraLDFlags string   `json:"extra_ldflags"`
	IncludeFiles []string `json:"include_files,omitempty"`
}

type Target struct {
	OS   string `json:"os"`
	Arch string `json:"arch"`
	ARM  string `json:"arm,omitempty"`
}

type Artifact struct {
	BuildName   string        `json:"build_name"`
	Target      Target        `json:"target"`
	Filename    string        `json:"filename,omitempty"`
	ArchiveName string        `json:"archive_name"`
	Checksum    string        `json:"checksum,omitempty"`
	Size        int64         `json:"size,omitempty"`
	Duration    time.Duration `json:"duration,omitempty"`
	Error       string        `json:"error,omitempty"`
}

type ReleaseConfig struct {
	PackageName  string        `json:"package_name"`
	Version      string        `json:"version"`
	BinaryName   string        `json:"binary_name"`
	Description  string        `json:"description"`
	Homepage     string        `json:"homepage"`
	License      string        `json:"license"`
	Author       string        `json:"author"`
	InstallDir   string        `json:"install_dir"`
	ReleaseDir   string        `json:"release_dir"`
	LDFlags      string        `json:"ldflags"`
	ExtraLDFlags string        `json:"extra_ldflags"`
	Compress     bool          `json:"compress"`
	Checksums    bool          `json:"checksums"`
	Sign         bool          `json:"sign"`
	SignKey      string        `json:"sign_key"`
	Builds       []BuildTarget `json:"builds"`
	Targets      []Target      `json:"targets"`
}

type ReleaseRequest struct {
	WorkingDir  string
	ConfigPath  string
	Version     string
	Parallelism int
	SkipTests   bool
}

type DryRunRequest struct {
	WorkingDir  string
	ConfigPath  string
	Version     string
	Parallelism int
}

type InitRequest struct {
	WorkingDir string
	Overwrite  bool
}

type ReleaseResult struct {
	Success    bool
	Message    string
	Version    string
	ReleaseDir string
	Artifacts  []Artifact
}

type DryRunResult struct {
	Version string
	Planned []Artifact
}

type InitResult struct {
	ConfigPath string
	Created    bool
}

type PipelineContext struct {
	Config      ReleaseConfig
	ConfigFile  string
	WorkingDir  string
	Version     string
	Commit      string
	BuildTime   string
	GoVersion   string
	Artifacts   []Artifact
	DryRun      bool
	Parallelism int
	SkipTests   bool
}

type Stage interface {
	Name() string
	Run(ctx *PipelineContext, runCtx RunContext) error
}

type RunContext struct {
	Context context.Context
	Clock   func() time.Time
}

type Pipeline struct {
	stages []Stage
}

func NewPipeline(stages ...Stage) *Pipeline {
	return &Pipeline{stages: stages}
}

func (p *Pipeline) Run(ctx *PipelineContext, runCtx RunContext) error {
	for _, s := range p.stages {
		if runCtx.Context != nil {
			if err := runCtx.Context.Err(); err != nil {
				return err
			}
		}
		if err := s.Run(ctx, runCtx); err != nil {
			return err
		}
	}
	return nil
}
