package buildrelease

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync"
	"text/template"
	"time"
)

const (
	defaultConfigPath  = "build_release.json"
	defaultReleaseDir  = "dist"
	defaultParallelism = 4
)

//go:embed templates/*
var templateFS embed.FS

type Runner struct {
	Clock func() time.Time
}

func NewRunner() *Runner {
	return &Runner{Clock: time.Now}
}

func (r *Runner) Release(ctx context.Context, req ReleaseRequest) (*ReleaseResult, error) {
	pipeCtx, runCtx, err := r.pipelineContext(ctx, req.WorkingDir, req.ConfigPath, req.Version, req.Parallelism, req.SkipTests, false)
	if err != nil {
		return nil, err
	}
	pipeline := NewPipeline(
		LoadConfigStage{},
		ResolveVersionStage{},
		ValidateStage{},
		TestStage{},
		PrepareStage{},
		BuildStage{},
		InstallScriptStage{},
		ChecksumStage{},
		SignStage{},
		ReadmeStage{},
		MetadataStage{},
	)
	if err := pipeline.Run(pipeCtx, runCtx); err != nil {
		return nil, err
	}
	return &ReleaseResult{
		Success:    true,
		Message:    "release completed",
		Version:    pipeCtx.Version,
		ReleaseDir: pipeCtx.Config.ReleaseDir,
		Artifacts:  cloneArtifacts(pipeCtx.Artifacts),
	}, nil
}

func (r *Runner) DryRun(ctx context.Context, req DryRunRequest) (*DryRunResult, error) {
	pipeCtx, runCtx, err := r.pipelineContext(ctx, req.WorkingDir, req.ConfigPath, req.Version, req.Parallelism, false, true)
	if err != nil {
		return nil, err
	}
	pipeline := NewPipeline(
		LoadConfigStage{},
		ResolveVersionStage{},
		ValidateStage{},
		BuildStage{},
	)
	if err := pipeline.Run(pipeCtx, runCtx); err != nil {
		return nil, err
	}
	return &DryRunResult{Version: pipeCtx.Version, Planned: cloneArtifacts(pipeCtx.Artifacts)}, nil
}

func (r *Runner) Init(ctx context.Context, req InitRequest) (*InitResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	workingDir, err := normalizeWorkingDir(req.WorkingDir)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(workingDir, 0o755); err != nil {
		return nil, fmt.Errorf("create working dir: %w", err)
	}
	path := filepath.Join(workingDir, defaultConfigPath)
	if _, err := os.Stat(path); err == nil && !req.Overwrite {
		return &InitResult{ConfigPath: path, Created: false}, nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("stat config: %w", err)
	}

	cfg := DefaultConfig("mytool")
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return nil, fmt.Errorf("write config: %w", err)
	}
	return &InitResult{ConfigPath: path, Created: true}, nil
}

func (r *Runner) pipelineContext(ctx context.Context, workingDir, configPath, version string, parallelism int, skipTests, dryRun bool) (*PipelineContext, RunContext, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	wd, err := normalizeWorkingDir(workingDir)
	if err != nil {
		return nil, RunContext{}, err
	}
	if configPath == "" {
		configPath = defaultConfigPath
	}
	configPath = resolvePath(wd, configPath)
	if parallelism <= 0 {
		parallelism = defaultParallelism
	}
	clock := r.Clock
	if clock == nil {
		clock = time.Now
	}
	pipeCtx := &PipelineContext{
		Config:      DefaultConfig("mytool"),
		ConfigFile:  configPath,
		WorkingDir:  wd,
		DryRun:      dryRun,
		Parallelism: parallelism,
		SkipTests:   skipTests,
	}
	if version != "" {
		pipeCtx.Config.Version = version
	}
	return pipeCtx, RunContext{Context: ctx, Clock: clock}, nil
}

