package natsapi

import (
	"context"
	"fmt"

	"github.com/quarkloop/pkg/boundary"
	"github.com/quarkloop/pkg/serviceapi/clientcontract"
)

func (s *Server) listServices(req clientcontract.RequestEnvelope) (any, error) {
	var payload clientcontract.ListServicesRequest
	if err := req.DecodePayload(&payload); err != nil {
		return nil, err
	}
	services, err := s.inspectServices(payload.SpaceID)
	if err != nil {
		return nil, err
	}
	return clientcontract.ListServicesResponse{Services: services}, nil
}

func (s *Server) inspectService(req clientcontract.RequestEnvelope) (any, error) {
	var payload clientcontract.InspectServiceRequest
	if err := req.DecodePayload(&payload); err != nil {
		return nil, err
	}
	services, err := s.inspectServices(payload.SpaceID)
	if err != nil {
		return nil, err
	}
	for _, service := range services {
		if service.Name == payload.Service {
			return service, nil
		}
	}
	return nil, boundary.New(boundary.Supervisor, boundary.NotFound, clientcontract.SubjectServiceInspect, "service "+payload.Service+" not found")
}

func (s *Server) serviceDoctor(req clientcontract.RequestEnvelope) (any, error) {
	var payload clientcontract.ListServicesRequest
	if err := req.DecodePayload(&payload); err != nil {
		return nil, err
	}
	services, err := s.inspectServices(payload.SpaceID)
	if err != nil {
		return nil, err
	}
	issues := make([]string, 0)
	for _, service := range services {
		if service.Status == clientcontract.ServiceStatusReady {
			continue
		}
		for _, diagnostic := range service.Diagnostics {
			issues = append(issues, service.Name+": "+diagnostic)
		}
		if len(service.Diagnostics) == 0 {
			issues = append(issues, fmt.Sprintf("%s: status is %s", service.Name, service.Status))
		}
	}
	return clientcontract.ServiceDoctorResponse{Services: services, Issues: issues}, nil
}

func (s *Server) inspectServices(spaceID string) ([]clientcontract.ServiceInfo, error) {
	if s.serviceInspector == nil {
		return nil, boundary.New(boundary.Supervisor, boundary.Unavailable, "service inspection", "service inspector is not configured")
	}
	services, err := s.serviceInspector.InspectServices(context.Background(), spaceID)
	if err != nil {
		return nil, err
	}
	out := make([]clientcontract.ServiceInfo, 0, len(services))
	for _, service := range services {
		service.Functions = append([]clientcontract.ServiceFunctionInfo(nil), service.Functions...)
		service.Diagnostics = append([]string(nil), service.Diagnostics...)
		out = append(out, service)
	}
	return out, nil
}
