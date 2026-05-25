package server

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	plugin "github.com/quarkloop/pkg/plugin"
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
	"github.com/quarkloop/supervisor/pkg/pluginmanager"
)

const defaultServiceFunctionTimeout = 30 * time.Second

func descriptorsFromServiceManifest(manifest *plugin.Manifest) ([]*servicev1.ServiceDescriptor, error) {
	if manifest == nil || manifest.Service == nil {
		return nil, fmt.Errorf("service manifest is missing service config")
	}
	desc := &servicev1.ServiceDescriptor{
		Name:    manifest.Name,
		Type:    manifest.Name,
		Version: manifest.Version,
		Address: manifest.Service.SubjectPrefix,
		Rpcs:    make([]*servicev1.RpcDescriptor, 0, len(manifest.Service.Functions)),
	}
	for _, function := range manifest.Service.Functions {
		desc.Rpcs = append(desc.Rpcs, &servicev1.RpcDescriptor{
			Service: function.Service,
			Method:  function.Method,
			Subject: function.Subject,
		})
	}
	if err := applyServiceFunctionMetadata(desc, manifest); err != nil {
		return nil, err
	}
	return []*servicev1.ServiceDescriptor{desc}, nil
}

func applyServiceFunctionMetadata(desc *servicev1.ServiceDescriptor, manifest *plugin.Manifest) error {
	if desc == nil || manifest == nil || manifest.Service == nil {
		return nil
	}
	functions := manifest.Service.Functions
	if len(functions) == 0 {
		return nil
	}
	byRPC := make(map[string]plugin.ServiceFunctionConfig, len(functions))
	for _, function := range functions {
		byRPC[serviceFunctionKey(function.Service, function.Method)] = function
	}
	for _, rpc := range desc.GetRpcs() {
		function, ok := byRPC[serviceFunctionKey(rpc.GetService(), rpc.GetMethod())]
		if !ok {
			return fmt.Errorf("missing manifest function for %s/%s", rpc.GetService(), rpc.GetMethod())
		}
		rpc.Request = function.Request
		rpc.Response = function.Response
		rpc.Description = function.Description
		rpc.Owner = serviceFunctionOwner(manifest.Name, function)
		rpc.FunctionName = function.Name
		rpc.Subject = function.Subject
		rpc.RiskLevel = serviceFunctionRisk(function)
		rpc.ApprovalRequired = function.ApprovalRequired
		rpc.ApprovalRequirements = append([]string(nil), function.ApprovalRequirements...)
		rpc.Streaming = function.Streaming
		rpc.Idempotent = function.Idempotent
		rpc.TimeoutMillis = serviceFunctionTimeoutMillis(function)
		rpc.RetryPolicy = serviceFunctionRetryPolicy(function)
		rpc.Examples = serviceFunctionExamples(function)
	}
	return nil
}

func serviceFunctionOwner(pluginName string, function plugin.ServiceFunctionConfig) string {
	if strings.TrimSpace(function.Owner) != "" {
		return strings.TrimSpace(function.Owner)
	}
	return strings.TrimSpace(pluginName)
}

func serviceFunctionRisk(function plugin.ServiceFunctionConfig) string {
	if strings.TrimSpace(function.RiskLevel) == "" {
		return "read"
	}
	return strings.TrimSpace(function.RiskLevel)
}

func serviceFunctionTimeoutMillis(function plugin.ServiceFunctionConfig) int32 {
	timeout := defaultServiceFunctionTimeout
	if strings.TrimSpace(function.Timeout) != "" {
		parsed, err := time.ParseDuration(function.Timeout)
		if err == nil {
			timeout = parsed
		}
	}
	return int32(timeout / time.Millisecond)
}

func serviceFunctionRetryPolicy(function plugin.ServiceFunctionConfig) *servicev1.RetryPolicy {
	policy := function.RetryPolicy
	if policy.MaxAttempts == 0 && len(policy.RetryableCodes) == 0 && policy.InitialBackoffMillis == 0 && policy.MaxBackoffMillis == 0 {
		return nil
	}
	return &servicev1.RetryPolicy{
		MaxAttempts:          int32(policy.MaxAttempts),
		RetryableCodes:       append([]string(nil), policy.RetryableCodes...),
		InitialBackoffMillis: int32(policy.InitialBackoffMillis),
		MaxBackoffMillis:     int32(policy.MaxBackoffMillis),
	}
}