func DefaultConfig(packageName string) ReleaseConfig {
	return ReleaseConfig{
		PackageName: packageName,
		BinaryName:  packageName,
		ReleaseDir:  defaultReleaseDir,
		InstallDir:  "/usr/local/bin",
		LDFlags:     "-s -w",
		Checksums:   true,
		Builds: []BuildTarget{
			{Name: packageName, SourcePath: ".", BinaryName: packageName, SourceDir: "."},
		},
		Targets: []Target{
			{OS: "linux", Arch: "amd64"},
			{OS: "linux", Arch: "arm64"},
			{OS: "darwin", Arch: "amd64"},
			{OS: "darwin", Arch: "arm64"},
		},
	}
}

type LoadConfigStage struct{}

func (LoadConfigStage) Name() string { return "Load configuration" }

func (LoadConfigStage) Run(ctx *PipelineContext, _ RunContext) error {
	data, err := os.ReadFile(ctx.ConfigFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			ctx.Config = normalizeConfig(ctx.WorkingDir, ctx.Config)
			return nil
		}
		return fmt.Errorf("read config: %w", err)
	}
	cfg := ctx.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parse %s: %w", ctx.ConfigFile, err)
	}
	if ctx.Config.Version != "" {
		cfg.Version = ctx.Config.Version
	}
	ctx.Config = normalizeConfig(ctx.WorkingDir, cfg)
	return nil
}

type ResolveVersionStage struct{}

func (ResolveVersionStage) Name() string { return "Resolve version" }

func (ResolveVersionStage) Run(ctx *PipelineContext, runCtx RunContext) error {
	version := ctx.Config.Version
	if version == "" {
		out, err := commandOutput(runCtx.Context, ctx.WorkingDir, "git", "describe", "--tags", "--always", "--dirty")
		if err != nil {
			version = "dev"
		} else {
			version = strings.TrimSpace(string(out))
		}
	}
	ctx.Version = version

	commit, err := commandOutput(runCtx.Context, ctx.WorkingDir, "git", "rev-parse", "--short", "HEAD")
	if err != nil {
		ctx.Commit = "unknown"
	} else {
		ctx.Commit = strings.TrimSpace(string(commit))
	}

	ctx.BuildTime = runCtx.Clock().UTC().Format(time.RFC3339)
	ctx.GoVersion = goruntime.Version()
	return nil
}

type ValidateStage struct{}

func (ValidateStage) Name() string { return "Validate" }

func (ValidateStage) Run(ctx *PipelineContext, _ RunContext) error {
	if strings.TrimSpace(ctx.Config.PackageName) == "" {
		return errors.New("package_name is required")
	}
	if strings.TrimSpace(ctx.Config.ReleaseDir) == "" {
		return errors.New("release_dir is required")
	}
	if len(ctx.Config.Builds) == 0 {
		return errors.New("at least one build target is required")
	}
	if len(ctx.Config.Targets) == 0 {
		return errors.New("at least one target is required")
	}
	for _, build := range ctx.Config.Builds {
		if build.Name == "" {
			return errors.New("build name is required")
		}
		if build.SourcePath == "" {
			return fmt.Errorf("build %s source_path is required", build.Name)
		}
		if build.BinaryName == "" {
			return fmt.Errorf("build %s binary_name is required", build.Name)
		}
	}
	return nil
}

type TestStage struct{}

func (TestStage) Name() string { return "Test" }

func (TestStage) Run(ctx *PipelineContext, runCtx RunContext) error {
	if ctx.SkipTests {
		return nil
	}
	for _, build := range ctx.Config.Builds {
		cmd := exec.CommandContext(runCtx.Context, "go", "test", "./...")
		cmd.Dir = build.SourceDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("tests failed for %s: %w", build.Name, err)
		}
	}
	return nil
}

type PrepareStage struct{}

func (PrepareStage) Name() string { return "Prepare" }

