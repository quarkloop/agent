// Package space owns supervisor's semantic view of a space. Persistence of the
// authoritative configuration is delegated to the Space service.
package space

import (
	"time"

	spacemodel "github.com/quarkloop/pkg/space"
)

type Space struct {
	Name       string
	Version    string
	WorkingDir string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type DoctorIssue struct {
	Severity string
	Message  string
}

type DoctorResult struct {
	OK     bool
	Issues []DoctorIssue
}

func FromConfig(cfg spacemodel.Config) *Space {
	return &Space{
		Name:       cfg.Name,
		Version:    cfg.Version,
		WorkingDir: cfg.WorkingDir,
		CreatedAt:  cfg.CreatedAt,
		UpdatedAt:  cfg.UpdatedAt,
	}
}
