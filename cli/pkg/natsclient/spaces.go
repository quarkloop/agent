package natsclient

import (
	"context"

	"github.com/quarkloop/pkg/serviceapi/clientcontract"
)

func (c *Client) CreateSpace(ctx context.Context, req clientcontract.CreateSpaceRequest) (clientcontract.SpaceInfo, error) {
	return requestPayload[clientcontract.SpaceInfo](ctx, c, clientcontract.SubjectSpaceCreate, "", req)
}

func (c *Client) ListSpaces(ctx context.Context) (clientcontract.ListSpacesResponse, error) {
	return requestPayload[clientcontract.ListSpacesResponse](ctx, c, clientcontract.SubjectSpaceList, "", struct{}{})
}

func (c *Client) GetSpace(ctx context.Context, name string) (clientcontract.SpaceInfo, error) {
	return requestPayload[clientcontract.SpaceInfo](ctx, c, clientcontract.SubjectSpaceGet, "", clientcontract.GetSpaceRequest{Name: name})
}

func (c *Client) UpdateSpace(ctx context.Context, name string, config []byte) (clientcontract.SpaceInfo, error) {
	return requestPayload[clientcontract.SpaceInfo](ctx, c, clientcontract.SubjectSpaceUpdate, name, clientcontract.UpdateSpaceRequest{
		Config: append([]byte(nil), config...),
	})
}

func (c *Client) DeleteSpace(ctx context.Context, name string) error {
	_, err := requestPayload[struct{}](ctx, c, clientcontract.SubjectSpaceDelete, name, clientcontract.DeleteSpaceRequest{Name: name})
	return err
}

func (c *Client) SpaceConfig(ctx context.Context, name string) (clientcontract.SpaceConfigResponse, error) {
	return requestPayload[clientcontract.SpaceConfigResponse](ctx, c, clientcontract.SubjectSpaceConfig, name, clientcontract.SpaceConfigRequest{Name: name})
}

func (c *Client) Doctor(ctx context.Context, name string) (clientcontract.DoctorResponse, error) {
	return requestPayload[clientcontract.DoctorResponse](ctx, c, clientcontract.SubjectSpaceDoctor, name, clientcontract.DoctorRequest{Name: name})
}

func (c *Client) IssueSpaceCredential(ctx context.Context, spaceID string) (clientcontract.NATSCredential, error) {
	resp, err := requestPayload[clientcontract.SpaceCredentialResponse](ctx, c, clientcontract.SubjectSpaceCredential, spaceID, clientcontract.SpaceCredentialRequest{
		SpaceID: spaceID,
	})
	if err != nil {
		return clientcontract.NATSCredential{}, err
	}
	return resp.Credential, nil
}

func (c *Client) IssueRuntimeCredential(ctx context.Context, spaceID string) (clientcontract.NATSCredential, error) {
	resp, err := requestPayload[clientcontract.SpaceCredentialResponse](ctx, c, clientcontract.SubjectRuntimeCredential, spaceID, clientcontract.SpaceCredentialRequest{
		SpaceID: spaceID,
	})
	if err != nil {
		return clientcontract.NATSCredential{}, err
	}
	return resp.Credential, nil
}