func (PrepareStage) Run(ctx *PipelineContext, _ RunContext) error {
	if err := os.MkdirAll(ctx.Config.ReleaseDir, 0o755); err != nil {
		return fmt.Errorf("create release dir: %w", err)
	}
	return nil
}

type BuildStage struct{}

func (BuildStage) Name() string { return "Build" }

func (BuildStage) Run(ctx *PipelineContext, runCtx RunContext) error {
	semaphore := make(chan struct{}, ctx.Parallelism)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, build := range ctx.Config.Builds {
		for _, target := range ctx.Config.Targets {
			build := build
			target := target
			wg.Add(1)
			go func() {
				defer wg.Done()
				select {
				case semaphore <- struct{}{}:
					defer func() { <-semaphore }()
				case <-runCtx.Context.Done():
					mu.Lock()
					ctx.Artifacts = append(ctx.Artifacts, Artifact{BuildName: build.Name, Target: target, Error: runCtx.Context.Err().Error()})
					mu.Unlock()
					return
				}

				artifact := buildOne(ctx, runCtx, build, target)
				mu.Lock()
				ctx.Artifacts = append(ctx.Artifacts, artifact)
				mu.Unlock()
			}()
		}
	}
	wg.Wait()

	var errs []string
	for _, artifact := range ctx.Artifacts {
		if artifact.Error != "" {
			errs = append(errs, fmt.Sprintf("%s: %s", artifact.ArchiveName, artifact.Error))
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "\n"))
	}
	return nil
}

func buildOne(ctx *PipelineContext, runCtx RunContext, build BuildTarget, target Target) Artifact {
	archiveBase := fmt.Sprintf("%s_%s_%s", ctx.Config.PackageName, target.OS, target.Arch)
	if len(ctx.Config.Builds) > 1 {
		archiveBase = fmt.Sprintf("%s_%s_%s_%s", ctx.Config.PackageName, build.Name, target.OS, target.Arch)
	}
	if target.ARM != "" {
		archiveBase = fmt.Sprintf("%s_v%s", archiveBase, target.ARM)
	}
	archiveName := archiveBase + ".tar.gz"
	binName := build.BinaryName
	if target.OS == "windows" {
		binName += ".exe"
	}

	artifact := Artifact{BuildName: build.Name, Target: target, ArchiveName: archiveName}
	if ctx.DryRun {
		return artifact
	}

	ldflags := strings.TrimSpace(ctx.Config.LDFlags)
	if build.LDFlags != "" {
		ldflags = build.LDFlags
	}
	extra := build.ExtraLDFlags
	if extra == "" {
		extra = ctx.Config.ExtraLDFlags
	}
	if expanded := strings.TrimSpace(expandLDFlags(extra, ctx)); expanded != "" {
		ldflags = strings.TrimSpace(ldflags + " " + expanded)
	}

	env := os.Environ()
	env = append(env, "CGO_ENABLED=0")
	env = append(env, "GOOS="+target.OS)
	env = append(env, "GOARCH="+target.Arch)
	if target.ARM != "" {
		env = append(env, "GOARM="+target.ARM)
	}

	tmpDir, err := os.MkdirTemp("", "buildrelease-*")
	if err != nil {
		artifact.Error = fmt.Sprintf("mkdtemp: %v", err)
		return artifact
	}
	defer os.RemoveAll(tmpDir)

	binPath := filepath.Join(tmpDir, binName)
	args := []string{"build", "-buildvcs=false"}
	if ldflags != "" {
		args = append(args, "-ldflags", ldflags)
	}
	args = append(args, "-o", binPath, build.SourcePath)

	cmd := exec.CommandContext(runCtx.Context, "go", args...)
	cmd.Dir = build.SourceDir
	cmd.Env = env

	start := runCtx.Clock()
	out, err := cmd.CombinedOutput()
	artifact.Duration = runCtx.Clock().Sub(start)
	if err != nil {
		artifact.Error = fmt.Sprintf("go build: %v\n%s", err, string(out))
		return artifact
	}

	for _, include := range build.IncludeFiles {
		src := resolvePath(build.SourceDir, include)
		if _, err := os.Stat(src); err == nil {
			dst := filepath.Join(tmpDir, filepath.Base(include))
			if err := copyFile(src, dst); err != nil {
				artifact.Error = fmt.Sprintf("include %s: %v", include, err)
				return artifact
			}
		}
	}

	if ctx.Config.Compress {
		_ = exec.CommandContext(runCtx.Context, "upx", "-q", binPath).Run()
	}

	archivePath := filepath.Join(ctx.Config.ReleaseDir, archiveName)
	if err := createTarGz(tmpDir, archivePath); err != nil {
		artifact.Error = fmt.Sprintf("archive: %v", err)
		return artifact
	}
	info, _ := os.Stat(archivePath)
	if info != nil {
		artifact.Size = info.Size()
		artifact.Filename = archivePath
	}
	return artifact
}

