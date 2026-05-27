//go:build e2e

package utils

import (
	"slices"
	"testing"
)

func TestComposeStartupContainersDeclareServiceInfrastructure(t *testing.T) {
	got := composeStartupContainers([]string{"io", "indexer", "secrets", "workflow", "indexer"})
	want := []string{"io", "indexer", "secrets", "workflow", "dgraph", "openbao", "temporal"}
	if !slices.Equal(got, want) {
		t.Fatalf("startup containers = %v, want %v", got, want)
	}
}

func TestComposeStartupContainersDoNotAddUnrequestedInfrastructure(t *testing.T) {
	got := composeStartupContainers([]string{"io", "gateway"})
	want := []string{"io", "gateway"}
	if !slices.Equal(got, want) {
		t.Fatalf("startup containers = %v, want %v", got, want)
	}
}

func TestComposeServicesAlwaysIncludeRuntimeHarness(t *testing.T) {
	got := composeServicesFor(StartOptions{DisableKnowledgeServices: true}, false)
	want := []string{"io", "harness"}
	if !slices.Equal(got, want) {
		t.Fatalf("focused runtime services = %v, want mandatory infrastructure %v", got, want)
	}
}

func TestProviderRequestBudgetDefaultsToFreeTierDailyCeiling(t *testing.T) {
	if got := int64Env("QUARK_E2E_MAX_PROVIDER_REQUESTS", 50); got != 50 {
		t.Fatalf("default provider request limit = %d, want 50", got)
	}
}
