package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	devopsv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/devops/v1"
	"github.com/quarkloop/pkg/serviceapi/servicebridge"
	"github.com/quarkloop/pkg/serviceapi/servicekit"
	"github.com/quarkloop/services/devops/internal/devopssvc"
)

type Config struct {
	Address  string
	SkillDir string
	NATS     servicebridge.NATSConfig
	Logger   *slog.Logger
}

func Run(ctx context.Context, cfg Config) error {
	if cfg.Address == "" {
		cfg.Address = "127.0.0.1:7310"
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	server := devopssvc.NewServer()

	skillPath, err := resolveSkillPath(cfg.SkillDir)
	if err != nil {
		return err
	}
	skill, err := servicekit.SkillFromFile("service-devops", "1.0.0", skillPath)
	if err != nil {
		return err
	}
	descriptor := devopssvc.Descriptor(cfg.Address, skill)
	cfg.NATS.Logger = cfg.Logger
	return servicebridge.RunNATSService(ctx, cfg.NATS, servicebridge.Binding{
		Descriptor: descriptor,
		Services: []servicebridge.RPCService{
			{Desc: &devopsv1.RepoService_ServiceDesc, Implementation: server},
			{Desc: &devopsv1.BuildService_ServiceDesc, Implementation: server},
			{Desc: &devopsv1.TestService_ServiceDesc, Implementation: server},
			{Desc: &devopsv1.ContainerService_ServiceDesc, Implementation: server},
			{Desc: &devopsv1.DeployService_ServiceDesc, Implementation: server},
			{Desc: &devopsv1.PolicyService_ServiceDesc, Implementation: server},
		},
	})
}

func resolveSkillPath(skillDir string) (string, error) {
	if skillDir != "" {
		path := filepath.Join(skillDir, "SKILL.md")
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("find devops skill at %s: %w", path, err)
		}
		return path, nil
	}
	for _, path := range []string{"SKILL.md", filepath.Join("plugins", "services", "devops", "SKILL.md"), filepath.Join("services", "devops", "SKILL.md")} {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("devops service SKILL.md not found; pass --skill-dir")
}
