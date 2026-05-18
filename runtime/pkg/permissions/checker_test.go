package permissions_test

import (
	"testing"

	"github.com/quarkloop/runtime/pkg/permissions"
)

func TestToolPermissions(t *testing.T) {
	tests := []struct {
		name    string
		policy  *permissions.Policy
		tool    string
		allowed bool
	}{
		{
			name:    "nil policy allows all",
			policy:  nil,
			tool:    "bash",
			allowed: true,
		},
		{
			name:    "empty policy allows all",
			policy:  &permissions.Policy{},
			tool:    "bash",
			allowed: true,
		},
		{
			name: "restricted empty policy denies all",
			policy: &permissions.Policy{
				RestrictTools: true,
			},
			tool:    "bash",
			allowed: false,
		},
		{
			name: "allowed tool",
			policy: &permissions.Policy{
				AllowedTools: []string{"bash", "read"},
			},
			tool:    "bash",
			allowed: true,
		},
		{
			name: "not in allowed list",
			policy: &permissions.Policy{
				AllowedTools: []string{"bash", "read"},
			},
			tool:    "write",
			allowed: false,
		},
		{
			name: "denied tool",
			policy: &permissions.Policy{
				DeniedTools: []string{"bash"},
			},
			tool:    "bash",
			allowed: false,
		},
		{
			name: "wildcard allowed",
			policy: &permissions.Policy{
				AllowedTools: []string{"tool-*"},
			},
			tool:    "tool-bash",
			allowed: true,
		},
		{
			name: "wildcard allowed no match",
			policy: &permissions.Policy{
				AllowedTools: []string{"tool-*"},
			},
			tool:    "bash",
			allowed: false,
		},
		{
			name: "denied takes precedence",
			policy: &permissions.Policy{
				AllowedTools: []string{"bash"},
				DeniedTools:  []string{"bash"},
			},
			tool:    "bash",
			allowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := permissions.NewChecker(tt.policy)
			got := checker.CanUseTool(tt.tool)
			if got != tt.allowed {
				t.Errorf("CanUseTool(%q) = %v, want %v", tt.tool, got, tt.allowed)
			}
		})
	}
}

func TestPathPermissions(t *testing.T) {
	tests := []struct {
		name    string
		policy  *permissions.Policy
		path    string
		allowed bool
	}{
		{
			name:    "nil policy allows all",
			policy:  nil,
			path:    "/etc/passwd",
			allowed: true,
		},
		{
			name:    "empty policy allows all",
			policy:  &permissions.Policy{},
			path:    "/etc/passwd",
			allowed: true,
		},
		{
			name: "path under allowed",
			policy: &permissions.Policy{
				AllowedPaths: []string{"/home/user"},
			},
			path:    "/home/user/file.txt",
			allowed: true,
		},
		{
			name: "exact path allowed",
			policy: &permissions.Policy{
				AllowedPaths: []string{"/home/user"},
			},
			path:    "/home/user",
			allowed: true,
		},
		{
			name: "path not under allowed",
			policy: &permissions.Policy{
				AllowedPaths: []string{"/home/user"},
			},
			path:    "/etc/passwd",
			allowed: false,
		},
		{
			name: "multiple allowed paths",
			policy: &permissions.Policy{
				AllowedPaths: []string{"/home", "/tmp"},
			},
			path:    "/tmp/file",
			allowed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := permissions.NewChecker(tt.policy)
			got := checker.CanAccessPath(tt.path)
			if got != tt.allowed {
				t.Errorf("CanAccessPath(%q) = %v, want %v", tt.path, got, tt.allowed)
			}
		})
	}
}

