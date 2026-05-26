//go:build e2e

package utils

import (
	"strings"
	"testing"
)

func TestValidateGatewayUsageBudget(t *testing.T) {
	if err := validateGatewayUsageBudget(2, 100, 2, 100); err != nil {
		t.Fatalf("within-budget usage rejected: %v", err)
	}
	if err := validateGatewayUsageBudget(3, 100, 2, 100); err == nil || !strings.Contains(err.Error(), "request budget") {
		t.Fatalf("request overage not rejected: %v", err)
	}
	if err := validateGatewayUsageBudget(2, 101, 2, 100); err == nil || !strings.Contains(err.Error(), "token budget") {
		t.Fatalf("token overage not rejected: %v", err)
	}
}

func TestDecodeAndValidateOpenRouterKeyStatus(t *testing.T) {
	status, err := decodeOpenRouterKeyStatus(strings.NewReader(`{"data":{"limit_remaining":12.5,"is_free_tier":true}}`))
	if err != nil {
		t.Fatalf("decode key status: %v", err)
	}
	if err := validateOpenRouterKeyStatus(status); err != nil {
		t.Fatalf("valid credit status rejected: %v", err)
	}
	negative, err := decodeOpenRouterKeyStatus(strings.NewReader(`{"data":{"limit_remaining":-0.1}}`))
	if err != nil {
		t.Fatalf("decode negative key status: %v", err)
	}
	if err := validateOpenRouterKeyStatus(negative); err == nil {
		t.Fatal("negative credit status was accepted")
	}
}
