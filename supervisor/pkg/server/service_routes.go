package server

import (
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
	"github.com/quarkloop/supervisor/pkg/natshub"
)

func (s *Server) importServiceFunctionRoutes(space string, descriptors []*servicev1.ServiceDescriptor) error {
	if s == nil || s.natsHub == nil || len(descriptors) == 0 {
		return nil
	}
	routes := make([]natshub.ServiceFunctionRoute, 0)
	for _, desc := range descriptors {
		for _, rpc := range desc.GetRpcs() {
			route, err := natshub.NewServiceFunctionRouteFromSubject(rpc.GetSubject(), rpc.GetStreaming())
			if err != nil {
				return err
			}
			routes = append(routes, route)
		}
	}
	if len(routes) == 0 {
		return nil
	}
	return s.natsHub.ImportServiceFunctions(space, routes)
}