func serviceFunctionExamples(function plugin.ServiceFunctionConfig) []*servicev1.ServiceFunctionExample {
	out := make([]*servicev1.ServiceFunctionExample, 0, len(function.Examples))
	for _, example := range function.Examples {
		out = append(out, &servicev1.ServiceFunctionExample{
			Name:        example.Name,
			Description: example.Description,
			RequestJson: example.RequestJSON,
		})
	}
	return out
}

func serviceFunctionKey(service, method string) string {
	return strings.TrimSpace(service) + "/" + strings.TrimSpace(method)
}

func serviceHealthName(manifest *plugin.Manifest) string {
	if manifest == nil || manifest.Service == nil {
		return ""
	}
	if manifest.Service.Health.Service != "" {
		return manifest.Service.Health.Service
	}
	if len(manifest.Service.ProtoServices) > 0 {
		return manifest.Service.ProtoServices[0]
	}
	return ""
}

func validateServicePluginDescriptors(descriptors []*servicev1.ServiceDescriptor, manifest *plugin.Manifest) error {
	if manifest == nil || manifest.Service == nil {
		return nil
	}
	if len(descriptors) == 0 {
		return fmt.Errorf("no descriptors returned")
	}
	minVersion := strings.TrimSpace(manifest.Service.Readiness.MinVersion)
	seenRPC := make(map[string]bool)
	seenFunction := make(map[string]bool)
	for _, desc := range descriptors {
		if desc.GetName() == "" {
			return fmt.Errorf("missing descriptor name")
		}
		if minVersion != "" && desc.GetVersion() != minVersion {
			return fmt.Errorf("unsupported version %q for %s (required: %s)", desc.GetVersion(), desc.GetName(), minVersion)
		}
		if desc.GetAddress() == "" {
			return fmt.Errorf("descriptor %s missing endpoint address", desc.GetName())
		}
		for _, rpc := range desc.GetRpcs() {
			seenRPC[serviceFunctionKey(rpc.GetService(), rpc.GetMethod())] = true
			if rpc.GetFunctionName() == "" {
				return fmt.Errorf("missing function name for %s/%s", rpc.GetService(), rpc.GetMethod())
			}
			if seenFunction[rpc.GetFunctionName()] {
				return fmt.Errorf("duplicate function name %s", rpc.GetFunctionName())
			}
			seenFunction[rpc.GetFunctionName()] = true
			function, ok := functionByRPC(manifest.Service.Functions, rpc.GetService(), rpc.GetMethod())
			if !ok || strings.TrimSpace(rpc.GetSubject()) != strings.TrimSpace(function.Subject) {
				return fmt.Errorf("NATS subject mismatch for %s/%s", rpc.GetService(), rpc.GetMethod())
			}
		}
	}
	for _, function := range manifest.Service.Functions {
		if !seenRPC[serviceFunctionKey(function.Service, function.Method)] {
			return fmt.Errorf("missing RPC descriptor for %s/%s", function.Service, function.Method)
		}
		if !seenFunction[function.Name] {
			return fmt.Errorf("missing function descriptor for %s", function.Name)
		}
	}
	return nil
}

func functionByRPC(functions []plugin.ServiceFunctionConfig, service, method string) (plugin.ServiceFunctionConfig, bool) {
	key := serviceFunctionKey(service, method)
	for _, function := range functions {
		if serviceFunctionKey(function.Service, function.Method) == key {
			return function, true
		}
	}
	return plugin.ServiceFunctionConfig{}, false
}

func loadServicePluginSkill(item pluginmanager.InstalledPlugin) *servicev1.SkillDescriptor {
	if item.Manifest == nil || item.Manifest.Service == nil {
		return nil
	}
	skillPath := item.Manifest.Service.Skill
	if skillPath == "" {
		skillPath = "SKILL.md"
	}
	data, err := os.ReadFile(filepath.Join(item.Path, skillPath))
	if err != nil {
		return nil
	}
	return &servicev1.SkillDescriptor{
		Name:     "service-" + item.Manifest.Name,
		Version:  item.Manifest.Version,
		Markdown: string(data),
	}
}

func replaceSkill(skills []*servicev1.SkillDescriptor, skill *servicev1.SkillDescriptor) []*servicev1.SkillDescriptor {
	out := make([]*servicev1.SkillDescriptor, 0, len(skills)+1)
	for _, existing := range skills {
		if existing.GetName() == skill.GetName() {
			continue
		}
		out = append(out, existing)
	}
	return append(out, skill)
}
