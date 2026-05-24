package services

import (
	citationv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/citation/v1"
	corev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/core/v1"
	devopsv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/devops/v1"
	documentv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/document/v1"
	embeddingv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/embedding/v1"
	indexerv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/indexer/v1"
	ingestionv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/ingestion/v1"
	iov1 "github.com/quarkloop/pkg/serviceapi/gen/quark/io/v1"
	memoryv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/memory/v1"
	modelv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/model/v1"
	spacev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/space/v1"
	systemv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/system/v1"
	"google.golang.org/protobuf/types/known/emptypb"
)

var _ = []any{
	indexerv1.File_quark_indexer_v1_indexer_proto,
	embeddingv1.File_quark_embedding_v1_embedding_proto,
	devopsv1.File_quark_devops_v1_devops_proto,
	corev1.File_quark_core_v1_core_proto,
	documentv1.File_quark_document_v1_document_proto,
	iov1.File_quark_io_v1_io_proto,
	ingestionv1.File_quark_ingestion_v1_ingestion_proto,
	citationv1.File_quark_citation_v1_citation_proto,
	memoryv1.File_quark_memory_v1_memory_proto,
	modelv1.File_quark_model_v1_model_proto,
	spacev1.File_quark_space_v1_space_proto,
	systemv1.File_quark_system_v1_system_proto,
	emptypb.File_google_protobuf_empty_proto,
}
