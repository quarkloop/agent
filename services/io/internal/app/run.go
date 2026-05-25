package app

import (
	"context"
	"fmt"
	"github.com/quarkloop/pkg/natskit"
	"github.com/quarkloop/pkg/serviceapi/servicekit"
	"github.com/quarkloop/services/io/internal/iosvc"
	"log/slog"
	"os"
	"path/filepath"
)

type Config struct {
	Address   string
	SkillDir  string
	PDFToText string
	NATS      natskit.Config
	Logger    *slog.Logger
}

func Run(ctx context.Context, cfg Config) error {
	if cfg.Address == "" {
		cfg.Address = "127.0.0.1:7310"
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	ioServer := iosvc.NewServer(iosvc.Config{
		PDFToText: cfg.PDFToText,
		Logger:    cfg.Logger,
	})

	skillPath, err := resolveSkillPath(cfg.SkillDir)
	if err != nil {
		return err
	}
	skill, err := servicekit.SkillFromFile("service-io", "1.0.0", skillPath)
	if err != nil {
		return err
	}
	descriptor := iosvc.Descriptor(cfg.Address, skill)
	cfg.NATS.Logger = cfg.Logger
	return natskit.RunRPCService(ctx, cfg.NATS, natskit.Binding{
		Descriptor: descriptor,
		Services: []natskit.RPCService{{
			Service:        "quark.io.v1.IOService",
			Implementation: ioServer,
		}},
	})
}

func resolveSkillPath(skillDir string) (string, error) {
	if skillDir != "" {
		path := filepath.Join(skillDir, "SKILL.md")
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("find io skill at %s: %w", path, err)
		}
		return path, nil
	}
	for _, path := range []string{"SKILL.md", filepath.Join("plugins", "services", "io", "SKILL.md"), filepath.Join("services", "io", "SKILL.md")} {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("io service SKILL.md not found; pass --skill-dir")
}
