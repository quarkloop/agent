package server

import servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"

func Descriptor(address string, skill *servicev1.SkillDescriptor) *servicev1.ServiceDescriptor {
	const service = "quark.indexer.v1.IndexerService"
	return &servicev1.ServiceDescriptor{
		Name:    "indexer",
		Type:    "indexer",
		Version: "1.0.0",
		Address: address,
		Rpcs: []*servicev1.RpcDescriptor{
			rpc("indexer_UpsertDocument", "upsert_document", service, "UpsertDocument", "quark.indexer.v1.UpsertDocumentRequest", "quark.indexer.v1.IndexStatus", "Upsert one canonical source document record."),
			rpc("indexer_UpsertChunk", "upsert_chunk", service, "UpsertChunk", "quark.indexer.v1.UpsertChunkRequest", "quark.indexer.v1.IndexStatus", "Upsert one canonical chunk with embedding metadata and provenance."),
			rpc("indexer_UpsertFact", "upsert_fact", service, "UpsertFact", "quark.indexer.v1.UpsertFactRequest", "quark.indexer.v1.IndexStatus", "Upsert one canonical fact record."),
			rpc("indexer_UpsertEntity", "upsert_entity", service, "UpsertEntity", "quark.indexer.v1.UpsertEntityRequest", "quark.indexer.v1.IndexStatus", "Upsert one canonical entity record."),
			rpc("indexer_UpsertRelation", "upsert_relation", service, "UpsertRelation", "quark.indexer.v1.UpsertRelationRequest", "quark.indexer.v1.IndexStatus", "Upsert one canonical relation record."),
			rpc("indexer_UpsertCitation", "upsert_citation", service, "UpsertCitation", "quark.indexer.v1.UpsertCitationRequest", "quark.indexer.v1.IndexStatus", "Upsert one canonical citation record."),
			rpc("indexer_QueryContext", "query_context", service, "QueryContext", "quark.indexer.v1.QueryRequest", "quark.indexer.v1.ContextResponse", "Retrieve vector and graph context for an agent-provided query embedding."),
			rpc("indexer_DeleteDocument", "delete_document", service, "DeleteDocument", "quark.indexer.v1.DeleteDocumentRequest", "quark.indexer.v1.DeleteDocumentResponse", "Delete one indexed document and document-owned chunks."),
			rpc("indexer_DeleteChunk", "delete_chunk", service, "DeleteChunk", "quark.indexer.v1.DeleteChunkRequest", "quark.indexer.v1.DeleteChunkResponse", "Delete one indexed chunk and its chunk-owned edges by canonical chunk ID."),
		},
		Skills: []*servicev1.SkillDescriptor{skill},
	}
}

func rpc(functionName, function, service, method, request, response, description string) *servicev1.RpcDescriptor {
	return &servicev1.RpcDescriptor{
		Service:      service,
		Method:       method,
		Request:      request,
		Response:     response,
		Description:  description,
		Owner:        "indexer",
		FunctionName: functionName,
		Subject:      "svc.indexer.v1." + function,
	}
}
