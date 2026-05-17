package app

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"

	indexerv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/indexer/v1"
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
	"github.com/quarkloop/pkg/serviceapi/servicekit"
	"github.com/quarkloop/services/indexer/internal/indexing"
	"github.com/quarkloop/services/indexer/internal/server"
	"github.com/quarkloop/services/indexer/pkg/indexer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

type Config struct {
	Address  string
	Driver   indexer.GraphVectorDriver
	SkillDir string
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

	grpcServer := grpc.NewServer(grpc.UnaryInterceptor(servicekit.UnaryLoggingInterceptor(cfg.Logger)))
	indexerv1.RegisterIndexerServiceServer(grpcServer, indexerServer)

	healthServer := health.NewServer()
	healthServer.SetServingStatus(indexerv1.IndexerService_ServiceDesc.ServiceName, healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(grpcServer, healthServer)

	registry := servicekit.NewRegistry()
	skillPath, err := resolveSkillPath(cfg.SkillDir)
	if err != nil {
		return err
	}
	skill, err := servicekit.SkillFromFile("service-indexer", "1.0.0", skillPath)
	if err != nil {
		return err
	}
	if err := registry.Register(&servicev1.ServiceDescriptor{
		Name:    "indexer",
		Type:    "indexer",
		Version: "1.0.0",
		Address: cfg.Address,
		Rpcs: []*servicev1.RpcDescriptor{
			{Service: indexerv1.IndexerService_ServiceDesc.ServiceName, Method: "UpsertDocument", Request: "quark.indexer.v1.UpsertDocumentRequest", Response: "quark.indexer.v1.IndexStatus", Description: "Upsert one canonical source document record."},
			{Service: indexerv1.IndexerService_ServiceDesc.ServiceName, Method: "UpsertChunk", Request: "quark.indexer.v1.UpsertChunkRequest", Response: "quark.indexer.v1.IndexStatus", Description: "Upsert one canonical chunk with embedding metadata and provenance."},
			{Service: indexerv1.IndexerService_ServiceDesc.ServiceName, Method: "UpsertFact", Request: "quark.indexer.v1.UpsertFactRequest", Response: "quark.indexer.v1.IndexStatus", Description: "Upsert one canonical fact record."},
			{Service: indexerv1.IndexerService_ServiceDesc.ServiceName, Method: "UpsertEntity", Request: "quark.indexer.v1.UpsertEntityRequest", Response: "quark.indexer.v1.IndexStatus", Description: "Upsert one canonical entity record."},
			{Service: indexerv1.IndexerService_ServiceDesc.ServiceName, Method: "UpsertRelation", Request: "quark.indexer.v1.UpsertRelationRequest", Response: "quark.indexer.v1.IndexStatus", Description: "Upsert one canonical relation record."},
			{Service: indexerv1.IndexerService_ServiceDesc.ServiceName, Method: "UpsertCitation", Request: "quark.indexer.v1.UpsertCitationRequest", Response: "quark.indexer.v1.IndexStatus", Description: "Upsert one canonical citation record."},
			{Service: indexerv1.IndexerService_ServiceDesc.ServiceName, Method: "IndexDocument", Request: "quark.indexer.v1.IndexRequest", Response: "quark.indexer.v1.IndexStatus", Description: "Persist one canonical index record: document, chunk, embedding metadata, graph data, facts, citations, and provenance."},
			{Service: indexerv1.IndexerService_ServiceDesc.ServiceName, Method: "QueryContext", Request: "quark.indexer.v1.QueryRequest", Response: "quark.indexer.v1.ContextResponse", Description: "Retrieve vector and graph context for an agent-provided query embedding."},
			{Service: indexerv1.IndexerService_ServiceDesc.ServiceName, Method: "GetContext", Request: "quark.indexer.v1.QueryRequest", Response: "quark.indexer.v1.ContextResponse", Description: "Retrieve vector and graph context for an agent-provided query embedding."},
			{Service: indexerv1.IndexerService_ServiceDesc.ServiceName, Method: "DeleteDocument", Request: "quark.indexer.v1.DeleteDocumentRequest", Response: "quark.indexer.v1.DeleteDocumentResponse", Description: "Delete one indexed document and document-owned chunks."},
			{Service: indexerv1.IndexerService_ServiceDesc.ServiceName, Method: "DeleteChunk", Request: "quark.indexer.v1.DeleteChunkRequest", Response: "quark.indexer.v1.DeleteChunkResponse", Description: "Delete one indexed chunk and its chunk-owned edges by canonical chunk ID."},
		},
		Skills: []*servicev1.SkillDescriptor{skill},
	}); err != nil {
		return err
	}
	servicev1.RegisterServiceRegistryServer(grpcServer, registry)

	ln, err := net.Listen("tcp", cfg.Address)
	if err != nil {
		return fmt.Errorf("listen %s: %w", cfg.Address, err)
	}
	errCh := make(chan error, 1)
	go func() {
		cfg.Logger.Info("indexer service listening", "addr", cfg.Address)
		errCh <- grpcServer.Serve(ln)
	}()

	select {
	case <-ctx.Done():
		grpcServer.GracefulStop()
		return cfg.Driver.Close()
	case err := <-errCh:
		_ = cfg.Driver.Close()
		return err
	}
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
