package server

import (
	"context"

	"github.com/quarkloop/pkg/serviceapi/clientcontract"
	"github.com/quarkloop/supervisor/pkg/api"
)

type natsServiceInspector struct {
	server *Server
}

func (i natsServiceInspector) InspectServices(ctx context.Context, spaceID string) ([]clientcontract.ServiceInfo, error) {
	services, err := i.server.inspectServices(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	out := make([]clientcontract.ServiceInfo, 0, len(services))
	for _, service := range services {
		out = append(out, serviceInfoToContract(service))
	}
	return out, nil
}

func serviceInfoToContract(service api.ServiceInfo) clientcontract.ServiceInfo {
	functions := make([]clientcontract.ServiceFunctionInfo, 0, len(service.Functions))
	for _, fn := range service.Functions {
		functions = append(functions, clientcontract.ServiceFunctionInfo{
			Name:        fn.Name,
			Subject:     fn.Subject,
			Service:     fn.Service,
			Method:      fn.Method,
			Request:     fn.Request,
			Response:    fn.Response,
			Description: fn.Description,
			RiskLevel:   fn.RiskLevel,
			Approval:    fn.Approval,
			Idempotent:  fn.Idempotent,
		})
	}
	return clientcontract.ServiceInfo{
		Name:          service.Name,
		Type:          service.Type,
		Version:       service.Version,
		Mode:          service.Mode,
		Description:   service.Description,
		Status:        clientcontract.ServiceStatus(service.Status),
		PID:           service.PID,
		Endpoint:      service.Endpoint,
		LogPath:       service.LogPath,
		StartedAt:     service.StartedAt,
		AddressEnv:    service.AddressEnv,
		HealthService: service.HealthService,
		MinVersion:    service.MinVersion,
		FunctionCount: service.FunctionCount,
		Functions:     functions,
		Diagnostics:   append([]string(nil), service.Diagnostics...),
	}
}
