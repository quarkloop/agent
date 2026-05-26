//go:build e2e

package e2e

import (
	"testing"
	"time"
)

func TestKnowledgeTraceOptionsAreCentralized(t *testing.T) {
	index := knowledgeIndexTraceOptions("index", 4)
	if index.OverallTimeout != 11*time.Minute || index.IdleTimeout != knowledgeMessageIdleTimeout {
		t.Fatalf("unexpected index trace options: %+v", index)
	}

	query := knowledgeQueryTraceOptions("query")
	if query.OverallTimeout != 6*time.Minute || query.IdleTimeout != knowledgeMessageIdleTimeout {
		t.Fatalf("unexpected query trace options: %+v", query)
	}
}
