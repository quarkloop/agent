package hierarchy_test

import (
	"testing"

	"github.com/quarkloop/runtime/pkg/hierarchy"
)

func TestRegistryMain(t *testing.T) {
	r := hierarchy.NewRegistry()

	entry, err := r.RegisterMain("main-1", "Main Agent", "The supervisor", nil)
	if err != nil {
		t.Fatalf("failed to register main: %v", err)
	}

	if entry.Identity.ID != "main-1" {
		t.Errorf("expected ID main-1, got %s", entry.Identity.ID)
	}
	if entry.Identity.Role != hierarchy.RoleMain {
		t.Errorf("expected role main_agent, got %s", entry.Identity.Role)
	}
	if !entry.Identity.IsRoot() {
		t.Error("main agent should be root")
	}

	// Cannot register twice
	_, err = r.RegisterMain("main-2", "Another Main", "desc", nil)
	if err == nil {
		t.Error("expected error registering second main agent")
	}
}

func TestRegistryMainProfile(t *testing.T) {
	r := hierarchy.NewRegistry()

	entry, err := r.RegisterMainWithProfile("main-1", "Main Agent", "The supervisor", "quark-knowledge", nil)
	if err != nil {
		t.Fatalf("failed to register main: %v", err)
	}
	if entry.Identity.ProfileID != "quark-knowledge" {
		t.Fatalf("profile id = %q", entry.Identity.ProfileID)
	}
}

func TestRegistrySpawn(t *testing.T) {
	r := hierarchy.NewRegistry()
	r.RegisterMain("main", "Main", "desc", hierarchy.DefaultPermissions())

	config := &hierarchy.SpawnConfig{
		Name:        "Worker",
		Description: "A worker agent",
		Task:        "Do something",
	}

	entry, err := r.Spawn("main", config)
	if err != nil {
		t.Fatalf("failed to spawn: %v", err)
	}

	if entry.Identity.Role != hierarchy.RoleSub {
		t.Errorf("expected role sub_agent, got %s", entry.Identity.Role)
	}
	if entry.Identity.ParentID != "main" {
		t.Errorf("expected parent main, got %s", entry.Identity.ParentID)
	}

	// Check tree
	children := r.Children("main")
	if len(children) != 1 {
		t.Errorf("expected 1 child, got %d", len(children))
	}
}

