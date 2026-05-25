package natshub

import (
	"errors"
	"fmt"
	"strings"

	"github.com/quarkloop/pkg/natskit"
)

type ServiceFunctionRoute struct {
	Name          string
	ExportSubject string
	ImportSubject string
	Streaming     bool
}

func NewServiceFunctionRoute(service, version, function string) (ServiceFunctionRoute, error) {
	if strings.TrimSpace(version) != natskit.DefaultVersion {
		return ServiceFunctionRoute{}, fmt.Errorf("unsupported service operation version %q", version)
	}
	operation, err := natskit.ServiceOperation(service, function)
	if err != nil {
		return ServiceFunctionRoute{}, err
	}
	return ServiceFunctionRoute{
		Name:          strings.ReplaceAll(strings.TrimPrefix(operation.Subject, natskit.ServiceSubjectPrefix+"."), ".", "_"),
		ExportSubject: operation.Subject,
		ImportSubject: operation.Subject,
	}, nil
}

func NewServiceFunctionRouteFromFunctionName(owner, functionName string) (ServiceFunctionRoute, error) {
	operation, err := natskit.ServiceOperationFromFunctionName(owner, functionName)
	if err != nil {
		return ServiceFunctionRoute{}, err
	}
	return ServiceFunctionRoute{
		Name:          strings.ReplaceAll(strings.TrimPrefix(operation.Subject, natskit.ServiceSubjectPrefix+"."), ".", "_"),
		ExportSubject: operation.Subject,
		ImportSubject: operation.Subject,
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
		key := normalized.key()
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

func (route ServiceFunctionRoute) key() string {
	return route.ImportSubject + "\x00" + route.ExportSubject + fmt.Sprintf("\x00%t", route.Streaming)
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
	if _, err := natskit.ParseServiceOperation(route.ExportSubject); err != nil {
		return ServiceFunctionRoute{}, fmt.Errorf("invalid service function export subject %q", route.ExportSubject)
	}
	if _, err := natskit.ParseServiceOperation(route.ImportSubject); err != nil {
		return ServiceFunctionRoute{}, fmt.Errorf("invalid service function import subject %q", route.ImportSubject)
	}
	if route.Name == "" {
		route.Name = route.ImportSubject
	}
	return route, nil
}
