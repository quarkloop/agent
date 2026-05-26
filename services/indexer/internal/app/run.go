package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/quarkloop/pkg/natskit"
	"github.com/quarkloop/pkg/serviceapi/servicekit"
	"github.com/quarkloop/services/indexer/internal/indexing"
	"github.com/quarkloop/services/indexer/internal/server"
	"github.com/quarkloop/services/indexer/pkg/indexer"
)

type Config struct {
	Driver   indexer.GraphVectorDriver
	SkillDir string
	NATS     natskit.Config
	Logger   *slog.Logger
}

func Run(ctx context.Context, cfg Config) error {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	indexingService, err := indexing.New(cfg.Driver)
	if err != nil {
		return err
	}
	indexerServer, err := server.New(indexingService)
	if err != nil {
		return err
	}

	skillPath, err := resolveSkillPath(cfg.SkillDir)
	if err != nil {
		return err
	}
	skill, err := servicekit.SkillFromFile("service-indexer", "1.0.0", skillPath)
	if err != nil {
		return err
	}
	descriptor := server.Descriptor(skill)
	defer cfg.Driver.Close()
	cfg.NATS.Logger = cfg.Logger
	return natskit.RunRPCService(ctx, cfg.NATS, natskit.Binding{
		Descriptor: descriptor,
		Services: []natskit.RPCService{{
			Service:        "quark.indexer.v1.IndexerService",
			Implementation: indexerServer,
		}},
	})
}

func resolveSkillPath(skillDir string) (string, error) {
	if skillDir != "" {
		path := filepath.Join(skillDir, "SKILL.md")
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("find indexer skill at %s: %w", path, err)
		}
		return path, nil
	}

	for _, path := range []string{"SKILL.md", filepath.Join("plugins", "services", "indexer", "SKILL.md"), filepath.Join("services", "indexer", "SKILL.md")} {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("indexer service SKILL.md not found; pass --skill-dir")
}
