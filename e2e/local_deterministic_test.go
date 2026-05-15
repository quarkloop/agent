//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/quarkloop/e2e/utils"
	"github.com/quarkloop/supervisor/pkg/api"
)

func TestLocalDeterministicSupervisorRuntimeAndServices(t *testing.T) {
	indexerAddr := fmt.Sprintf("127.0.0.1:%d", utils.ReservePort(t))
	embeddingAddr := fmt.Sprintf("127.0.0.1:%d", utils.ReservePort(t))
	embedding := utils.EmbeddingOptions{
		Plugin:     "embedding",
		Mode:       "local",
		Provider:   "local",
		Model:      "local-hash-v1",
		Dimensions: 32,
	}

	env := utils.StartE2E(t, false, utils.StartOptions{
		Embedding: embedding,
		SupervisorEnv: map[string]string{
			"QUARK_INDEXER_ADDR":   indexerAddr,
			"QUARK_EMBEDDING_ADDR": embeddingAddr,
		},
		BeforeRuntime: func(t *testing.T, setup utils.RuntimeSetup, bins utils.BuiltBinaries) {
			t.Helper()
			dgraphAddr := utils.StartDgraph(t)
			startIndexerServiceAt(t, bins.Indexer, dgraphAddr, indexerAddr)
			startEmbeddingServiceAt(t, bins.Embedding, embeddingAddr, embedding)
		},
	})

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
