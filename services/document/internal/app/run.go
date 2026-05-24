package app

import (
	"context"
	"fmt"
	"github.com/quarkloop/pkg/serviceapi/servicebridge"
	"github.com/quarkloop/pkg/serviceapi/servicekit"
	"github.com/quarkloop/services/document/internal/docsvc"
	"log/slog"
	"os"
	"path/filepath"
)

type Config struct {
	Address   string
	SkillDir  string
	PDFToText string
	NATS      servicebridge.NATSConfig
	Logger    *slog.Logger
}

func Run(ctx context.Context, cfg Config) error {
	if cfg.Address == "" {
		cfg.Address = "127.0.0.1:7307"
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	documentServer := docsvc.NewServer(docsvc.Config{
		PDFToText: cfg.PDFToText,
		Logger:    cfg.Logger,
	})

	skillPath, err := resolveSkillPath(cfg.SkillDir)
	if err != nil {
		return err
	}
	skill, err := servicekit.SkillFromFile("service-document", "1.0.0", skillPath)
	if err != nil {
		return err
	}
	descriptor := docsvc.Descriptor(cfg.Address, skill)
	cfg.NATS.Logger = cfg.Logger
	return servicebridge.RunNATSService(ctx, cfg.NATS, servicebridge.Binding{
		Descriptor: descriptor,
		Services: []servicebridge.RPCService{{
			Service:        "quark.document.v1.DocumentService",
			Implementation: documentServer,
		}},
	})
}

func resolveSkillPath(skillDir string) (string, error) {
	if skillDir != "" {
		path := filepath.Join(skillDir, "SKILL.md")
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("find document skill at %s: %w", path, err)
		}
		return path, nil
	}
	for _, path := range []string{"SKILL.md", filepath.Join("plugins", "services", "document", "SKILL.md"), filepath.Join("services", "document", "SKILL.md")} {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("document service SKILL.md not found; pass --skill-dir")
}
