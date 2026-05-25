package auditcmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/quarkloop/pkg/serviceapi/clientcontract"
)

func TestWriteAuditRecordHumanOutputDoesNotRenderSnapshots(t *testing.T) {
	record := clientcontract.AuditRecord{
		ReferenceID: "ref-1", SpaceID: "docs", Service: "indexer", Function: "query_context",
		Status: "ok",
	}
	var out bytes.Buffer
	if err := writeAuditRecord(&out, record, false); err != nil {
		t.Fatalf("write record: %v", err)
	}
	if strings.Contains(out.String(), "snapshot") || !strings.Contains(out.String(), "ref-1") {
		t.Fatalf("human output = %s", out.String())
	}
}

func TestWriteAuditPageIncludesCursor(t *testing.T) {
	var out bytes.Buffer
	err := writeAuditPage(&out, clientcontract.AuditListResponse{
		Records:    []clientcontract.AuditRecord{{Sequence: 4, ReferenceID: "ref-4", Service: "gateway", Function: "generate", Status: "ok"}},
		NextCursor: 4,
	}, false)
	if err != nil {
		t.Fatalf("write page: %v", err)
	}
	if !strings.Contains(out.String(), "ref-4") || !strings.Contains(out.String(), "Next cursor: 4") {
		t.Fatalf("page output = %s", out.String())
	}
}

func TestWriteAuditRecordJSONContainsOnlyPublicAuditMetadata(t *testing.T) {
	var out bytes.Buffer
	err := writeAuditRecord(&out, clientcontract.AuditRecord{
		ReferenceID: "ref-json",
		Service:     "gateway",
	}, true)
	if err != nil {
		t.Fatalf("write json: %v", err)
	}
	if !strings.Contains(out.String(), `"ref-json"`) || strings.Contains(out.String(), "snapshot") {
		t.Fatalf("json output = %s", out.String())
	}
}
