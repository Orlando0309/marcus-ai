package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// SkillDef is the YAML definition for a skill
type SkillDef struct {
	Name        string            `yaml:"name"`
	Pattern     string            `yaml:"pattern"`
	Description string            `yaml:"description"`
	Args        []string          `yaml:"args,omitempty"`
	Handler     string            `yaml:"handler,omitempty"`
	Run         string            `yaml:"run,omitempty"`
	Timeout     string            `yaml:"timeout,omitempty"`
	Env         map[string]string `yaml:"env,omitempty"`
}

// LoadSkillDef loads a skill definition from a YAML file
func LoadSkillDef(path string) (*SkillDef, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	var def SkillDef
	if err := yaml.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}

	// Validate
	if def.Name == "" {
		return nil, fmt.Errorf("skill name is required")
	}
	if def.Pattern == "" {
		// Default pattern is /name
		def.Pattern = "/" + def.Name
	}
	if def.Description == "" {
		def.Description = fmt.Sprintf("Skill: %s", def.Name)
	}

	return &def, nil
}

// DiscoverSkills finds all skill definitions in a directory
func DiscoverSkills(dir string) ([]SkillDef, error) {
	var skills []SkillDef

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return skills, nil
		}
		return nil, fmt.Errorf("read directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}

		path := filepath.Join(dir, name)
		def, err := LoadSkillDef(path)
		if err != nil {
			// Log but continue with other skills
			continue
		}

		skills = append(skills, *def)
	}

	return skills, nil
}

// IsExternalHandler returns true if the skill uses an external handler
func (d *SkillDef) IsExternalHandler() bool {
	return d.Handler != "" && d.Run == ""
}

// IsShellCommand returns true if the skill runs a shell command
func (d *SkillDef) IsShellCommand() bool {
	return d.Run != ""
}