func TestWritePathPermissions(t *testing.T) {
	tests := []struct {
		name     string
		policy   *permissions.Policy
		path     string
		writable bool
	}{
		{
			name: "read-only path",
			policy: &permissions.Policy{
				AllowedPaths:  []string{"/home"},
				ReadOnlyPaths: []string{"/home/readonly"},
			},
			path:     "/home/readonly/file.txt",
			writable: false,
		},
		{
			name: "writable path",
			policy: &permissions.Policy{
				AllowedPaths:  []string{"/home"},
				ReadOnlyPaths: []string{"/home/readonly"},
			},
			path:     "/home/writable/file.txt",
			writable: true,
		},
		{
			name: "path not allowed at all",
			policy: &permissions.Policy{
				AllowedPaths: []string{"/home"},
			},
			path:     "/etc/passwd",
			writable: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := permissions.NewChecker(tt.policy)
			got := checker.CanWritePath(tt.path)
			if got != tt.writable {
				t.Errorf("CanWritePath(%q) = %v, want %v", tt.path, got, tt.writable)
			}
		})
	}
}

func TestHostPermissions(t *testing.T) {
	tests := []struct {
		name    string
		policy  *permissions.Policy
		host    string
		allowed bool
	}{
		{
			name:    "nil policy allows all",
			policy:  nil,
			host:    "example.com",
			allowed: true,
		},
		{
			name:    "empty policy allows all",
			policy:  &permissions.Policy{},
			host:    "example.com",
			allowed: true,
		},
		{
			name: "allowed host",
			policy: &permissions.Policy{
				AllowedHosts: []string{"example.com"},
			},
			host:    "example.com",
			allowed: true,
		},
		{
			name: "not in allowed list",
			policy: &permissions.Policy{
				AllowedHosts: []string{"example.com"},
			},
			host:    "evil.com",
			allowed: false,
		},
		{
			name: "denied host",
			policy: &permissions.Policy{
				DeniedHosts: []string{"evil.com"},
			},
			host:    "evil.com",
			allowed: false,
		},
		{
			name: "wildcard subdomain",
			policy: &permissions.Policy{
				AllowedHosts: []string{"*.example.com"},
			},
			host:    "api.example.com",
			allowed: true,
		},
		{
			name: "wildcard includes base domain",
			policy: &permissions.Policy{
				AllowedHosts: []string{"*.example.com"},
			},
			host:    "example.com",
			allowed: true,
		},
		{
			name: "host with port stripped",
			policy: &permissions.Policy{
				AllowedHosts: []string{"example.com"},
			},
			host:    "example.com:8080",
			allowed: true,
		},
		{
			name: "denied takes precedence",
			policy: &permissions.Policy{
				AllowedHosts: []string{"example.com"},
				DeniedHosts:  []string{"example.com"},
			},
			host:    "example.com",
			allowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := permissions.NewChecker(tt.policy)
			got := checker.CanAccessHost(tt.host)
			if got != tt.allowed {
				t.Errorf("CanAccessHost(%q) = %v, want %v", tt.host, got, tt.allowed)
			}
		})
	}
}

func TestValidateErrors(t *testing.T) {
	policy := &permissions.Policy{
		AllowedTools:  []string{"bash"},
		AllowedPaths:  []string{"/home"},
		ReadOnlyPaths: []string{"/home/readonly"},
		AllowedHosts:  []string{"example.com"},
	}
	checker := permissions.NewChecker(policy)

	// Tool not allowed
	err := checker.ValidateTool("write")
	if err != permissions.ErrToolNotAllowed {
		t.Errorf("expected ErrToolNotAllowed, got %v", err)
	}

	// Path not allowed
	err = checker.ValidatePath("/etc/passwd")
	if err != permissions.ErrPathNotAllowed {
		t.Errorf("expected ErrPathNotAllowed, got %v", err)
	}

	// Write to read-only path
	err = checker.ValidateWritePath("/home/readonly/file.txt")
	if err != permissions.ErrPathReadOnly {
		t.Errorf("expected ErrPathReadOnly, got %v", err)
	}

	// Host not allowed
	err = checker.ValidateHost("evil.com")
	if err != permissions.ErrHostNotAllowed {
		t.Errorf("expected ErrHostNotAllowed, got %v", err)
	}
}
