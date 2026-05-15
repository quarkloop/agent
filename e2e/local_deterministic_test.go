//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/quarkloop/e2e/utils"
	"github.com/quarkloop/supervisor/pkg/api"
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

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	session, err := env.Sup.CreateSession(ctx, env.Space, api.CreateSessionRequest{
		Type:  api.SessionTypeChat,
		Title: "local-deterministic-startup",
	})
	if err != nil {
		t.Fatalf("create local deterministic session: %v", err)
	}
	utils.WaitForAgentSession(t, env, session.ID, 10*time.Second)
	if got := utils.AgentSessionsCount(t, env); got == 0 {
		t.Fatal("expected runtime to report at least one mirrored session")
	}
}
