package server

import (
	"github.com/quarkloop/pkg/natskit"
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
)

func Descriptor(address string, skill *servicev1.SkillDescriptor) *servicev1.ServiceDescriptor {
	const service = "quark.indexer.v1.IndexerService"
	return &servicev1.ServiceDescriptor{
		Name:    "indexer",
		Type:    "indexer",
		Version: "1.0.0",
		Address: address,
		Rpcs: []*servicev1.RpcDescriptor{
			rpc("indexer_UpsertDocument", service, "UpsertDocument", "quark.indexer.v1.UpsertDocumentRequest", "quark.indexer.v1.IndexStatus", "Upsert one canonical source document record."),
			rpc("indexer_UpsertChunk", service, "UpsertChunk", "quark.indexer.v1.UpsertChunkRequest", "quark.indexer.v1.IndexStatus", "Upsert one canonical chunk with embedding metadata and provenance."),
			rpc("indexer_UpsertFact", service, "UpsertFact", "quark.indexer.v1.UpsertFactRequest", "quark.indexer.v1.IndexStatus", "Upsert one canonical fact record."),
			rpc("indexer_UpsertEntity", service, "UpsertEntity", "quark.indexer.v1.UpsertEntityRequest", "quark.indexer.v1.IndexStatus", "Upsert one canonical entity record."),
			rpc("indexer_UpsertRelation", service, "UpsertRelation", "quark.indexer.v1.UpsertRelationRequest", "quark.indexer.v1.IndexStatus", "Upsert one canonical relation record."),
			rpc("indexer_UpsertCitation", service, "UpsertCitation", "quark.indexer.v1.UpsertCitationRequest", "quark.indexer.v1.IndexStatus", "Upsert one canonical citation record."),
			rpc("indexer_QueryContext", service, "QueryContext", "quark.indexer.v1.QueryRequest", "quark.indexer.v1.ContextResponse", "Retrieve vector and graph context for an agent-provided query embedding."),
			rpc("indexer_DeleteDocument", service, "DeleteDocument", "quark.indexer.v1.DeleteDocumentRequest", "quark.indexer.v1.DeleteDocumentResponse", "Delete one indexed document and document-owned chunks."),
			rpc("indexer_DeleteChunk", service, "DeleteChunk", "quark.indexer.v1.DeleteChunkRequest", "quark.indexer.v1.DeleteChunkResponse", "Delete one indexed chunk and its chunk-owned edges by canonical chunk ID."),
		},
		Skills: []*servicev1.SkillDescriptor{skill},
	}
}

func rpc(functionName, service, method, request, response, description string) *servicev1.RpcDescriptor {
	return natskit.MustServiceRPC("indexer", functionName, service, method, request, response, description)
}
