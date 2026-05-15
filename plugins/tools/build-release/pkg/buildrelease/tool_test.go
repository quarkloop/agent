package buildrelease

import (
	"testing"

	"github.com/quarkloop/pkg/toolkit"
)

func TestLegacyToolMapsReleaseRequestToServiceRunnerContract(t *testing.T) {
	req, err := releaseRequestFromInput(toolkit.Input{
		Args: map[string]string{"config": "release.json"},
		Flags: map[string]any{
			"version":    "v1.2.3",
			"parallel":   2,
			"skip-tests": true,
		},
	})
	if err != nil {
		t.Fatalf("release request: %v", err)
	}
	if req.WorkingDir == "" || req.ConfigPath != "release.json" || req.Version != "v1.2.3" || req.Parallelism != 2 || !req.SkipTests {
		t.Fatalf("release request = %+v", req)
	}
}

func TestLegacyToolMapsDryRunRequestToServiceRunnerContract(t *testing.T) {
	req, err := dryRunRequestFromInput(toolkit.Input{
		Args: map[string]string{"config": ""},
		Flags: map[string]any{
			"version":  "v1.2.3",
			"parallel": 3,
		},
	})
	if err != nil {
		t.Fatalf("dry run request: %v", err)
	}
	if req.WorkingDir == "" || req.ConfigPath != "build_release.json" || req.Version != "v1.2.3" || req.Parallelism != 3 {
		t.Fatalf("dry run request = %+v", req)
	}
}
