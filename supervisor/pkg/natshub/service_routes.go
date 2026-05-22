package natshub

import (
	"errors"
	"fmt"
	"strings"

	natsserver "github.com/nats-io/nats-server/v2/server"
)

type ServiceFunctionRoute struct {
	Name          string
	ExportSubject string
	ImportSubject string
}

func NewServiceFunctionRoute(service, version, function string) (ServiceFunctionRoute, error) {
	service = subjectToken(service)
	version = subjectToken(version)
	function = subjectToken(function)
	if service == "_" || version == "_" || function == "_" {
		return ServiceFunctionRoute{}, errors.New("service, version, and function are required")
	}
	subject := fmt.Sprintf("svc.%s.%s.%s", service, version, function)
	return ServiceFunctionRoute{
		Name:          strings.Join([]string{service, version, function}, "_"),
		ExportSubject: subject,
		ImportSubject: subject,
	}, nil
}

func NormalizeServiceFunctionRoutes(routes []ServiceFunctionRoute) ([]ServiceFunctionRoute, error) {
	if len(routes) == 0 {
		return nil, errors.New("at least one service function route is required")
	}
	out := make([]ServiceFunctionRoute, 0, len(routes))
	seen := make(map[string]struct{}, len(routes))
	for _, route := range routes {
		normalized, err := normalizeServiceFunctionRoute(route)
		if err != nil {
			return nil, err
		}
		key := normalized.ImportSubject + "\x00" + normalized.ExportSubject
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, normalized)
	}
	return out, nil
}

func cloneServiceFunctionRoutes(routes []ServiceFunctionRoute) []ServiceFunctionRoute {
	out := make([]ServiceFunctionRoute, len(routes))
	copy(out, routes)
	return out
}

func normalizeServiceFunctionRoute(route ServiceFunctionRoute) (ServiceFunctionRoute, error) {
	route.Name = strings.TrimSpace(route.Name)
	route.ExportSubject = strings.TrimSpace(route.ExportSubject)
	route.ImportSubject = strings.TrimSpace(route.ImportSubject)
	if route.ExportSubject == "" {
		return ServiceFunctionRoute{}, errors.New("service function export subject is required")
	}
	if route.ImportSubject == "" {
		route.ImportSubject = route.ExportSubject
	}
	if !natsserver.IsValidLiteralSubject(route.ExportSubject) {
		return ServiceFunctionRoute{}, fmt.Errorf("invalid service function export subject %q", route.ExportSubject)
	}
	if !natsserver.IsValidLiteralSubject(route.ImportSubject) {
		return ServiceFunctionRoute{}, fmt.Errorf("invalid service function import subject %q", route.ImportSubject)
	}
	if route.Name == "" {
		route.Name = route.ImportSubject
	}
	return route, nil
}
