package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	buildreleasev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/buildrelease/v1"
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
	"github.com/quarkloop/pkg/serviceapi/servicebridge"
	"github.com/quarkloop/pkg/serviceapi/servicekit"
	"github.com/quarkloop/services/build-release/internal/server"
	"github.com/quarkloop/services/build-release/pkg/buildrelease"
)

type Config struct {
	Address  string
	SkillDir string
	NATS     servicebridge.NATSConfig
	Logger   *slog.Logger
}

func Run(ctx context.Context, cfg Config) error {
	if cfg.Address == "" {
		cfg.Address = "127.0.0.1:7302"
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	releaseServer, err := server.New(buildrelease.NewRunner())
	if err != nil {
		return err
	}

	skillPath, err := resolveSkillPath(cfg.SkillDir)
	if err != nil {
		return err
	}
	skill, err := servicekit.SkillFromFile("service-build-release", "1.0.0", skillPath)
	if err != nil {
		return err
	}
	descriptor := &servicev1.ServiceDescriptor{
		Name:    "build-release",
		Type:    "build-release",
		Version: "1.0.0",
		Address: cfg.Address,
		Rpcs: []*servicev1.RpcDescriptor{
			{Service: buildreleasev1.BuildReleaseService_ServiceDesc.ServiceName, Method: "Release", Request: "quark.buildrelease.v1.ReleaseRequest", Response: "quark.buildrelease.v1.ReleaseResponse", Description: "Run the configured build and release pipeline."},
			{Service: buildreleasev1.BuildReleaseService_ServiceDesc.ServiceName, Method: "DryRun", Request: "quark.buildrelease.v1.DryRunRequest", Response: "quark.buildrelease.v1.DryRunResponse", Description: "Preview planned release artifacts without compiling."},
			{Service: buildreleasev1.BuildReleaseService_ServiceDesc.ServiceName, Method: "Init", Request: "quark.buildrelease.v1.InitRequest", Response: "quark.buildrelease.v1.InitResponse", Description: "Create a default build_release.json in a working directory."},
		},
		Skills: []*servicev1.SkillDescriptor{skill},
	}
	cfg.NATS.Logger = cfg.Logger
	return servicebridge.RunNATSService(ctx, cfg.NATS, servicebridge.Binding{
		Descriptor: descriptor,
		Services: []servicebridge.GRPCService{{
			Desc:           &buildreleasev1.BuildReleaseService_ServiceDesc,
			Implementation: releaseServer,
		}},
	})
}

func resolveSkillPath(skillDir string) (string, error) {
	if skillDir != "" {
		path := filepath.Join(skillDir, "SKILL.md")
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("find build-release skill at %s: %w", path, err)
		}
		return path, nil
	}
	for _, path := range []string{"SKILL.md", filepath.Join("plugins", "services", "build-release", "SKILL.md"), filepath.Join("services", "build-release", "SKILL.md")} {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("build-release service SKILL.md not found; pass --skill-dir")
}
