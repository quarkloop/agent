//go:build e2e

package e2e

import (
	"time"

	"github.com/quarkloop/e2e/utils"
)

const (
	knowledgeMessageIdleTimeout = 90 * time.Second
	devOpsServiceFlowTimeout    = 5 * time.Minute
	systemServiceFlowTimeout    = 3 * time.Minute
)

func knowledgeIndexTraceOptions(label string, sourceCount int) utils.MessageTraceOptions {
	return utils.MessageTraceOptions{
		Label: label, OverallTimeout: knowledgeIndexMessageTimeout(sourceCount),
		IdleTimeout: knowledgeMessageIdleTimeout,
	}
}

func knowledgeQueryTraceOptions(label string) utils.MessageTraceOptions {
	return utils.MessageTraceOptions{
		Label: label, OverallTimeout: 6 * time.Minute, IdleTimeout: knowledgeMessageIdleTimeout,
	}
}

func knowledgeIndexMessageTimeout(sourceCount int) time.Duration {
	if sourceCount < 1 {
		sourceCount = 1
	}
	return 7*time.Minute + time.Duration(sourceCount)*time.Minute
}

func devOpsServiceTraceOptions(label string) utils.MessageTraceOptions {
	return utils.MessageTraceOptions{
		Label: label, OverallTimeout: devOpsServiceFlowTimeout, IdleTimeout: 90 * time.Second,
	}
}

func systemServiceTraceOptions(label string) utils.MessageTraceOptions {
	return utils.MessageTraceOptions{
		Label: label, OverallTimeout: systemServiceFlowTimeout, IdleTimeout: 90 * time.Second,
	}
}
