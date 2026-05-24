package app

import (
	"context"
	"fmt"
	"github.com/quarkloop/pkg/serviceapi/servicebridge"
	"github.com/quarkloop/pkg/serviceapi/servicekit"
	"github.com/quarkloop/services/ingestion/internal/ingestionsvc"
	"log/slog"
	"os"
	"path/filepath"
)

type Config struct {
	Address  string
	RootDir  string
	SkillDir string
	NATS     servicebridge.NATSConfig
	Logger   *slog.Logger
}

func Run(ctx context.Context, cfg Config) error {
	if cfg.Address == "" {
		cfg.Address = "127.0.0.1:7308"
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	root, err := resolveRoot(cfg.RootDir)
	if err != nil {
		return err
	}
	server, err := ingestionsvc.New(root)
	if err != nil {
		return err
	}

	skillPath, err := resolveSkillPath(cfg.SkillDir)
	if err != nil {
		return err
	}
	skill, err := servicekit.SkillFromFile("service-ingestion", "1.0.0", skillPath)
	if err != nil {
		return err
	}
	descriptor := ingestionsvc.Descriptor(cfg.Address, skill)
	cfg.Logger.Info("ingestion service configured", "root", root)
	cfg.NATS.Logger = cfg.Logger
	return servicebridge.RunNATSService(ctx, cfg.NATS, servicebridge.Binding{
		Descriptor: descriptor,
		Services: []servicebridge.RPCService{{
			Service:        "quark.ingestion.v1.IngestionService",
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
		root = filepath.Join(dir, "quarkloop", "ingestion")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve ingestion root: %w", err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return "", fmt.Errorf("create ingestion root: %w", err)
	}
	return abs, nil
}

func resolveSkillPath(skillDir string) (string, error) {
	if skillDir != "" {
		path := filepath.Join(skillDir, "SKILL.md")
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("find ingestion skill at %s: %w", path, err)
		}
		return path, nil
	}
	for _, path := range []string{"SKILL.md", filepath.Join("plugins", "services", "ingestion", "SKILL.md"), filepath.Join("services", "ingestion", "SKILL.md")} {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("ingestion service SKILL.md not found; pass --skill-dir")
}
