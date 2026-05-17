package servicekit

import (
	"context"
	"fmt"
	"os"
	"sort"
	"sync"

	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Registry is a process-local service registry exposed over gRPC. Services use
// it to publish their RPC contracts and SKILL.md contents to agents and
// operators.
type Registry struct {
	servicev1.UnimplementedServiceRegistryServer

	mu       sync.RWMutex
	services map[string]*servicev1.ServiceDescriptor
}

func NewRegistry() *Registry {
	return &Registry{services: make(map[string]*servicev1.ServiceDescriptor)}
}

func (r *Registry) Register(desc *servicev1.ServiceDescriptor) error {
	if desc == nil {
		return fmt.Errorf("service descriptor is required")
	}
	if err := ValidateServiceDescriptor(desc); err != nil {
		return err
	}
	cp := CloneDescriptor(desc)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.services[cp.Name] = cp
	return nil
}

func (r *Registry) ListServices(context.Context, *emptypb.Empty) (*servicev1.ListServicesResponse, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := &servicev1.ListServicesResponse{Services: make([]*servicev1.ServiceDescriptor, 0, len(r.services))}
	for _, desc := range r.services {
		out.Services = append(out.Services, CloneDescriptor(desc))
	}
	sort.Slice(out.Services, func(i, j int) bool {
		return out.Services[i].GetName() < out.Services[j].GetName()
	})
	return out, nil
}

func (r *Registry) GetService(_ context.Context, req *servicev1.GetServiceRequest) (*servicev1.ServiceDescriptor, error) {
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	desc, ok := r.services[req.GetName()]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "service %q not found", req.GetName())
	}
	return CloneDescriptor(desc), nil
}

// SkillFromFile returns a service skill descriptor sourced from SKILL.md.
func SkillFromFile(name, version, path string) (*servicev1.SkillDescriptor, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read skill %s: %w", path, err)
	}
	return &servicev1.SkillDescriptor{
		Name:     name,
		Version:  version,
		Markdown: string(data),
	}, nil
}
