package natsclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/quarkloop/pkg/natskit"
	"github.com/quarkloop/pkg/serviceapi/clientcontract"
)

func (c *Client) RuntimePlan(ctx context.Context, spaceID string) (clientcontract.RuntimePlanResponse, error) {
	return requestPayload[clientcontract.RuntimePlanResponse](ctx, c, clientcontract.SubjectRuntimePlanGet, spaceID, clientcontract.RuntimePlanRequest{SpaceID: spaceID})
}

func (c *Client) ApproveRuntimePlan(ctx context.Context, spaceID, planID string) (clientcontract.RuntimePlanResponse, error) {
	return requestPayload[clientcontract.RuntimePlanResponse](ctx, c, clientcontract.SubjectRuntimePlanApprove, spaceID, clientcontract.RuntimePlanRequest{
		SpaceID: spaceID,
		PlanID:  planID,
	})
}

func (c *Client) RejectRuntimePlan(ctx context.Context, spaceID, planID string) (clientcontract.RuntimePlanResponse, error) {
	return requestPayload[clientcontract.RuntimePlanResponse](ctx, c, clientcontract.SubjectRuntimePlanReject, spaceID, clientcontract.RuntimePlanRequest{
		SpaceID: spaceID,
		PlanID:  planID,
	})
}

func (c *Client) RuntimeActivity(ctx context.Context, spaceID string, limit int) ([]clientcontract.RuntimeActivityRecord, error) {
	resp, err := requestPayload[clientcontract.RuntimeActivityListResponse](ctx, c, clientcontract.SubjectRuntimeActivityList, spaceID, clientcontract.RuntimeActivityListRequest{
		SpaceID: spaceID,
		Limit:   limit,
	})
	if err != nil {
		return nil, err
	}
	return cloneActivityRecords(resp.Records), nil
}

func (c *Client) SubscribeRuntimeActivity(ctx context.Context) (<-chan clientcontract.RuntimeActivityRecord, <-chan error, func(), error) {
	if c == nil || c.client == nil {
		return nil, nil, nil, errors.New("nats client is not connected")
	}
	records := make(chan clientcontract.RuntimeActivityRecord, 64)
	errs := make(chan error, 8)
	sub, err := c.client.Subscribe(clientcontract.SubjectRuntimeActivityFeed, func(msg natskit.Message) {
		var record clientcontract.RuntimeActivityRecord
		if err := json.Unmarshal(msg.Data, &record); err != nil {
			notifySubscriptionError(errs, fmt.Errorf("decode runtime activity: %w", err))
			return
		}
		record.Data = append(json.RawMessage(nil), record.Data...)
		select {
		case records <- record:
		case <-ctx.Done():
		}
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("subscribe %s: %w", clientcontract.SubjectRuntimeActivityFeed, err)
	}
	stopOnce := make(chan struct{})
	stop := func() {
		select {
		case <-stopOnce:
			return
		default:
			close(stopOnce)
			_ = sub.Unsubscribe()
		}
	}
	go func() {
		select {
		case <-ctx.Done():
			stop()
		case <-stopOnce:
		}
	}()
	if err := c.flush(ctx); err != nil {
		stop()
		return nil, nil, nil, fmt.Errorf("flush runtime activity subscription: %w", err)
	}
	return records, errs, stop, nil
}

func cloneActivityRecords(in []clientcontract.RuntimeActivityRecord) []clientcontract.RuntimeActivityRecord {
	out := make([]clientcontract.RuntimeActivityRecord, 0, len(in))
	for _, record := range in {
		copyRecord := record
		copyRecord.Data = append(json.RawMessage(nil), record.Data...)
		out = append(out, copyRecord)
	}
	return out
}
