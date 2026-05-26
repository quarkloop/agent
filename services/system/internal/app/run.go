package app

import (
	"context"
	"fmt"
	"github.com/quarkloop/pkg/natskit"
	"github.com/quarkloop/pkg/serviceapi/servicekit"
	"github.com/quarkloop/services/system/internal/systemsvc"
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
	server := systemsvc.NewServer()

	skillPath, err := resolveSkillPath(cfg.SkillDir)
	if err != nil {
		return err
	}
	skill, err := servicekit.SkillFromFile("service-system", "1.0.0", skillPath)
	if err != nil {
		return err
	}
	descriptor := systemsvc.Descriptor(skill)
	cfg.NATS.Logger = cfg.Logger
	return natskit.RunRPCService(ctx, cfg.NATS, natskit.Binding{
		Descriptor: descriptor,
		Services: []natskit.RPCService{{
			Service:        "quark.system.v1.SystemService",
			Implementation: server,
		}},
	})
}

func resolveSkillPath(skillDir string) (string, error) {
	if skillDir != "" {
		path := filepath.Join(skillDir, "SKILL.md")
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("find system skill at %s: %w", path, err)
		}
		return path, nil
	}
	for _, path := range []string{"SKILL.md", filepath.Join("plugins", "services", "system", "SKILL.md"), filepath.Join("services", "system", "SKILL.md")} {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("system service SKILL.md not found; pass --skill-dir")
}