type InstallScriptStage struct{}

func (InstallScriptStage) Name() string { return "Generate install script" }

func (InstallScriptStage) Run(ctx *PipelineContext, runCtx RunContext) error {
	if ctx.DryRun {
		return nil
	}
	repoOwner, repoName := splitRepo(ctx.Config.Homepage)
	data := map[string]any{
		"PackageName": ctx.Config.PackageName,
		"BinaryName":  ctx.Config.BinaryName,
		"Version":     ctx.Version,
		"InstallDir":  ctx.Config.InstallDir,
		"Homepage":    ctx.Config.Homepage,
		"RepoOwner":   repoOwner,
		"RepoName":    repoName,
		"Artifacts":   ctx.Artifacts,
		"Date":        runCtx.Clock().UTC().Format("2006-01-02"),
	}
	return executeTemplate("templates/install.sh.tmpl", filepath.Join(ctx.Config.ReleaseDir, "install.sh"), 0o755, data, nil)
}

type ChecksumStage struct{}

func (ChecksumStage) Name() string { return "Generate checksums" }

func (ChecksumStage) Run(ctx *PipelineContext, _ RunContext) error {
	if !ctx.Config.Checksums || ctx.DryRun {
		return nil
	}
	outPath := filepath.Join(ctx.Config.ReleaseDir, "checksums.txt")
	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer out.Close()

	for i := range ctx.Artifacts {
		artifact := &ctx.Artifacts[i]
		if artifact.Error != "" || artifact.Filename == "" {
			continue
		}
		sum, err := sha256File(artifact.Filename)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(out, "%s  %s\n", sum, artifact.ArchiveName); err != nil {
			return err
		}
		artifact.Checksum = sum
	}
	return nil
}

type SignStage struct{}

func (SignStage) Name() string { return "Sign artifacts" }

