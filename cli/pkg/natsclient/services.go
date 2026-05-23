package natsclient

import (
	"context"

	"github.com/quarkloop/pkg/serviceapi/clientcontract"
)

func (c *Client) ListServices(ctx context.Context, spaceID string) ([]clientcontract.ServiceInfo, error) {
	resp, err := requestPayload[clientcontract.ListServicesResponse](ctx, c, clientcontract.SubjectServiceList, spaceID, clientcontract.ListServicesRequest{SpaceID: spaceID})
	if err != nil {
		return nil, err
	}
	return cloneServices(resp.Services), nil
}

func (c *Client) InspectService(ctx context.Context, spaceID, service string) (clientcontract.ServiceInfo, error) {
	return requestPayload[clientcontract.ServiceInfo](ctx, c, clientcontract.SubjectServiceInspect, spaceID, clientcontract.InspectServiceRequest{
		SpaceID: spaceID,
		Service: service,
	})
}

func (c *Client) ServiceDoctor(ctx context.Context, spaceID string) (clientcontract.ServiceDoctorResponse, error) {
	resp, err := requestPayload[clientcontract.ServiceDoctorResponse](ctx, c, clientcontract.SubjectServiceDoctor, spaceID, clientcontract.ListServicesRequest{SpaceID: spaceID})
	if err != nil {
		return clientcontract.ServiceDoctorResponse{}, err
	}
	resp.Services = cloneServices(resp.Services)
	resp.Issues = append([]string(nil), resp.Issues...)
	return resp, nil
}

func cloneServices(in []clientcontract.ServiceInfo) []clientcontract.ServiceInfo {
	out := make([]clientcontract.ServiceInfo, 0, len(in))
	for _, service := range in {
		copyService := service
		copyService.Functions = append([]clientcontract.ServiceFunctionInfo(nil), service.Functions...)
		copyService.Diagnostics = append([]string(nil), service.Diagnostics...)
		out = append(out, copyService)
	}
	return out
}
