package natsclient

import (
	"context"

	"github.com/quarkloop/pkg/serviceapi/clientcontract"
)

func (c *Client) KBGet(ctx context.Context, spaceID, namespace, key string) ([]byte, error) {
	resp, err := requestPayload[clientcontract.KBValueResponse](ctx, c, clientcontract.SubjectKBGet, spaceID, clientcontract.KBRefRequest{
		SpaceID:   spaceID,
		Namespace: namespace,
		Key:       key,
	})
	if err != nil {
		return nil, err
	}
	return append([]byte(nil), resp.Value...), nil
}

func (c *Client) KBSet(ctx context.Context, spaceID, namespace, key string, value []byte) error {
	_, err := requestPayload[struct{}](ctx, c, clientcontract.SubjectKBSet, spaceID, clientcontract.KBSetRequest{
		SpaceID:   spaceID,
		Namespace: namespace,
		Key:       key,
		Value:     append([]byte(nil), value...),
	})
	return err
}

func (c *Client) KBDelete(ctx context.Context, spaceID, namespace, key string) error {
	_, err := requestPayload[struct{}](ctx, c, clientcontract.SubjectKBDelete, spaceID, clientcontract.KBRefRequest{
		SpaceID:   spaceID,
		Namespace: namespace,
		Key:       key,
	})
	return err
}

func (c *Client) KBList(ctx context.Context, spaceID, namespace string) ([]string, error) {
	resp, err := requestPayload[clientcontract.KBListResponse](ctx, c, clientcontract.SubjectKBList, spaceID, clientcontract.KBListRequest{
		SpaceID:   spaceID,
		Namespace: namespace,
	})
	if err != nil {
		return nil, err
	}
	return append([]string(nil), resp.Keys...), nil
}
