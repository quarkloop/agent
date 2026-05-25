package handoff

import (
	"fmt"
	"sort"
	"strings"
)

// Policy is the runtime handoff contract for a resolved agent profile.
type Policy struct {
	ownerProfileID string
	targets        map[string]struct{}
}

// NewPolicy creates a handoff policy with copied, normalized target profile IDs.
func NewPolicy(ownerProfileID string, allowedTargets []string) Policy {
	targets := make(map[string]struct{}, len(allowedTargets))
	for _, target := range allowedTargets {
		target = strings.TrimSpace(target)
		if target == "" {
			continue
		}
		targets[target] = struct{}{}
	}
	return Policy{ownerProfileID: strings.TrimSpace(ownerProfileID), targets: targets}
}

// OwnerProfileID returns the profile this policy belongs to.
func (p Policy) OwnerProfileID() string {
	return p.ownerProfileID
}

// Targets returns the sorted list of profile IDs this policy allows.
func (p Policy) Targets() []string {
	out := make([]string, 0, len(p.targets))
	for target := range p.targets {
		out = append(out, target)
	}
	sort.Strings(out)
	return out
}

// Allows reports whether the owner profile can hand work to the target profile.
func (p Policy) Allows(targetProfileID string) bool {
	targetProfileID = strings.TrimSpace(targetProfileID)
	if targetProfileID == "" {
		return false
	}
	_, ok := p.targets[targetProfileID]
	return ok
}

// ValidateTarget returns an actionable error when a requested handoff target is
// not permitted by the resolved agent profile.
func (p Policy) ValidateTarget(targetProfileID string) error {
	targetProfileID = strings.TrimSpace(targetProfileID)
	if targetProfileID == "" {
		return fmt.Errorf("handoff target profile is required")
	}
	if p.Allows(targetProfileID) {
		return nil
	}
	if p.ownerProfileID == "" {
		return fmt.Errorf("handoff to profile %q is not allowed", targetProfileID)
	}
	return fmt.Errorf("agent profile %q cannot hand off to profile %q", p.ownerProfileID, targetProfileID)
}
