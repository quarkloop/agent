//go:build e2e

package e2e

import (
	"testing"
	"time"

	"github.com/quarkloop/e2e/utils"
)

func TestLocalDeterministicSupervisorRuntimeAndServices(t *testing.T) {
	embedding := utils.EmbeddingOptions{
		Plugin:     "embedding",
		Mode:       "local",
		Provider:   "local",
		Model:      "local-hash-v1",
		Dimensions: 32,
	}

	env := utils.StartE2E(t, false, standardKnowledgeServicesStartOptions(t, embedding, ""))

	session := utils.CreateChatSession(t, env, "local-deterministic-startup")
	utils.WaitForAgentSession(t, env, session.ID, 10*time.Second)
	if got := utils.AgentSessionsCount(t, env); got == 0 {
		t.Fatal("expected runtime to report at least one mirrored session")
	}
}
