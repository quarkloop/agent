package servicekit

import (
	"fmt"
	"os"

	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
)

// SkillFromFile returns a service skill descriptor sourced from SKILL.md.
func SkillFromFile(name, version, path string) (*servicev1.SkillDescriptor, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read skill %s: %w", path, err)
	}
	return &servicev1.SkillDescriptor{
		Name:     name,
		Version:  version,
		Markdown: string(data),
	}, nil
}
