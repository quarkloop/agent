package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	citationv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/citation/v1"
	"github.com/quarkloop/pkg/serviceapi/servicebridge"
	"github.com/quarkloop/pkg/serviceapi/servicekit"
	"github.com/quarkloop/services/citation/internal/citationsvc"
)

type Config struct {
	Address  string
	SkillDir string
	NATS     servicebridge.NATSConfig
	Logger   *slog.Logger
}

func Run(ctx context.Context, cfg Config) error {
	if cfg.Address == "" {
		cfg.Address = "127.0.0.1:7309"
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	server := citationsvc.NewServer()
	skillPath, err := resolveSkillPath(cfg.SkillDir)
	if err != nil {
		return err
	}
	skill, err := servicekit.SkillFromFile("service-citation", "1.0.0", skillPath)
	if err != nil {
		return err
	}
	descriptor := citationsvc.Descriptor(cfg.Address, skill)
	cfg.NATS.Logger = cfg.Logger
	return servicebridge.RunNATSService(ctx, cfg.NATS, servicebridge.Binding{
		Descriptor: descriptor,
		Services: []servicebridge.GRPCService{{
			Desc:           &citationv1.CitationService_ServiceDesc,
			Implementation: server,
		}},
	})
}

func resolveSkillPath(skillDir string) (string, error) {
	if skillDir != "" {
		path := filepath.Join(skillDir, "SKILL.md")
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("find citation skill at %s: %w", path, err)
		}
		return path, nil
	}
	for _, path := range []string{"SKILL.md", filepath.Join("plugins", "services", "citation", "SKILL.md"), filepath.Join("services", "citation", "SKILL.md")} {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("citation service SKILL.md not found; pass --skill-dir")
}
