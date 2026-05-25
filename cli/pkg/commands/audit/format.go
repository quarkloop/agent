package auditcmd

import (
	"fmt"
	"io"
	"time"

	"github.com/quarkloop/pkg/serviceapi/clientcontract"
)

func writeAuditRecord(out io.Writer, record clientcontract.AuditRecord, jsonOutput bool) error {
	if jsonOutput {
		return writeJSON(out, record)
	}
	fmt.Fprintf(out, "Reference:  %s\n", record.ReferenceID)
	fmt.Fprintf(out, "Service:    %s.%s\n", record.Service, record.Function)
	fmt.Fprintf(out, "Status:     %s\n", record.Status)
	fmt.Fprintf(out, "Space:      %s\n", record.SpaceID)
	if record.SessionID != "" {
		fmt.Fprintf(out, "Session:    %s\n", record.SessionID)
	}
	if record.RunID != "" {
		fmt.Fprintf(out, "Run:        %s\n", record.RunID)
	}
	if record.TraceID != "" {
		fmt.Fprintf(out, "Trace:      %s\n", record.TraceID)
	}
	fmt.Fprintf(out, "Duration:   %d ms\n", record.DurationMillis)
	fmt.Fprintf(out, "Recorded:   %s\n", record.RecordedAt)
	fmt.Fprintf(out, "Expires:    %s\n", record.RetentionExpiresAt)
	return nil
}

func writeAuditPage(out io.Writer, page clientcontract.AuditListResponse, jsonOutput bool) error {
	if jsonOutput {
		return writeJSON(out, page)
	}
	if len(page.Records) == 0 {
		fmt.Fprintln(out, "No audit records.")
		return nil
	}
	fmt.Fprintf(out, "%-10s %-30s %-18s %-26s %-10s %s\n", "SEQ", "REFERENCE", "SERVICE", "FUNCTION", "STATUS", "RECORDED")
	for _, record := range page.Records {
		fmt.Fprintf(out, "%-10d %-30s %-18s %-26s %-10s %s\n",
			record.Sequence, record.ReferenceID, record.Service, record.Function, record.Status, record.RecordedAt)
	}
	if page.NextCursor > 0 {
		fmt.Fprintf(out, "Next cursor: %d\n", page.NextCursor)
	}
	return nil
}

func writeRetention(out io.Writer, retention clientcontract.AuditRetentionResponse, jsonOutput bool) error {
	if jsonOutput {
		return writeJSON(out, retention)
	}
	fmt.Fprintf(out, "Maximum age:      %s\n", time.Duration(retention.MaxAgeSeconds)*time.Second)
	fmt.Fprintf(out, "Maximum records:  %d\n", retention.MaxMessages)
	return nil
}
