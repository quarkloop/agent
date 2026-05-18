//go:build e2e

package e2e

import (
	"testing"
	"time"

	"github.com/quarkloop/e2e/utils"
)

const knowledgeMessageIdleTimeout = 90 * time.Second

func knowledgeIndexTraceOptions(label string, sourceCount int) utils.MessageTraceOptions {
	return utils.MessageTraceOptions{
		Label:          label,
		OverallTimeout: knowledgeIndexMessageTimeout(sourceCount),
		IdleTimeout:    knowledgeMessageIdleTimeout,
	}
}

func knowledgeQueryTraceOptions(label string) utils.MessageTraceOptions {
	return utils.MessageTraceOptions{
		Label:          label,
		OverallTimeout: 6 * time.Minute,
		IdleTimeout:    knowledgeMessageIdleTimeout,
	}
}

func knowledgeIndexMessageTimeout(sourceCount int) time.Duration {
	if sourceCount < 1 {
		sourceCount = 1
	}
	return 7*time.Minute + time.Duration(sourceCount)*time.Minute
}

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
