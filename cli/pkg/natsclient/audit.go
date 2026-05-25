package natsclient

import (
	"context"

	"github.com/quarkloop/pkg/serviceapi/clientcontract"
)

func (c *Client) GetAuditRecord(ctx context.Context, spaceID, referenceID string) (clientcontract.AuditRecord, error) {
	record, err := requestPayload[clientcontract.AuditRecord](ctx, c, clientcontract.SubjectAuditGet, spaceID, clientcontract.AuditGetRequest{
		SpaceID:     spaceID,
		ReferenceID: referenceID,
	})
	if err != nil {
		return clientcontract.AuditRecord{}, err
	}
	return record, nil
}

func (c *Client) ListAuditRecords(ctx context.Context, request clientcontract.AuditListRequest) (clientcontract.AuditListResponse, error) {
	page, err := requestPayload[clientcontract.AuditListResponse](ctx, c, clientcontract.SubjectAuditList, request.SpaceID, request)
	if err != nil {
		return clientcontract.AuditListResponse{}, err
	}
	return page, nil
}

func (c *Client) AuditRetention(ctx context.Context) (clientcontract.AuditRetentionResponse, error) {
	return requestPayload[clientcontract.AuditRetentionResponse](ctx, c, clientcontract.SubjectAuditRetention, "", struct{}{})
}
