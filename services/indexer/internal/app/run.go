package app

import (
	"context"
	"fmt"
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
	"github.com/quarkloop/pkg/serviceapi/servicebridge"
	"github.com/quarkloop/pkg/serviceapi/servicekit"
	"github.com/quarkloop/services/indexer/internal/indexing"
	"github.com/quarkloop/services/indexer/internal/server"
	"github.com/quarkloop/services/indexer/pkg/indexer"
	"log/slog"
	"os"
	"path/filepath"
)

type Config struct {
	Address  string
	Driver   indexer.GraphVectorDriver
	SkillDir string
	NATS     servicebridge.NATSConfig
	Logger   *slog.Logger
}

func Run(ctx context.Context, cfg Config) error {
	if cfg.Address == "" {
		cfg.Address = "127.0.0.1:7301"
	}
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
	descriptor := &servicev1.ServiceDescriptor{
		Name:    "indexer",
		Type:    "indexer",
		Version: "1.0.0",
		Address: cfg.Address,
		Rpcs: []*servicev1.RpcDescriptor{
			{Service: "quark.indexer.v1.IndexerService", Method: "UpsertDocument", Request: "quark.indexer.v1.UpsertDocumentRequest", Response: "quark.indexer.v1.IndexStatus", Description: "Upsert one canonical source document record."},
			{Service: "quark.indexer.v1.IndexerService", Method: "UpsertChunk", Request: "quark.indexer.v1.UpsertChunkRequest", Response: "quark.indexer.v1.IndexStatus", Description: "Upsert one canonical chunk with embedding metadata and provenance."},
			{Service: "quark.indexer.v1.IndexerService", Method: "UpsertFact", Request: "quark.indexer.v1.UpsertFactRequest", Response: "quark.indexer.v1.IndexStatus", Description: "Upsert one canonical fact record."},
			{Service: "quark.indexer.v1.IndexerService", Method: "UpsertEntity", Request: "quark.indexer.v1.UpsertEntityRequest", Response: "quark.indexer.v1.IndexStatus", Description: "Upsert one canonical entity record."},
			{Service: "quark.indexer.v1.IndexerService", Method: "UpsertRelation", Request: "quark.indexer.v1.UpsertRelationRequest", Response: "quark.indexer.v1.IndexStatus", Description: "Upsert one canonical relation record."},
			{Service: "quark.indexer.v1.IndexerService", Method: "UpsertCitation", Request: "quark.indexer.v1.UpsertCitationRequest", Response: "quark.indexer.v1.IndexStatus", Description: "Upsert one canonical citation record."},
			{Service: "quark.indexer.v1.IndexerService", Method: "IndexDocument", Request: "quark.indexer.v1.IndexRequest", Response: "quark.indexer.v1.IndexStatus", Description: "Persist one canonical index record: document, chunk, embedding metadata, graph data, facts, citations, and provenance."},
			{Service: "quark.indexer.v1.IndexerService", Method: "QueryContext", Request: "quark.indexer.v1.QueryRequest", Response: "quark.indexer.v1.ContextResponse", Description: "Retrieve vector and graph context for an agent-provided query embedding."},
			{Service: "quark.indexer.v1.IndexerService", Method: "GetContext", Request: "quark.indexer.v1.QueryRequest", Response: "quark.indexer.v1.ContextResponse", Description: "Retrieve vector and graph context for an agent-provided query embedding."},
			{Service: "quark.indexer.v1.IndexerService", Method: "DeleteDocument", Request: "quark.indexer.v1.DeleteDocumentRequest", Response: "quark.indexer.v1.DeleteDocumentResponse", Description: "Delete one indexed document and document-owned chunks."},
			{Service: "quark.indexer.v1.IndexerService", Method: "DeleteChunk", Request: "quark.indexer.v1.DeleteChunkRequest", Response: "quark.indexer.v1.DeleteChunkResponse", Description: "Delete one indexed chunk and its chunk-owned edges by canonical chunk ID."},
		},
		Skills: []*servicev1.SkillDescriptor{skill},
	}
	defer cfg.Driver.Close()
	cfg.NATS.Logger = cfg.Logger
	return servicebridge.RunNATSService(ctx, cfg.NATS, servicebridge.Binding{
		Descriptor: descriptor,
		Services: []servicebridge.RPCService{{
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
