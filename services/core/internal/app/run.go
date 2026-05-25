package app

import (
	"context"
	"fmt"
	"github.com/quarkloop/pkg/natskit"
	"github.com/quarkloop/pkg/serviceapi/servicekit"
	"github.com/quarkloop/services/core/internal/server"
	"log/slog"
	"os"
	"path/filepath"
)

type Config struct {
	Address  string
	RootDir  string
	SkillDir string
	NATS     natskit.Config
	Logger   *slog.Logger
}

func Run(ctx context.Context, cfg Config) error {
	if cfg.Address == "" {
		cfg.Address = "127.0.0.1:7305"
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	root, err := resolveRoot(cfg.RootDir)
	if err != nil {
		return err
	}
	coreServer, err := server.New(root)
	if err != nil {
		return err
	}

	skillPath, err := resolveSkillPath(cfg.SkillDir)
	if err != nil {
		return err
	}
	skill, err := servicekit.SkillFromFile("service-core", "1.0.0", skillPath)
	if err != nil {
		return err
	}
	descriptor := server.Descriptor(cfg.Address, skill)
	cfg.Logger.Info("core service configured", "root", root)
	cfg.NATS.Logger = cfg.Logger
	return natskit.RunRPCService(ctx, cfg.NATS, natskit.Binding{
		Descriptor: descriptor,
		Services: []natskit.RPCService{{
			Service:        "quark.core.v1.CoreService",
			Implementation: coreServer,
		}},
	})
}

func resolveRoot(root string) (string, error) {
	if root == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("resolve config dir: %w", err)
		}
		root = filepath.Join(dir, "quarkloop", "core")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve core root: %w", err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return "", fmt.Errorf("create core root: %w", err)
	}
	return abs, nil
}

func resolveSkillPath(skillDir string) (string, error) {
	if skillDir != "" {
		path := filepath.Join(skillDir, "SKILL.md")
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("find core skill at %s: %w", path, err)
		}
		return path, nil
	}
	for _, path := range []string{"SKILL.md", filepath.Join("plugins", "services", "core", "SKILL.md"), filepath.Join("services", "core", "SKILL.md")} {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("core service SKILL.md not found; pass --skill-dir")
}