func TestRegistrySpawnProfile(t *testing.T) {
	r := hierarchy.NewRegistry()
	r.RegisterMain("main", "Main", "desc", hierarchy.DefaultPermissions())

	entry, err := r.Spawn("main", &hierarchy.SpawnConfig{
		Name:      "Worker",
		Task:      "Do something",
		ProfileID: "quark-devops",
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if entry.Identity.ProfileID != "quark-devops" {
		t.Fatalf("profile id = %q", entry.Identity.ProfileID)
	}
}

func TestRegistrySpawnValidation(t *testing.T) {
	r := hierarchy.NewRegistry()
	r.RegisterMain("main", "Main", "desc", hierarchy.DefaultPermissions())

	// Missing name
	_, err := r.Spawn("main", &hierarchy.SpawnConfig{Task: "do something"})
	if err == nil {
		t.Error("expected error for missing name")
	}

	// Missing task
	_, err = r.Spawn("main", &hierarchy.SpawnConfig{Name: "Worker"})
	if err == nil {
		t.Error("expected error for missing task")
	}

	// Invalid parent
	_, err = r.Spawn("nonexistent", &hierarchy.SpawnConfig{Name: "W", Task: "T"})
	if err == nil {
		t.Error("expected error for invalid parent")
	}
}

func TestRegistryMaxSubAgents(t *testing.T) {
	r := hierarchy.NewRegistry()
	r.RegisterMain("main", "Main", "desc", &hierarchy.Permissions{
		CanSpawnAgents: true,
		MaxSubAgents:   2,
	})

	// First two should succeed
	r.Spawn("main", &hierarchy.SpawnConfig{Name: "W1", Task: "T"})
	r.Spawn("main", &hierarchy.SpawnConfig{Name: "W2", Task: "T"})

	// Third should fail
	_, err := r.Spawn("main", &hierarchy.SpawnConfig{Name: "W3", Task: "T"})
	if err == nil {
		t.Error("expected error for exceeding max sub-agents")
	}
}

func TestRegistryNoSpawnPermission(t *testing.T) {
	r := hierarchy.NewRegistry()
	r.RegisterMain("main", "Main", "desc", &hierarchy.Permissions{
		CanSpawnAgents: false,
	})

	_, err := r.Spawn("main", &hierarchy.SpawnConfig{Name: "W1", Task: "T"})
	if err == nil {
		t.Error("expected error when parent cannot spawn")
	}
}

func TestRegistryDescendants(t *testing.T) {
	r := hierarchy.NewRegistry()
	r.RegisterMain("main", "Main", "desc", hierarchy.DefaultPermissions())

	child1, _ := r.Spawn("main", &hierarchy.SpawnConfig{
		ID:          "child1",
		Name:        "Child1",
		Task:        "T",
		Permissions: hierarchy.DefaultPermissions(),
	})
	r.Spawn("main", &hierarchy.SpawnConfig{ID: "child2", Name: "Child2", Task: "T"})
	r.Spawn(child1.Identity.ID, &hierarchy.SpawnConfig{ID: "grandchild", Name: "GC", Task: "T"})

	descendants := r.Descendants("main")
	if len(descendants) != 3 {
		t.Errorf("expected 3 descendants, got %d", len(descendants))
	}

	descendants = r.Descendants("child1")
	if len(descendants) != 1 {
		t.Errorf("expected 1 descendant of child1, got %d", len(descendants))
	}
}

func TestRegistryAncestors(t *testing.T) {
	r := hierarchy.NewRegistry()
	r.RegisterMain("main", "Main", "desc", hierarchy.DefaultPermissions())
	child, _ := r.Spawn("main", &hierarchy.SpawnConfig{
		ID:          "child",
		Name:        "Child",
		Task:        "T",
		Permissions: hierarchy.DefaultPermissions(),
	})
	grandchild, _ := r.Spawn(child.Identity.ID, &hierarchy.SpawnConfig{
		ID:   "grandchild",
		Name: "GC",
		Task: "T",
	})

	ancestors := r.Ancestors(grandchild.Identity.ID)
	if len(ancestors) != 2 {
		t.Errorf("expected 2 ancestors, got %d", len(ancestors))
	}
	if ancestors[0] != "child" {
		t.Errorf("expected first ancestor to be child, got %s", ancestors[0])
	}
	if ancestors[1] != "main" {
		t.Errorf("expected second ancestor to be main, got %s", ancestors[1])
	}
}

func TestRegistryRemove(t *testing.T) {
	r := hierarchy.NewRegistry()
	r.RegisterMain("main", "Main", "desc", hierarchy.DefaultPermissions())
	child, _ := r.Spawn("main", &hierarchy.SpawnConfig{
		ID:          "child",
		Name:        "Child",
		Task:        "T",
		Permissions: hierarchy.DefaultPermissions(),
	})
	r.Spawn(child.Identity.ID, &hierarchy.SpawnConfig{ID: "grandchild", Name: "GC", Task: "T"})

	// Should have 3 agents
	if r.Count() != 3 {
		t.Errorf("expected 3 agents, got %d", r.Count())
	}

	// Remove child (and grandchild)
	if !r.Remove("child") {
		t.Error("remove should succeed")
	}

	// Should have 1 agent (main only)
	if r.Count() != 1 {
		t.Errorf("expected 1 agent after removal, got %d", r.Count())
	}

	// Cannot remove main
	if r.Remove("main") {
		t.Error("should not be able to remove main agent")
	}
}

func TestRegistryStatus(t *testing.T) {
	r := hierarchy.NewRegistry()
	r.RegisterMain("main", "Main", "desc", nil)

	entry, ok := r.Get("main")
	if !ok {
		t.Fatal("expected main agent to exist")
	}
	if entry.Status != hierarchy.StatusPending {
		t.Errorf("expected pending status, got %s", entry.Status)
	}

	r.SetStatus("main", hierarchy.StatusRunning)
	entry, ok = r.Get("main")
	if !ok {
		t.Fatal("expected main agent to exist")
	}
	if entry.Status != hierarchy.StatusRunning {
		t.Errorf("expected running status, got %s", entry.Status)
	}
	if entry.StartedAt.IsZero() {
		t.Error("StartedAt should be set when running")
	}

	r.SetStatus("main", hierarchy.StatusComplete)
	entry, ok = r.Get("main")
	if !ok {
		t.Fatal("expected main agent to exist")
	}
	if entry.CompletedAt.IsZero() {
		t.Error("CompletedAt should be set when complete")
	}
}

func TestRegistryDepth(t *testing.T) {
	r := hierarchy.NewRegistry()
	r.RegisterMain("main", "Main", "desc", hierarchy.DefaultPermissions())
	child, _ := r.Spawn("main", &hierarchy.SpawnConfig{
		ID:          "child",
		Name:        "Child",
		Task:        "T",
		Permissions: hierarchy.DefaultPermissions(),
	})
	grandchild, _ := r.Spawn(child.Identity.ID, &hierarchy.SpawnConfig{
		ID:   "grandchild",
		Name: "GC",
		Task: "T",
	})

	if r.Depth("main") != 0 {
		t.Errorf("expected depth 0 for main, got %d", r.Depth("main"))
	}
	if r.Depth("child") != 1 {
		t.Errorf("expected depth 1 for child, got %d", r.Depth("child"))
	}
	if r.Depth(grandchild.Identity.ID) != 2 {
		t.Errorf("expected depth 2 for grandchild, got %d", r.Depth(grandchild.Identity.ID))
	}
}
