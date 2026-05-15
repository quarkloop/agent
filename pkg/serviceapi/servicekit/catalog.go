package servicekit

import (
	"encoding/json"
	"fmt"

	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

const RuntimeServiceCatalogVersion = 1

type runtimeServiceCatalogEnvelope struct {
	Version  int             `json:"version"`
	Services json.RawMessage `json:"services"`
}

// MarshalRuntimeServiceCatalog encodes service descriptors into the versioned
// supervisor-to-runtime catalog contract.
func MarshalRuntimeServiceCatalog(descriptors []*servicev1.ServiceDescriptor) ([]byte, error) {
	if err := ValidateRuntimeServiceCatalog(descriptors); err != nil {
		return nil, err
	}
	resp, err := protojson.Marshal(&servicev1.ListServicesResponse{Services: cloneDescriptors(descriptors)})
	if err != nil {
		return nil, fmt.Errorf("marshal services: %w", err)
	}
	var body struct {
		Services json.RawMessage `json:"services"`
	}
	if err := json.Unmarshal(resp, &body); err != nil {
		return nil, fmt.Errorf("extract services payload: %w", err)
	}
	if len(body.Services) == 0 {
		body.Services = json.RawMessage("[]")
	}
	out, err := json.Marshal(runtimeServiceCatalogEnvelope{
		Version:  RuntimeServiceCatalogVersion,
		Services: body.Services,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal service catalog envelope: %w", err)
	}
	return out, nil
}

// UnmarshalRuntimeServiceCatalog decodes and validates the versioned
// supervisor-to-runtime service catalog contract.
func UnmarshalRuntimeServiceCatalog(data []byte) ([]*servicev1.ServiceDescriptor, error) {
	var envelope runtimeServiceCatalogEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("parse service catalog envelope: %w", err)
	}
	if envelope.Version != RuntimeServiceCatalogVersion {
		return nil, fmt.Errorf("unsupported runtime service catalog version %d (supported: %d)", envelope.Version, RuntimeServiceCatalogVersion)
	}
	if len(envelope.Services) == 0 {
		envelope.Services = json.RawMessage("[]")
	}
	payload, err := json.Marshal(struct {
		Services json.RawMessage `json:"services"`
	}{Services: envelope.Services})
	if err != nil {
		return nil, fmt.Errorf("marshal services payload: %w", err)
	}
	var resp servicev1.ListServicesResponse
	if err := protojson.Unmarshal(payload, &resp); err != nil {
		return nil, fmt.Errorf("parse services payload: %w", err)
	}
	descriptors := cloneDescriptors(resp.GetServices())
	if err := ValidateRuntimeServiceCatalog(descriptors); err != nil {
		return nil, err
	}
	return descriptors, nil
}

func ValidateRuntimeServiceCatalog(descriptors []*servicev1.ServiceDescriptor) error {
	for i, desc := range descriptors {
		if desc == nil {
			return fmt.Errorf("services[%d]: descriptor is nil", i)
		}
		if desc.GetName() == "" {
			return fmt.Errorf("services[%d]: missing name", i)
		}
		if desc.GetAddress() == "" {
			return fmt.Errorf("services[%d] %q: missing endpoint address", i, desc.GetName())
		}
		if desc.GetVersion() == "" {
			return fmt.Errorf("services[%d] %q: missing version", i, desc.GetName())
		}
		for j, rpc := range desc.GetRpcs() {
			if rpc.GetService() == "" || rpc.GetMethod() == "" {
				return fmt.Errorf("services[%d] %q rpcs[%d]: missing service or method", i, desc.GetName(), j)
			}
			if rpc.GetRequest() == "" || rpc.GetResponse() == "" {
				return fmt.Errorf("services[%d] %q rpcs[%d]: missing request or response type", i, desc.GetName(), j)
			}
			if rpc.GetDescription() == "" {
				return fmt.Errorf("services[%d] %q rpcs[%d]: missing description", i, desc.GetName(), j)
			}
		}
	}
	return nil
}

func cloneDescriptors(descriptors []*servicev1.ServiceDescriptor) []*servicev1.ServiceDescriptor {
	out := make([]*servicev1.ServiceDescriptor, 0, len(descriptors))
	for _, desc := range descriptors {
		out = append(out, CloneDescriptor(desc))
	}
	return out
}
