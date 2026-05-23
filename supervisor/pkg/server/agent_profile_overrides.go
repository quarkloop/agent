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
}

func newAgentProfileOverrideResolver(qf *spacemodel.Quarkfile) *agentProfileOverrideResolver {
	resolver := &agentProfileOverrideResolver{
		quarkfile: qf,
		agents:    make(map[string]spacemodel.AgentRef),
		order:     make([]string, 0),
		matched:   make(map[string]bool),
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
	selected, err := r.selectedAgentProfile(resolved)
	if err != nil {
		return nil, "", err
	}
	return resolved, selected, nil
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
	if r.explicit && !hasOverride {
		return entry, false, nil
	}

	model := resolvedAgentModel(r.quarkfile, override, hasOverride, profile.Model)
	if model.Provider != "" || model.Model != "" {
		profile = profile.WithModel(model.Provider, model.Model)
	}
	if hasOverride {
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
	if r.explicit {
		for _, profileID := range r.order {
			override := r.agents[profileID]
			if override.Enabled != nil && !*override.Enabled {
				continue
			}
			for _, entry := range entries {
				if entry.AgentProfile != nil && entry.AgentProfile.ID == profileID {
					return profileID, nil
				}
			}
		}
		return "", fmt.Errorf("quarkfile enables no installed agent profiles")
	}
	ids := make([]string, 0)
	for _, entry := range entries {
		if entry.AgentProfile != nil {
			ids = append(ids, entry.AgentProfile.ID)
		}
	}
	if len(ids) == 0 {
		return "", nil
	}
	sort.Strings(ids)
	return ids[0], nil
}
