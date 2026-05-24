package server

import (
	"fmt"
	"sort"
	"strings"

	"github.com/quarkloop/pkg/plugin"
	spacemodel "github.com/quarkloop/pkg/space"
)

// agentProfileOverrideResolver applies the Quarkfile override layer over
// installed agent plugin profiles. Precedence is:
//
// built-in profile defaults -> installed plugin defaults -> Quarkfile
// overrides -> temporary session overrides.
//
// Temporary session overrides are runtime-scoped and intentionally not applied
// here.
type agentProfileOverrideResolver struct {
	quarkfile *spacemodel.Quarkfile
	agents    map[string]spacemodel.AgentRef
	order     []string
	explicit  bool
	matched   map[string]bool
	catalog   agentPluginValidationCatalog
}

func newAgentProfileOverrideResolver(qf *spacemodel.Quarkfile, catalog agentPluginValidationCatalog) *agentProfileOverrideResolver {
	resolver := &agentProfileOverrideResolver{
		quarkfile: qf,
		agents:    make(map[string]spacemodel.AgentRef),
		order:     make([]string, 0),
		matched:   make(map[string]bool),
		catalog:   catalog,
	}
	if qf == nil {
		return resolver
	}
	resolver.explicit = len(qf.Agents) > 0
	for _, agent := range qf.Agents {
		resolver.agents[agent.Profile] = agent
		resolver.order = append(resolver.order, agent.Profile)
	}
	return resolver
}

func (r *agentProfileOverrideResolver) apply(entries []runtimePluginCatalogEntry) ([]runtimePluginCatalogEntry, string, error) {
	resolved := make([]runtimePluginCatalogEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.AgentProfile == nil {
			resolved = append(resolved, entry)
			continue
		}
		next, enabled, err := r.applyOne(entry)
		if err != nil {
			return nil, "", err
		}
		if enabled {
			resolved = append(resolved, next)
		}
	}
	if err := r.rejectUnknownOverrides(); err != nil {
		return nil, "", err
	}
	resolved = resolveMainHandoffTargets(resolved)
	selected, err := r.selectedAgentProfile(resolved)
	if err != nil {
		return nil, "", err
	}
	return resolved, selected, nil
}

func resolveMainHandoffTargets(entries []runtimePluginCatalogEntry) []runtimePluginCatalogEntry {
	delegates := make(map[string]struct{})
	for _, entry := range entries {
		if entry.AgentProfile != nil && !entry.AgentProfile.IsMain() {
			delegates[entry.AgentProfile.ID] = struct{}{}
		}
	}
	for i, entry := range entries {
		if entry.AgentProfile == nil || !entry.AgentProfile.IsMain() {
			continue
		}
		profile := entry.AgentProfile.Clone()
		targets := make([]string, 0, len(profile.Handoff.CanDelegateTo))
		for _, target := range profile.Handoff.CanDelegateTo {
			if _, ok := delegates[target]; ok {
				targets = append(targets, target)
			}
		}
		profile.Handoff.CanDelegateTo = targets
		entry.AgentProfile = &profile
		entries[i] = entry
	}
	return entries
}

func (r *agentProfileOverrideResolver) applyOne(entry runtimePluginCatalogEntry) (runtimePluginCatalogEntry, bool, error) {
	profile := entry.AgentProfile.Clone()
	override, hasOverride := r.agents[profile.ID]
	if hasOverride {
		r.matched[profile.ID] = true
		if override.Enabled != nil && !*override.Enabled {
			return entry, false, nil
		}
	}
	if !profile.IsMain() && r.explicit && !hasOverride {
		return entry, false, nil
	}
	if !profile.IsMain() && !r.explicit {
		return entry, false, nil
	}

	model := resolvedAgentModel(r.quarkfile, override, hasOverride, profile.Model)
	if model.Provider != "" || model.Model != "" {
		profile = profile.WithModel(model.Provider, model.Model)
	}
	if profile.IsMain() {
		next, err := r.applyMainPermissionResolution(profile, override, hasOverride)
		if err != nil {
			return entry, false, err
		}
		profile = next
		if hasOverride {
			profile = applyAgentApprovalOverride(r.quarkfile, profile, override)
			profile = applyAgentMemoryOverride(profile, override)
		} else {
			profile = applyQuarkfileApprovalPolicy(r.quarkfile, profile)
		}
	} else if hasOverride {
		next, err := applyAgentPermissionOverride(profile, override)
		if err != nil {
			return entry, false, err
		}
		profile = next
		profile = applyAgentApprovalOverride(r.quarkfile, profile, override)
		profile = applyAgentMemoryOverride(profile, override)
	} else {
		profile = applyQuarkfileApprovalPolicy(r.quarkfile, profile)
	}
	entry.AgentProfile = &profile
	return entry, true, nil
}

