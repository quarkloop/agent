package prompt

import (
	"strings"
	"testing"
)

func TestBuildSystemPromptUsesBaseBlocksAndAddenda(t *testing.T) {
	got := BuildSystemPrompt(SystemInput{
		BasePrompt:    "  You are Quark Knowledge.  ",
		RuntimeBlocks: []string{"", "Runtime extraction block."},
		Addenda:       []string{"  Service function guidance.  "},
	})

	for _, want := range []string{"You are Quark Knowledge.", "Runtime extraction block.", "Service function guidance."} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "\n\n\n") {
		t.Fatalf("prompt contains empty sections:\n%s", got)
	}
}

func TestBuildSystemPromptFallsBackToEmbeddedSystemPrompt(t *testing.T) {
	previous := GetSystemPrompt()
	SetSystemPrompt("Embedded fallback.")
	t.Cleanup(func() { SetSystemPrompt(previous) })

	got := BuildSystemPrompt(SystemInput{})
	if got != "Embedded fallback." {
		t.Fatalf("prompt = %q, want embedded fallback", got)
	}
}

func TestBuildRuntimeSystemPromptIncludesRuntimeBlocks(t *testing.T) {
	got := BuildRuntimeSystemPrompt("Profile prompt.", nil)
	for _, want := range []string{"Profile prompt.", "Runtime Extraction Profiles", "Workspace Sidecars"} {
		if !strings.Contains(got, want) {
			t.Fatalf("runtime prompt missing %q:\n%s", want, got)
		}
	}
}
