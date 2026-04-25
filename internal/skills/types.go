package skills

import (
	"fmt"
	"strings"
)

const (
	DefaultMaxDocumentBytes = 64 * 1024
	DefaultSelectionCap     = 3
)

// Skill is the typed in-memory representation of one SKILL.md artifact.
type Skill struct {
	Name        string
	Description string
	Body        string
	Path        string
	RawBytes    int

	Platforms       []string
	RequiredEnvVars []string
}

// Validate enforces the minimal Phase 2.G0 contract for a parsed skill.
func (s Skill) Validate(maxBytes int) error {
	if strings.TrimSpace(s.Name) == "" {
		return fmt.Errorf("skill name is required")
	}
	if strings.TrimSpace(s.Description) == "" {
		return fmt.Errorf("skill description is required")
	}
	if strings.TrimSpace(s.Body) == "" {
		return fmt.Errorf("skill body is required")
	}

	size := s.RawBytes
	if size == 0 {
		size = len(s.Name) + len(s.Description) + len(s.Body)
	}
	if maxBytes > 0 && size > maxBytes {
		return fmt.Errorf("skill document too large: %d > %d bytes", size, maxBytes)
	}
	return nil
}
