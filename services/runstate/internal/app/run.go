package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/quarkloop/pkg/natskit"
	"github.com/quarkloop/pkg/serviceapi/servicekit"
	"github.com/quarkloop/services/runstate/internal/runstatesvc"
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
	root, err := resolveRoot(cfg.RootDir)
	if err != nil {
		return err
	}
	leases, closeLeases, err := openLeaseStore(ctx, cfg.NATS)
	if err != nil {
		return err
	}
	defer closeLeases()
	server, err := runstatesvc.New(root, leases)
	if err != nil {
		return err
	}
	skillPath, err := resolveSkillPath(cfg.SkillDir)
	if err != nil {
		return err
	}
	skill, err := servicekit.SkillFromFile("service-runstate", "1.0.0", skillPath)
	if err != nil {
		return err
	}
	descriptor := runstatesvc.Descriptor(skill)
	cfg.Logger.Info("runstate service configured", "root", root)
	cfg.NATS.Logger = cfg.Logger
	return natskit.RunRPCService(ctx, cfg.NATS, natskit.Binding{
		Descriptor: descriptor,
		Services: []natskit.RPCService{{
			Service:        "quark.runstate.v1.RunStateService",
			Implementation: server,
		}},
	})
}

func resolveRoot(root string) (string, error) {
	if root == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("resolve config dir: %w", err)
		}
		root = filepath.Join(dir, "quarkloop", "runstate")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve runstate root: %w", err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return "", fmt.Errorf("create runstate root: %w", err)
	}
	return abs, nil
}

func resolveSkillPath(skillDir string) (string, error) {
	if skillDir != "" {
		path := filepath.Join(skillDir, "SKILL.md")
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("find runstate skill at %s: %w", path, err)
		}
		return path, nil
	}
	for _, path := range []string{"SKILL.md", filepath.Join("plugins", "services", "runstate", "SKILL.md"), filepath.Join("services", "runstate", "SKILL.md")} {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("runstate service SKILL.md not found; pass --skill-dir")
}
