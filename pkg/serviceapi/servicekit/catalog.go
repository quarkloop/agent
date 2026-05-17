package servicekit

import (
	"encoding/json"
	"fmt"
	"regexp"

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
		if err := validateServiceDescriptor(i, desc); err != nil {
			return err
		}
		seenFunctionNames := make(map[string]struct{}, len(desc.GetRpcs()))
		for j, rpc := range desc.GetRpcs() {
			if err := validateResolvedRPC(i, desc.GetName(), j, rpc); err != nil {
				return err
			}
			if _, ok := seenFunctionNames[rpc.GetFunctionName()]; ok {
				return fmt.Errorf("services[%d] %q rpcs[%d]: duplicate function name %q", i, desc.GetName(), j, rpc.GetFunctionName())
			}
			seenFunctionNames[rpc.GetFunctionName()] = struct{}{}
		}
	}
	return nil
}

func ValidateServiceDescriptor(desc *servicev1.ServiceDescriptor) error {
	return validateServiceDescriptor(0, desc)
}

func validateServiceDescriptor(i int, desc *servicev1.ServiceDescriptor) error {
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
	return nil
}

var serviceFunctionNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)

func validateResolvedRPC(i int, serviceName string, j int, rpc *servicev1.RpcDescriptor) error {
	if rpc.GetOwner() == "" {
		return fmt.Errorf("services[%d] %q rpcs[%d]: missing owner", i, serviceName, j)
	}
	if rpc.GetFunctionName() == "" {
		return fmt.Errorf("services[%d] %q rpcs[%d]: missing function name", i, serviceName, j)
	}
	if !serviceFunctionNamePattern.MatchString(rpc.GetFunctionName()) {
		return fmt.Errorf("services[%d] %q rpcs[%d]: invalid function name %q", i, serviceName, j, rpc.GetFunctionName())
	}
	switch rpc.GetRiskLevel() {
	case "read", "write", "admin":
	default:
		return fmt.Errorf("services[%d] %q rpcs[%d]: invalid risk level %q", i, serviceName, j, rpc.GetRiskLevel())
	}
	if rpc.GetTimeoutMillis() < 0 {
		return fmt.Errorf("services[%d] %q rpcs[%d]: timeout_millis must be non-negative", i, serviceName, j)
	}
	if retry := rpc.GetRetryPolicy(); retry != nil {
		if retry.GetMaxAttempts() < 0 {
			return fmt.Errorf("services[%d] %q rpcs[%d]: retry_policy.max_attempts must be non-negative", i, serviceName, j)
		}
		if retry.GetInitialBackoffMillis() < 0 {
			return fmt.Errorf("services[%d] %q rpcs[%d]: retry_policy.initial_backoff_millis must be non-negative", i, serviceName, j)
		}
		if retry.GetMaxBackoffMillis() < 0 {
			return fmt.Errorf("services[%d] %q rpcs[%d]: retry_policy.max_backoff_millis must be non-negative", i, serviceName, j)
		}
	}
	for k, example := range rpc.GetExamples() {
		if example.GetName() == "" {
			return fmt.Errorf("services[%d] %q rpcs[%d] examples[%d]: missing name", i, serviceName, j, k)
		}
		if example.GetRequestJson() == "" {
			return fmt.Errorf("services[%d] %q rpcs[%d] examples[%d]: missing request_json", i, serviceName, j, k)
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