func (SignStage) Run(ctx *PipelineContext, runCtx RunContext) error {
	if !ctx.Config.Sign || ctx.Config.SignKey == "" || ctx.DryRun {
		return nil
	}
	checksumPath := filepath.Join(ctx.Config.ReleaseDir, "checksums.txt")
	cmd := exec.CommandContext(runCtx.Context, "gpg", "--detach-sign", "--armor", "-u", ctx.Config.SignKey, checksumPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

type ReadmeStage struct{}

func (ReadmeStage) Name() string { return "Generate release README" }

func (ReadmeStage) Run(ctx *PipelineContext, _ RunContext) error {
	if ctx.DryRun {
		return nil
	}
	data := map[string]any{
		"PackageName": ctx.Config.PackageName,
		"Version":     ctx.Version,
		"Description": ctx.Config.Description,
		"Homepage":    ctx.Config.Homepage,
		"License":     ctx.Config.License,
		"Author":      ctx.Config.Author,
		"Artifacts":   ctx.Artifacts,
	}
	funcs := template.FuncMap{"humanSize": humanSize}
	return executeTemplate("templates/readme.md.tmpl", filepath.Join(ctx.Config.ReleaseDir, "README.md"), 0o644, data, funcs)
}

type MetadataStage struct{}

func (MetadataStage) Name() string { return "Generate release metadata" }

func (MetadataStage) Run(ctx *PipelineContext, _ RunContext) error {
	if ctx.DryRun {
		return nil
	}
	meta := map[string]any{
		"package_name": ctx.Config.PackageName,
		"version":      ctx.Version,
		"commit":       ctx.Commit,
		"go_version":   ctx.GoVersion,
		"build_time":   ctx.BuildTime,
		"artifacts":    ctx.Artifacts,
	}
	out := filepath.Join(ctx.Config.ReleaseDir, "release.json")
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(out, append(data, '\n'), 0o644)
}

func normalizeConfig(workingDir string, cfg ReleaseConfig) ReleaseConfig {
	if cfg.PackageName == "" {
		cfg.PackageName = "mytool"
	}
	if cfg.BinaryName == "" {
		cfg.BinaryName = cfg.PackageName
	}
	if cfg.ReleaseDir == "" {
		cfg.ReleaseDir = defaultReleaseDir
	}
	cfg.ReleaseDir = resolvePath(workingDir, cfg.ReleaseDir)
	for i := range cfg.Builds {
		if cfg.Builds[i].BinaryName == "" {
			cfg.Builds[i].BinaryName = cfg.PackageName
		}
		if cfg.Builds[i].SourceDir == "" {
			cfg.Builds[i].SourceDir = "."
		}
		if cfg.Builds[i].SourcePath == "" {
			cfg.Builds[i].SourcePath = "."
		}
		cfg.Builds[i].SourceDir = resolvePath(workingDir, cfg.Builds[i].SourceDir)
	}
	return cfg
}

func normalizeWorkingDir(dir string) (string, error) {
	if dir == "" {
		dir = "."
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolve working dir: %w", err)
	}
	return abs, nil
}

func resolvePath(base, path string) string {
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(base, path)
}

func commandOutput(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	return cmd.Output()
}

func expandLDFlags(flags string, ctx *PipelineContext) string {
	repl := strings.NewReplacer(
		"{{.Version}}", ctx.Version,
		"{{.Commit}}", ctx.Commit,
		"{{.BuildTime}}", ctx.BuildTime,
		"{{.Date}}", ctx.BuildTime,
	)
	return repl.Replace(flags)
}

func createTarGz(srcDir, dstPath string) error {
	f, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer f.Close()

	gzw := gzip.NewWriter(f)
	defer gzw.Close()

	tw := tar.NewWriter(gzw)
	defer tw.Close()

	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		path := filepath.Join(srcDir, entry.Name())
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = entry.Name()
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if entry.IsDir() {
			continue
		}
		data, err := os.Open(path)
		if err != nil {
			return err
		}
		if _, err := io.Copy(tw, data); err != nil {
			data.Close()
			return err
		}
		data.Close()
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func splitRepo(homepage string) (string, string) {
	parts := strings.Split(strings.TrimPrefix(homepage, "https://github.com/"), "/")
	if len(parts) >= 2 {
		return parts[0], parts[1]
	}
	return "", ""
}

func executeTemplate(name, dst string, mode os.FileMode, data any, funcs template.FuncMap) error {
	raw, err := templateFS.ReadFile(name)
	if err != nil {
		return err
	}
	tmpl := template.New(filepath.Base(name))
	if funcs != nil {
		tmpl = tmpl.Funcs(funcs)
	}
	parsed, err := tmpl.Parse(string(raw))
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := parsed.Execute(&buf, data); err != nil {
		return err
	}
	return os.WriteFile(dst, buf.Bytes(), mode)
}

func humanSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(size)/float64(div), "KMGTPE"[exp])
}

func cloneArtifacts(in []Artifact) []Artifact {
	out := make([]Artifact, len(in))
	copy(out, in)
	return out
}
