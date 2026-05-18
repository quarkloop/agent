// Package permissions provides runtime permission enforcement for agents.
// It validates tool calls, file access, and network operations against
// the permissions declared in Quarkfile.
package permissions

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"github.com/quarkloop/runtime/pkg/loop"
)

// ErrToolNotAllowed is returned when a tool is not in the allowed list.
var ErrToolNotAllowed = errors.New("tool not allowed by permissions")

// ErrPathNotAllowed is returned when a file path is not accessible.
var ErrPathNotAllowed = errors.New("path not allowed by permissions")

// ErrPathReadOnly is returned when attempting to write to a read-only path.
var ErrPathReadOnly = errors.New("path is read-only")

// ErrHostNotAllowed is returned when a network host is not accessible.
var ErrHostNotAllowed = errors.New("host not allowed by permissions")

// Policy defines the permission constraints for an agent.
type Policy struct {
	// RestrictTools makes AllowedTools authoritative. Nil policies remain
	// permissive for standalone agents, while supervisor-resolved profiles use
	// restricted policies so an empty permission set denies all tool calls.
	RestrictTools bool

	// Tool permissions
	AllowedTools []string
	DeniedTools  []string

	// Filesystem permissions
	AllowedPaths  []string
	ReadOnlyPaths []string

	// Network permissions
	AllowedHosts []string
	DeniedHosts  []string

	// Audit settings
	LogToolCalls    bool
	LogLLMResponses bool
}

// NewPolicy creates a new policy with no restrictions (permissive by default).
func NewPolicy() *Policy {
	return &Policy{}
}

// Checker validates operations against a permission policy.
type Checker struct {
	policy *Policy
}

// NewChecker creates a new permission checker with the given policy.
func NewChecker(policy *Policy) *Checker {
	return &Checker{policy: policy}
}

// CanUseTool checks if a tool is allowed by the policy.
func (c *Checker) CanUseTool(tool string) bool {
	if c.policy == nil {
		return true
	}

	// Check denied list first
	for _, denied := range c.policy.DeniedTools {
		if matchPattern(denied, tool) {
			return false
		}
	}

	// If allowed list is empty and the policy is not restricted, all tools are
	// allowed. Restricted policies use an empty allowlist to deny all tools.
	if len(c.policy.AllowedTools) == 0 && !c.policy.RestrictTools {
		return true
	}

	// Check if in allowed list
	for _, allowed := range c.policy.AllowedTools {
		if matchPattern(allowed, tool) {
			return true
		}
	}
	return false
}

// CanAccessPath checks if a file path is accessible.
func (c *Checker) CanAccessPath(path string) bool {
	if c.policy == nil {
		return true
	}

	// Normalize path
	path = filepath.Clean(path)

	// If no allowed paths specified, all paths are allowed
	if len(c.policy.AllowedPaths) == 0 {
		return true
	}

	// Check if path is under any allowed path
	for _, allowed := range c.policy.AllowedPaths {
		allowed = filepath.Clean(allowed)
		if isUnderPath(path, allowed) {
			return true
		}
	}
	return false
}

// CanWritePath checks if a file path is writable (not read-only).
func (c *Checker) CanWritePath(path string) bool {
	if c.policy == nil {
		return true
	}

	// First check if path is accessible at all
	if !c.CanAccessPath(path) {
		return false
	}

	// Normalize path
	path = filepath.Clean(path)

	// Check if path is under any read-only path
	for _, ro := range c.policy.ReadOnlyPaths {
		ro = filepath.Clean(ro)
		if isUnderPath(path, ro) {
			return false
		}
	}
	return true
}

// CanAccessHost checks if a network host is accessible.
func (c *Checker) CanAccessHost(host string) bool {
	if c.policy == nil {
		return true
	}

	// Normalize host (remove port if present)
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		host = host[:idx]
	}
	host = strings.ToLower(host)

	// Check denied list first
	for _, denied := range c.policy.DeniedHosts {
		if matchHost(denied, host) {
			return false
		}
	}

	// If allowed list is empty, all hosts are allowed
	if len(c.policy.AllowedHosts) == 0 {
		return true
	}

	// Check if in allowed list
	for _, allowed := range c.policy.AllowedHosts {
		if matchHost(allowed, host) {
			return true
		}
	}
	return false
}

// ValidateTool validates a tool call and returns an error if not allowed.
func (c *Checker) ValidateTool(tool string) error {
	if !c.CanUseTool(tool) {
		return ErrToolNotAllowed
	}
	return nil
}

// ValidatePath validates a file path for reading and returns an error if not allowed.
func (c *Checker) ValidatePath(path string) error {
	if !c.CanAccessPath(path) {
		return ErrPathNotAllowed
	}
	return nil
}

// ValidateWritePath validates a file path for writing and returns an error if not allowed.
func (c *Checker) ValidateWritePath(path string) error {
	if !c.CanAccessPath(path) {
		return ErrPathNotAllowed
	}
	if !c.CanWritePath(path) {
		return ErrPathReadOnly
	}
	return nil
}

// ValidateHost validates a network host and returns an error if not allowed.
func (c *Checker) ValidateHost(host string) error {
	if !c.CanAccessHost(host) {
		return ErrHostNotAllowed
	}
	return nil
}

// ToolPermissionMessage is a message interface for tool calls that can be permission-checked.
type ToolPermissionMessage interface {
	loop.Message
	ToolName() string
}

// ToolMiddleware creates loop middleware that enforces tool permissions.
func ToolMiddleware(checker *Checker) loop.Middleware {
	return func(next loop.HandlerFunc) loop.HandlerFunc {
		return func(ctx context.Context, msg loop.Message) error {
			toolMsg, ok := msg.(ToolPermissionMessage)
			if !ok {
				return next(ctx, msg)
			}

			if err := checker.ValidateTool(toolMsg.ToolName()); err != nil {
				return err
			}

			return next(ctx, msg)
		}
	}
}

// -- Helper functions --

// matchPattern checks if a tool name matches a pattern.
// Patterns can use * as a wildcard.
func matchPattern(pattern, name string) bool {
	if pattern == "*" {
		return true
	}
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(name, prefix)
	}
	if strings.HasPrefix(pattern, "*") {
		suffix := strings.TrimPrefix(pattern, "*")
		return strings.HasSuffix(name, suffix)
	}
	return pattern == name
}

// isUnderPath checks if path is under or equal to base.
func isUnderPath(path, base string) bool {
	if path == base {
		return true
	}
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return false
	}
	return !strings.HasPrefix(rel, "..")
}

// matchHost checks if a host matches a pattern.
// Patterns can use * as a wildcard for subdomains.
func matchHost(pattern, host string) bool {
	pattern = strings.ToLower(pattern)
	if pattern == "*" {
		return true
	}
	if strings.HasPrefix(pattern, "*.") {
		// Wildcard subdomain: *.example.com matches foo.example.com
		suffix := strings.TrimPrefix(pattern, "*")
		return strings.HasSuffix(host, suffix) || host == strings.TrimPrefix(suffix, ".")
	}
	return pattern == host
}
