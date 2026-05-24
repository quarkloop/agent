package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	embeddingv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/embedding/v1"
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
	"github.com/quarkloop/pkg/serviceapi/servicebridge"
	"github.com/quarkloop/pkg/serviceapi/servicekit"
)

type server struct {
	embedder embedder
}

func Run(ctx context.Context, cfg Config) error {
	if cfg.Address == "" {
		cfg.Address = "127.0.0.1:7304"
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	embedder, err := newEmbedder(cfg)
	if err != nil {
		return err
	}

	embeddingServer := &server{embedder: embedder}

	skillPath, err := resolveSkillPath(cfg.SkillDir)
	if err != nil {
		return err
	}
	skill, err := servicekit.SkillFromFile("service-embedding", "1.0.0", skillPath)
	if err != nil {
		return err
	}
	descriptor := &servicev1.ServiceDescriptor{
		Name:    "embedding",
		Type:    "embedding",
		Version: "1.0.0",
		Address: cfg.Address,
		Rpcs: []*servicev1.RpcDescriptor{
			{Service: "quark.embedding.v1.EmbeddingService", Method: "Embed", Request: "quark.embedding.v1.EmbedRequest", Response: "quark.embedding.v1.EmbedResponse", Description: embedder.Description()},
		},
		Skills: []*servicev1.SkillDescriptor{skill},
	}
	cfg.Logger.Info("embedding service configured", "provider", embedder.Provider(), "model", embedder.Model(), "dimensions", embedder.Dimensions())
	cfg.NATS.Logger = cfg.Logger
	return servicebridge.RunNATSService(ctx, cfg.NATS, servicebridge.Binding{
		Descriptor: descriptor,
		Services: []servicebridge.RPCService{{
			Service:        "quark.embedding.v1.EmbeddingService",
			Implementation: embeddingServer,
		}},
	})
}

func (s *server) Embed(ctx context.Context, req *embeddingv1.EmbedRequest) (*embeddingv1.EmbedResponse, error) {
	cmd := embedCommandFromProto(req)
	hash := sha256.Sum256([]byte(cmd.Input))
	result, err := s.embedder.Embed(ctx, cmd.Input, cmd.Model, cmd.Dimensions)
	if err != nil {
		return nil, err
	}
	return embedResponseToProto(result, hex.EncodeToString(hash[:])), nil
}

func resolveSkillPath(skillDir string) (string, error) {
	if skillDir != "" {
		path := filepath.Join(skillDir, "SKILL.md")
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("find embedding skill at %s: %w", path, err)
		}
		return path, nil
	}
	for _, path := range []string{"SKILL.md", filepath.Join("plugins", "services", "embedding", "SKILL.md")} {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("embedding service SKILL.md not found; pass --skill-dir")
}
