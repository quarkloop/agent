package services

import (
	citationv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/citation/v1"
	corev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/core/v1"
	devopsv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/devops/v1"
	documentv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/document/v1"
	gatewayv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/gateway/v1"
	harnessv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/harness/v1"
	indexerv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/indexer/v1"
	iov1 "github.com/quarkloop/pkg/serviceapi/gen/quark/io/v1"
	runstatev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/runstate/v1"
	spacev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/space/v1"
	systemv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/system/v1"
	"google.golang.org/protobuf/types/known/emptypb"
)

var _ = []any{
	indexerv1.File_quark_indexer_v1_indexer_proto,
	devopsv1.File_quark_devops_v1_devops_proto,
	corev1.File_quark_core_v1_core_proto,
	documentv1.File_quark_document_v1_document_proto,
	iov1.File_quark_io_v1_io_proto,
	runstatev1.File_quark_runstate_v1_runstate_proto,
	citationv1.File_quark_citation_v1_citation_proto,
	harnessv1.File_quark_harness_v1_harness_proto,
	gatewayv1.File_quark_gateway_v1_gateway_proto,
	spacev1.File_quark_space_v1_space_proto,
	systemv1.File_quark_system_v1_system_proto,
	emptypb.File_google_protobuf_empty_proto,
}
