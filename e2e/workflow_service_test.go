//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/quarkloop/e2e/utils"
	"github.com/quarkloop/pkg/natskit"
	workflowv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/workflow/v1"
	"github.com/quarkloop/supervisor/pkg/natshub"
)

func TestWorkflowServiceNATSContract(t *testing.T) {
	env := utils.StartE2E(t, false, utils.StartOptions{
		DisableKnowledgeServices: true,
		Services:                 localServicePlugins("workflow"),
	})
	conn, err := natskit.Connect(context.Background(), natskit.Config{
		URL: env.NATS.ClientURL, Username: natshub.DefaultControlUser,
		Password: natshub.DefaultControlPassword, Name: "quark-e2e-workflow", Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("connect NATS: %v", err)
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	workflowID := "e2e-empty-document-ingestion"
	var started workflowv1.StartWorkflowResponse
	requestServiceFunction(t, ctx, conn, env.Space, "workflow", "start", &workflowv1.StartWorkflowRequest{
		SpaceId:    env.Space,
		WorkflowId: workflowID,
		DocumentIngestion: &workflowv1.DocumentIngestionWorkflow{
			Title: "E2E empty ingestion contract verification",
		},
	}, &started)
	if started.GetWorkflow().GetWorkflowId() != workflowID || started.GetWorkflow().GetRunId() == "" {
		t.Fatalf("workflow start response missing identity: %+v", &started)
	}
	var described workflowv1.DescribeWorkflowResponse
	requestServiceFunction(t, ctx, conn, env.Space, "workflow", "describe", &workflowv1.DescribeWorkflowRequest{
		WorkflowId: workflowID,
		RunId:      started.GetWorkflow().GetRunId(),
	}, &described)
	if described.GetWorkflow().GetWorkflowId() != workflowID {
		t.Fatalf("workflow describe response does not identify requested execution: %+v", &described)
	}
}
