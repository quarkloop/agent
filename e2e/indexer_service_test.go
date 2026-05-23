//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/quarkloop/e2e/utils"
	indexerv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/indexer/v1"
	"github.com/quarkloop/pkg/serviceapi/servicefunction"
	"github.com/quarkloop/supervisor/pkg/natshub"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func TestIndexerServiceWithRealDgraph(t *testing.T) {
	indexerAddr := reserveLoopbackAddress(t)
	env := utils.StartE2E(t, false, utils.StartOptions{
		DisableKnowledgeServices: true,
		Services:                 localServicePlugins("indexer"),
		SupervisorEnv: map[string]string{
			"QUARK_INDEXER_ADDR": indexerAddr,
		},
		BeforeRuntime: func(t *testing.T, setup utils.RuntimeSetup, bins utils.BuiltBinaries) {
			t.Helper()
			dgraphAddr := utils.StartDgraph(t)
			startIndexerServiceAt(t, bins.Indexer, dgraphAddr, indexerAddr, setup.NATS)
		},
	})

	conn, err := nats.Connect(
		env.NATS.ClientURL,
		nats.UserInfo(natshub.DefaultControlUser, natshub.DefaultControlPassword),
		nats.Name("quark-e2e-indexer-service"),
		nats.Timeout(5*time.Second),
	)
	if err != nil {
		t.Fatalf("connect control nats: %v", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var indexResp indexerv1.IndexStatus
	requestServiceFunction(t, ctx, conn, env.Space, "svc.indexer.v1.index_document", "indexer", "index_document", &indexerv1.IndexRequest{
		ChunkId:     "e2e-chunk-1",
		TextContent: "Quark service extraction uses NATS service functions and Dgraph vector indexes.",
		Embedding:   []float32{1, 0, 0},
		Entities: []*indexerv1.Entity{
			{Id: "quark", Name: "Quark", Type: "PROJECT"},
			{Id: "dgraph", Name: "Dgraph", Type: "DATABASE"},
		},
		Relations: []*indexerv1.Relation{
			{FromId: "quark", ToId: "dgraph", Relation: "USES"},
		},
		SourceMetadata: map[string]string{"source": "e2e", "tenant": "quark"},
	}, &indexResp)
	if !indexResp.GetSuccess() {
		t.Fatalf("index response failed: %+v", &indexResp)
	}

	var contextResp indexerv1.ContextResponse
	requestServiceFunction(t, ctx, conn, env.Space, "svc.indexer.v1.get_context", "indexer", "get_context", &indexerv1.QueryRequest{
		QueryVector: []float32{1, 0, 0},
		Limit:       5,
		Depth:       2,
		Filters:     map[string]string{"tenant": "quark"},
	}, &contextResp)
	if len(contextResp.GetChunks()) == 0 || contextResp.GetChunks()[0].GetId() != "e2e-chunk-1" {
		t.Fatalf("unexpected chunks: %+v", contextResp.GetChunks())
	}
	if !strings.Contains(contextResp.GetReasoningContext(), "Quark service extraction") {
		t.Fatalf("context missing indexed text: %q", contextResp.GetReasoningContext())
	}
	if !strings.Contains(contextResp.GetReasoningContext(), "USES") {
		t.Fatalf("context missing graph relation: %q", contextResp.GetReasoningContext())
	}
}

func requestServiceFunction(t *testing.T, ctx context.Context, conn *nats.Conn, spaceID, subject, service, function string, req proto.Message, resp proto.Message) {
	t.Helper()
	payload, err := protojson.MarshalOptions{UseProtoNames: false}.Marshal(req)
	if err != nil {
		t.Fatalf("marshal service request: %v", err)
	}
	envelope := servicefunction.RequestEnvelope{
		Version:  servicefunction.EnvelopeVersion,
		CallID:   fmt.Sprintf("e2e-%s-%d", function, time.Now().UnixNano()),
		SpaceID:  spaceID,
		Actor:    servicefunction.ActorRuntime,
		Service:  service,
		Function: function,
		Subject:  subject,
		Payload:  payload,
	}
	if err := envelope.Validate(); err != nil {
		t.Fatalf("validate service request: %v", err)
	}
	data, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("encode service request: %v", err)
	}
	reply, err := conn.RequestWithContext(ctx, subject, data)
	if err != nil {
		t.Fatalf("request %s: %v", subject, err)
	}
	var out servicefunction.ResponseEnvelope
	if err := json.Unmarshal(reply.Data, &out); err != nil {
		t.Fatalf("decode service response: %v", err)
	}
	if out.Status != servicefunction.StatusOK {
		t.Fatalf("service response failed: %+v", out.Error)
	}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(out.Payload, resp); err != nil {
		t.Fatalf("decode service payload: %v", err)
	}
}
