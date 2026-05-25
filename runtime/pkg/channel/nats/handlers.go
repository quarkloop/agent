package nats

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/quarkloop/pkg/boundary"
	"github.com/quarkloop/pkg/natskit"
	"github.com/quarkloop/pkg/serviceapi/clientcontract"
)

func (c *Channel) handleInput(msg natskit.Message) clientcontract.ResponseEnvelope {
	req, ok := decodeRequest(msg)
	if !ok {
		return clientcontract.Error("unknown", string(boundary.InvalidArgument), "invalid request envelope")
	}
	var payload clientcontract.SendMessageRequest
	if err := req.DecodePayload(&payload); err != nil {
		return clientcontract.Error(req.RequestID, string(boundary.InvalidArgument), err.Error())
	}
	if err := validateSendMessage(payload); err != nil {
		return clientcontract.Error(req.RequestID, string(boundary.InvalidArgument), err.Error())
	}
	c.sessions.GetOrCreate(payload.SessionID, string(clientcontract.SessionTypeChat), "")
	ack, err := clientcontract.OK(req.RequestID, clientcontract.SendMessageResponse{SessionID: payload.SessionID, Accepted: true})
	if err != nil {
		return clientcontract.Error(req.RequestID, string(boundary.Internal), err.Error())
	}
	go c.postAndStream(c.requestContext(), req, payload)
	return ack
}

func (c *Channel) handleInfoGet(msg natskit.Message) clientcontract.ResponseEnvelope {
	req, ok := decodeRequest(msg)
	if !ok {
		return clientcontract.Error("unknown", string(boundary.InvalidArgument), "invalid request envelope")
	}
	var payload clientcontract.RuntimeInfoRequest
	if err := req.DecodePayload(&payload); err != nil {
		return clientcontract.Error(req.RequestID, string(boundary.InvalidArgument), err.Error())
	}
	if strings.TrimSpace(payload.SpaceID) == "" {
		return clientcontract.Error(req.RequestID, string(boundary.InvalidArgument), "space_id is required")
	}
	return responsePayload(req.RequestID, clientcontract.RuntimeInfoResponse{Sessions: len(c.sessions.List())})
}

func (c *Channel) handleSessionGet(msg natskit.Message) clientcontract.ResponseEnvelope {
	req, ok := decodeRequest(msg)
	if !ok {
		return clientcontract.Error("unknown", string(boundary.InvalidArgument), "invalid request envelope")
	}
	var payload clientcontract.RuntimeSessionRequest
	if err := req.DecodePayload(&payload); err != nil {
		return clientcontract.Error(req.RequestID, string(boundary.InvalidArgument), err.Error())
	}
	if strings.TrimSpace(payload.SpaceID) == "" {
		return clientcontract.Error(req.RequestID, string(boundary.InvalidArgument), "space_id is required")
	}
	if strings.TrimSpace(payload.SessionID) == "" {
		return clientcontract.Error(req.RequestID, string(boundary.InvalidArgument), "session_id is required")
	}
	return responsePayload(req.RequestID, clientcontract.RuntimeSessionResponse{
		SessionID: payload.SessionID,
		Found:     c.sessions.Has(payload.SessionID),
	})
}

func (c *Channel) handlePlanGet(msg natskit.Message) clientcontract.ResponseEnvelope {
	req, ok := decodeRequest(msg)
	if !ok {
		return clientcontract.Error("unknown", string(boundary.InvalidArgument), "invalid request envelope")
	}
	if _, err := decodeRuntimePlanRequest(req); err != nil {
		return clientcontract.Error(req.RequestID, string(boundary.InvalidArgument), err.Error())
	}
	return responsePayload(req.RequestID, c.planResponse())
}

func (c *Channel) handlePlanApprove(msg natskit.Message) clientcontract.ResponseEnvelope {
	req, ok := decodeRequest(msg)
	if !ok {
		return clientcontract.Error("unknown", string(boundary.InvalidArgument), "invalid request envelope")
	}
	if _, err := decodeRuntimePlanRequest(req); err != nil {
		return clientcontract.Error(req.RequestID, string(boundary.InvalidArgument), err.Error())
	}
	if c.plan != nil {
		c.plan.Resume()
	}
	return responsePayload(req.RequestID, c.planResponse())
}

func (c *Channel) handlePlanReject(msg natskit.Message) clientcontract.ResponseEnvelope {
	req, ok := decodeRequest(msg)
	if !ok {
		return clientcontract.Error("unknown", string(boundary.InvalidArgument), "invalid request envelope")
	}
	if _, err := decodeRuntimePlanRequest(req); err != nil {
		return clientcontract.Error(req.RequestID, string(boundary.InvalidArgument), err.Error())
	}
	if c.plan != nil {
		c.plan.Pause()
	}
	return responsePayload(req.RequestID, c.planResponse())
}

func (c *Channel) handleActivityList(msg natskit.Message) clientcontract.ResponseEnvelope {
	req, ok := decodeRequest(msg)
	if !ok {
		return clientcontract.Error("unknown", string(boundary.InvalidArgument), "invalid request envelope")
	}
	var payload clientcontract.RuntimeActivityListRequest
	if err := req.DecodePayload(&payload); err != nil {
		return clientcontract.Error(req.RequestID, string(boundary.InvalidArgument), err.Error())
	}
	if strings.TrimSpace(payload.SpaceID) == "" {
		return clientcontract.Error(req.RequestID, string(boundary.InvalidArgument), "space_id is required")
	}
	var records []clientcontract.RuntimeActivityRecord
	if c.activity != nil {
		records = mapActivityRecords(c.activity.List(payload.Limit))
	}
	return responsePayload(req.RequestID, clientcontract.RuntimeActivityListResponse{Records: records})
}

func decodeRequest(msg natskit.Message) (clientcontract.RequestEnvelope, bool) {
	var req clientcontract.RequestEnvelope
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		return clientcontract.RequestEnvelope{}, false
	}
	if err := req.Validate(); err != nil {
		return clientcontract.RequestEnvelope{}, false
	}
	return req.Clone(), true
}

func decodeRuntimePlanRequest(req clientcontract.RequestEnvelope) (clientcontract.RuntimePlanRequest, error) {
	var payload clientcontract.RuntimePlanRequest
	if err := req.DecodePayload(&payload); err != nil {
		return clientcontract.RuntimePlanRequest{}, err
	}
	if strings.TrimSpace(payload.SpaceID) == "" {
		return clientcontract.RuntimePlanRequest{}, errors.New("space_id is required")
	}
	return payload, nil
}

func validateSendMessage(payload clientcontract.SendMessageRequest) error {
	if strings.TrimSpace(payload.SpaceID) == "" {
		return errors.New("space_id is required")
	}
	if strings.TrimSpace(payload.SessionID) == "" {
		return errors.New("session_id is required")
	}
	if strings.TrimSpace(payload.Content) == "" {
		return errors.New("content is required")
	}
	return nil
}

func responsePayload(requestID string, payload any) clientcontract.ResponseEnvelope {
	resp, err := clientcontract.OK(requestID, payload)
	if err != nil {
		return clientcontract.Error(requestID, string(boundary.Internal), err.Error())
	}
	return resp
}

func encodeResponse(resp clientcontract.ResponseEnvelope) []byte {
	data, err := json.Marshal(resp)
	if err != nil {
		data = []byte(`{"version":"v1","request_id":"unknown","status":"error","error":{"category":"internal","message":"marshal response"}}`)
	}
	return data
}
