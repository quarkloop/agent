package server

import (
	"context"
	"errors"
	"fmt"

	indexerv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/indexer/v1"
	"github.com/quarkloop/services/indexer/internal/indexing"
	"github.com/quarkloop/services/indexer/pkg/indexer"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
	indexerv1.UnimplementedIndexerServiceServer
	service *indexing.Service
}

func New(service *indexing.Service) (*Server, error) {
	if service == nil {
		return nil, fmt.Errorf("indexing service is required")
	}
	return &Server{service: service}, nil
}

func (s *Server) IndexDocument(ctx context.Context, req *indexerv1.IndexRequest) (*indexerv1.IndexStatus, error) {
	if err := s.service.IndexDocument(ctx, indexCommand(req)); err != nil {
		return nil, grpcError(err)
	}
	return &indexerv1.IndexStatus{Success: true, Message: "indexed"}, nil
}

func (s *Server) GetContext(ctx context.Context, req *indexerv1.QueryRequest) (*indexerv1.ContextResponse, error) {
	result, err := s.service.GetContext(ctx, contextQuery(req))
	if err != nil {
		return nil, grpcError(err)
	}
	return &indexerv1.ContextResponse{
		ReasoningContext: result.ReasoningContext,
		Citations:        result.Citations,
		Chunks:           toProtoChunks(result.Chunks),
		Graph:            toProtoGraph(result.Graph),
		ContextPackage:   toProtoContextPackage(result.Package),
	}, nil
}

func (s *Server) DeleteChunk(ctx context.Context, req *indexerv1.DeleteChunkRequest) (*indexerv1.DeleteChunkResponse, error) {
	if err := s.service.DeleteChunk(ctx, req.GetChunkId()); err != nil {
		return nil, grpcError(err)
	}
	return &indexerv1.DeleteChunkResponse{Deleted: true}, nil
}

func toProtoChunks(chunks []indexer.Chunk) []*indexerv1.Chunk {
	out := make([]*indexerv1.Chunk, 0, len(chunks))
	for _, chunk := range chunks {
		out = append(out, &indexerv1.Chunk{
			Id:                chunk.ID,
			Text:              chunk.Text,
			Score:             chunk.Score,
			Metadata:          cloneMap(chunk.Metadata),
			Document:          toProtoDocument(chunk.Document),
			EmbeddingMetadata: toProtoEmbeddingMetadata(chunk.EmbeddingMetadata),
			Facts:             toProtoFacts(chunk.Facts),
			Citations:         toProtoCitations(chunk.Citations),
			Provenance:        toProtoProvenance(chunk.Provenance),
		})
	}
	return out
}

func toProtoDocument(document indexer.Document) *indexerv1.Document {
	if document.ID == "" && document.Name == "" && document.Type == "" && document.SourceURI == "" && len(document.Metadata) == 0 {
		return nil
	}
	return &indexerv1.Document{
		Id:        document.ID,
		Name:      document.Name,
		Type:      document.Type,
		SourceUri: document.SourceURI,
		Metadata:  cloneMap(document.Metadata),
	}
}

func toProtoEmbeddingMetadata(embedding indexer.EmbeddingMetadata) *indexerv1.EmbeddingMetadata {
	if embedding.Provider == "" && embedding.Model == "" && embedding.Dimensions == 0 && embedding.ContentHash == "" && embedding.Version == "" {
		return nil
	}
	return &indexerv1.EmbeddingMetadata{
		Provider:    embedding.Provider,
		Model:       embedding.Model,
		Dimensions:  int32(embedding.Dimensions),
		ContentHash: embedding.ContentHash,
		Version:     embedding.Version,
	}
}

func toProtoFacts(facts []indexer.Fact) []*indexerv1.Fact {
	out := make([]*indexerv1.Fact, 0, len(facts))
	for _, fact := range facts {
		out = append(out, &indexerv1.Fact{
			Id:         fact.ID,
			Subject:    fact.Subject,
			Predicate:  fact.Predicate,
			Object:     fact.Object,
			Confidence: fact.Confidence,
			Citations:  toProtoCitations(fact.Citations),
			Metadata:   cloneMap(fact.Metadata),
		})
	}
	return out
}

func toProtoCitations(citations []indexer.Citation) []*indexerv1.Citation {
	out := make([]*indexerv1.Citation, 0, len(citations))
	for _, citation := range citations {
		out = append(out, &indexerv1.Citation{
			Id:          citation.ID,
			SourceUri:   citation.SourceURI,
			ChunkId:     citation.ChunkID,
			TextSpan:    citation.TextSpan,
			StartOffset: int32(citation.StartOffset),
			EndOffset:   int32(citation.EndOffset),
			Confidence:  citation.Confidence,
		})
	}
	return out
}

func toProtoProvenance(provenance indexer.Provenance) *indexerv1.Provenance {
	if provenance.SourceURI == "" && provenance.SourceHash == "" && provenance.IngestedAt == "" && provenance.ProducedBy == "" && provenance.TraceID == "" && len(provenance.Metadata) == 0 {
		return nil
	}
	return &indexerv1.Provenance{
		SourceUri:  provenance.SourceURI,
		SourceHash: provenance.SourceHash,
		IngestedAt: provenance.IngestedAt,
		ProducedBy: provenance.ProducedBy,
		TraceId:    provenance.TraceID,
		Metadata:   cloneMap(provenance.Metadata),
	}
}

func toProtoProvenanceList(provenance []indexer.Provenance) []*indexerv1.Provenance {
	out := make([]*indexerv1.Provenance, 0, len(provenance))
	for _, item := range provenance {
		if proto := toProtoProvenance(item); proto != nil {
			out = append(out, proto)
		}
	}
	return out
}

func toProtoContextPackage(pkg indexer.ContextPackage) *indexerv1.ContextPackage {
	if len(pkg.Chunks) == 0 && len(pkg.Facts) == 0 && len(pkg.Citations) == 0 && len(pkg.Provenance) == 0 && pkg.Graph == nil && pkg.Confidence == 0 {
		return nil
	}
	return &indexerv1.ContextPackage{
		Chunks:     toProtoChunks(pkg.Chunks),
		Facts:      toProtoFacts(pkg.Facts),
		Citations:  toProtoCitations(pkg.Citations),
		Provenance: toProtoProvenanceList(pkg.Provenance),
		Graph:      toProtoGraph(pkg.Graph),
		Confidence: pkg.Confidence,
	}
}

func toProtoGraph(graph *indexer.GraphFragment) *indexerv1.GraphFragment {
	if graph == nil {
		return nil
	}
	out := &indexerv1.GraphFragment{
		Nodes: make([]*indexerv1.GraphNode, 0, len(graph.Nodes)),
		Edges: make([]*indexerv1.GraphEdge, 0, len(graph.Edges)),
	}
	for _, node := range graph.Nodes {
		out.Nodes = append(out.Nodes, &indexerv1.GraphNode{Id: node.ID, Label: node.Label, Type: node.Type})
	}
	for _, edge := range graph.Edges {
		out.Edges = append(out.Edges, &indexerv1.GraphEdge{FromId: edge.FromID, ToId: edge.ToID, Relation: edge.Relation})
	}
	return out
}

func grpcError(err error) error {
	var validation *indexing.ValidationError
	if errors.As(err, &validation) {
		return status.Error(codes.InvalidArgument, validation.Error())
	}
	return status.Errorf(codes.Internal, "%v", err)
}