func (r *agentProfileOverrideResolver) applyMainPermissionResolution(profile plugin.AgentProfile, override spacemodel.AgentRef, hasOverride bool) (plugin.AgentProfile, error) {
	tools := r.catalog.sortedTools()
	services := r.catalog.sortedServiceFunctions()
	if hasOverride {
		if override.Tools != nil {
			if err := ensurePatternsWithin("tools", profile.ID, tools, override.Tools); err != nil {
				return plugin.AgentProfile{}, err
			}
			tools = override.Tools
		}
		if override.Services != nil {
			if err := ensurePatternsWithin("services", profile.ID, services, override.Services); err != nil {
				return plugin.AgentProfile{}, err
			}
			services = override.Services
		}
	}
	return profile.WithPermissions(tools, services), nil
}

func resolvedAgentModel(qf *spacemodel.Quarkfile, override spacemodel.AgentRef, hasOverride bool, base plugin.AgentProfileModel) plugin.AgentProfileModel {
	model := base
	if qf != nil && !qf.Model.IsZero() {
		model.Provider = qf.Model.Provider
		model.Model = qf.Model.Name
	}
	if hasOverride && !override.Model.IsZero() {
		model.Provider = override.Model.Provider
		model.Model = override.Model.Name
	}
	return model
}

func applyAgentPermissionOverride(profile plugin.AgentProfile, override spacemodel.AgentRef) (plugin.AgentProfile, error) {
	tools := []string(nil)
	services := []string(nil)
	if override.Tools != nil {
		if err := ensurePatternsWithin("tools", profile.ID, profile.Permissions.Tools, override.Tools); err != nil {
			return plugin.AgentProfile{}, err
		}
		tools = override.Tools
	}
	if override.Services != nil {
		if err := ensurePatternsWithin("services", profile.ID, profile.Permissions.Services, override.Services); err != nil {
			return plugin.AgentProfile{}, err
		}
		services = override.Services
	}
	return profile.WithPermissions(tools, services), nil
}

func applyAgentApprovalOverride(qf *spacemodel.Quarkfile, profile plugin.AgentProfile, override spacemodel.AgentRef) plugin.AgentProfile {
	profile = applyQuarkfileApprovalPolicy(qf, profile)
	if override.Approval.Policy != "" || override.Approval.RequiredFor != nil {
		profile = profile.WithApproval(override.Approval.Policy, override.Approval.RequiredFor)
	}
	return profile
}

func applyQuarkfileApprovalPolicy(qf *spacemodel.Quarkfile, profile plugin.AgentProfile) plugin.AgentProfile {
	if qf == nil || qf.Capabilities.ApprovalPolicy == "" {
		return profile
	}
	return profile.WithApproval(qf.Capabilities.ApprovalPolicy, nil)
}

func applyAgentMemoryOverride(profile plugin.AgentProfile, override spacemodel.AgentRef) plugin.AgentProfile {
	if override.Memory.Scope == "" && override.Memory.Collections == nil {
		return profile
	}
	return profile.WithMemory(override.Memory.Scope, override.Memory.Collections)
}

func ensurePatternsWithin(kind, profileID string, maximum, requested []string) error {
	for _, pattern := range requested {
		if !patternAllowedByAny(pattern, maximum) {
			return fmt.Errorf("agent profile %s %s override %q exceeds installed profile maximum %v", profileID, kind, pattern, maximum)
		}
	}
	return nil
}

func patternAllowedByAny(requested string, maximum []string) bool {
	for _, allowed := range maximum {
		if patternAllows(allowed, requested) {
			return true
		}
	}
	return false
}

func patternAllows(allowed, requested string) bool {
	if allowed == "*" || allowed == requested {
		return true
	}
	if strings.HasSuffix(allowed, ".*") {
		prefix := strings.TrimSuffix(allowed, ".*") + "."
		return strings.HasPrefix(requested, prefix)
	}
	return false
}

func (r *agentProfileOverrideResolver) rejectUnknownOverrides() error {
	for _, profileID := range r.order {
		if !r.matched[profileID] {
			return fmt.Errorf("quarkfile agent profile %q is not installed", profileID)
		}
	}
	return nil
}

func (r *agentProfileOverrideResolver) selectedAgentProfile(entries []runtimePluginCatalogEntry) (string, error) {
	mainIDs := make([]string, 0, 1)
	for _, entry := range entries {
		if entry.AgentProfile != nil && entry.AgentProfile.IsMain() {
			mainIDs = append(mainIDs, entry.AgentProfile.ID)
		}
	}
	switch len(mainIDs) {
	case 0:
		return "", fmt.Errorf("runtime catalog must include exactly one enabled main agent profile")
	case 1:
		return mainIDs[0], nil
	default:
		sort.Strings(mainIDs)
		return "", fmt.Errorf("runtime catalog has multiple enabled main agent profiles: %v", mainIDs)
	}
}
