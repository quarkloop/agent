package nats

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/quarkloop/pkg/serviceapi/clientcontract"
	"github.com/quarkloop/runtime/pkg/activity"
)

func (c *Channel) planResponse() clientcontract.RuntimePlanResponse {
	now := time.Now().UTC()
	if c.plan == nil {
		return clientcontract.RuntimePlanResponse{
			Goal:      "No active plan",
			Status:    "idle",
			Complete:  true,
			Summary:   "No active work.",
			CreatedAt: now,
			UpdatedAt: now,
		}
	}
	steps := c.plan.GetSteps()
	resp := clientcontract.RuntimePlanResponse{
		Goal:      "Current runtime plan",
		Status:    mapPlanStatus(c.plan.GetStatus()),
		Steps:     make([]clientcontract.RuntimePlanStep, 0, len(steps)),
		Complete:  c.plan.GetStatus() == "completed" || c.plan.GetStatus() == "idle",
		Summary:   c.plan.GetSummary(),
		CreatedAt: now,
		UpdatedAt: now,
	}
	for index, step := range steps {
		resp.Steps = append(resp.Steps, clientcontract.RuntimePlanStep{
			ID:          fmt.Sprintf("step-%d", index+1),
			Agent:       "main",
			Description: step.Description(),
			Status:      mapStepStatus(step.Status()),
			Result:      step.Result(),
		})
	}
	return resp
}

func mapPlanStatus(status string) string {
	switch status {
	case "active":
		return "executing"
	case "paused":
		return "draft"
	case "completed":
		return "succeeded"
	case "failed":
		return "failed"
	default:
		return status
	}
}

func mapStepStatus(status string) string {
	switch status {
	case "active":
		return "running"
	case "completed":
		return "complete"
	default:
		return status
	}
}

func mapActivityRecords(records []activity.Record) []clientcontract.RuntimeActivityRecord {
	out := make([]clientcontract.RuntimeActivityRecord, 0, len(records))
	for _, record := range records {
		out = append(out, mapActivityRecord(record))
	}
	return out
}

func mapActivityRecord(record activity.Record) clientcontract.RuntimeActivityRecord {
	return clientcontract.RuntimeActivityRecord{
		ID:        record.ID,
		SessionID: record.SessionID,
		Type:      record.Type,
		Timestamp: record.Timestamp,
		Data:      append(json.RawMessage(nil), record.Data...),
	}
}
