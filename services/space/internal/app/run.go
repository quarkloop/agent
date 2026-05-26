package app

import (
	"context"
	"fmt"
	"github.com/quarkloop/pkg/natskit"
	"github.com/quarkloop/pkg/serviceapi/servicekit"
	"github.com/quarkloop/services/space/pkg/spacesvc"
	"log/slog"
	"os"
	"path/filepath"
)

type Config struct {
	RootDir  string
	SkillDir string
	NATS     natskit.Config
	Logger   *slog.Logger
}

func Run(ctx context.Context, cfg Config) error {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	store, err := spacesvc.NewStore(cfg.RootDir)
	if err != nil {
		return err
	}
	server, err := spacesvc.NewServer(store)
	if err != nil {
		return err
	}

	skillPath, err := resolveSkillPath(cfg.SkillDir)
	if err != nil {
		return err
	}
	skill, err := servicekit.SkillFromFile("service-space", "1.0.0", skillPath)
	if err != nil {
		return err
	}
	descriptor := spacesvc.Descriptor(skill)
	cfg.Logger.Info("space service configured", "root", store.Root())
	cfg.NATS.Logger = cfg.Logger
	return natskit.RunRPCService(ctx, cfg.NATS, natskit.Binding{
		Descriptor: descriptor,
		Services: []natskit.RPCService{{
			Service:        "quark.space.v1.SpaceService",
			Implementation: server,
		}},
	})
}

func resolveSkillPath(skillDir string) (string, error) {
	if skillDir != "" {
		path := filepath.Join(skillDir, "SKILL.md")
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("find space skill at %s: %w", path, err)
		}
		return path, nil
	}
	for _, path := range []string{"SKILL.md", filepath.Join("plugins", "services", "space", "SKILL.md"), filepath.Join("services", "space", "SKILL.md")} {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("space service SKILL.md not found; pass --skill-dir")
}
