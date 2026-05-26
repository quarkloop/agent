package app

import (
	"context"
	"fmt"
	"github.com/quarkloop/pkg/natskit"
	"github.com/quarkloop/pkg/serviceapi/servicekit"
	"github.com/quarkloop/services/citation/internal/citationsvc"
	"log/slog"
	"os"
	"path/filepath"
)

type Config struct {
	SkillDir string
	NATS     natskit.Config
	Logger   *slog.Logger
}

func Run(ctx context.Context, cfg Config) error {
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
	descriptor := citationsvc.Descriptor(skill)
	cfg.NATS.Logger = cfg.Logger
	return natskit.RunRPCService(ctx, cfg.NATS, natskit.Binding{
		Descriptor: descriptor,
		Services: []natskit.RPCService{{
			Service:        "quark.citation.v1.CitationService",
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
