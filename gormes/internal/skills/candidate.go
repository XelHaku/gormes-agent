package skills

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

type CandidateStatus string

const (
	CandidateStatusCandidate CandidateStatus = "candidate"
	CandidateStatusPromoted  CandidateStatus = "promoted"
	CandidateStatusRejected  CandidateStatus = "rejected"

	ActiveStatus = "active"
)

type CandidateDraft struct {
	Slug            string
	Goal            string
	Summary         string
	SourceRunID     string
	ParentSessionID string
	ChildAgentID    string
	ToolNames       []string
}

type CandidateMetadata struct {
	CandidateID     string          `json:"candidate_id"`
	Slug            string          `json:"slug"`
	SourceRunID     string          `json:"source_run_id"`
	ParentSessionID string          `json:"parent_session_id,omitempty"`
	ChildAgentID    string          `json:"child_agent_id,omitempty"`
	ToolNames       []string        `json:"tool_names,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	PromotedAt      *time.Time      `json:"promoted_at,omitempty"`
	Status          CandidateStatus `json:"status"`
}

type ActiveMetadata struct {
	Slug              string    `json:"slug"`
	Title             string    `json:"title"`
	Version           string    `json:"version"`
	CreatedAt         time.Time `json:"created_at"`
	PromotedAt        time.Time `json:"promoted_at"`
	SourceCandidateID string    `json:"source_candidate_id"`
	Status            string    `json:"status"`
}

func (s *Store) CandidateDir() string {
	if s == nil {
		return ""
	}
	return filepath.Join(s.root, "candidates")
}

func (s *Store) DraftCandidate(draft CandidateDraft) (CandidateMetadata, error) {
	if s == nil {
		return CandidateMetadata{}, fmt.Errorf("skills: nil store")
	}
	if strings.TrimSpace(draft.SourceRunID) == "" {
		return CandidateMetadata{}, fmt.Errorf("skills: source_run_id is required")
	}

	slug := normalizeSlug(draft.Slug)
	if slug == "" {
		slug = normalizeSlug(draft.Goal)
	}
	if slug == "" {
		slug = "delegated-run"
	}

	skill := Skill{
		Name:        slug,
		Description: candidateDescription(draft),
		Body:        candidateBody(draft),
	}
	if err := skill.Validate(s.maxBytes); err != nil {
		return CandidateMetadata{}, err
	}

	createdAt := time.Now().UTC()
	candidateID := fmt.Sprintf("%s-%s-%s", createdAt.Format("20060102T150405Z"), normalizeSlug(draft.SourceRunID), slug)

	meta := CandidateMetadata{
		CandidateID:     candidateID,
		Slug:            slug,
		SourceRunID:     strings.TrimSpace(draft.SourceRunID),
		ParentSessionID: strings.TrimSpace(draft.ParentSessionID),
		ChildAgentID:    strings.TrimSpace(draft.ChildAgentID),
		ToolNames:       append([]string(nil), draft.ToolNames...),
		CreatedAt:       createdAt,
		Status:          CandidateStatusCandidate,
	}

	dir := filepath.Join(s.CandidateDir(), meta.CandidateID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return CandidateMetadata{}, err
	}
	if err := writeJSON(filepath.Join(dir, "meta.json"), meta); err != nil {
		return CandidateMetadata{}, err
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(RenderDocument(skill)), 0o644); err != nil {
		return CandidateMetadata{}, err
	}

	return meta, nil
}

func (s *Store) PromoteCandidate(candidateID string) (ActiveMetadata, error) {
	if s == nil {
		return ActiveMetadata{}, fmt.Errorf("skills: nil store")
	}
	metaPath := filepath.Join(s.CandidateDir(), candidateID, "meta.json")
	var meta CandidateMetadata
	if err := readJSON(metaPath, &meta); err != nil {
		return ActiveMetadata{}, err
	}

	skillRaw, err := os.ReadFile(filepath.Join(s.CandidateDir(), candidateID, "SKILL.md"))
	if err != nil {
		return ActiveMetadata{}, err
	}
	skill, err := Parse(skillRaw, s.maxBytes)
	if err != nil {
		return ActiveMetadata{}, err
	}

	activeDir := filepath.Join(s.ActiveDir(), meta.Slug)
	if _, err := os.Stat(activeDir); err == nil {
		return ActiveMetadata{}, fmt.Errorf("skills: active skill %q already exists", meta.Slug)
	} else if !os.IsNotExist(err) {
		return ActiveMetadata{}, err
	}
	if err := os.MkdirAll(activeDir, 0o755); err != nil {
		return ActiveMetadata{}, err
	}

	now := time.Now().UTC()
	activeMeta := ActiveMetadata{
		Slug:              meta.Slug,
		Title:             meta.Slug,
		Version:           "1.0.0",
		CreatedAt:         meta.CreatedAt,
		PromotedAt:        now,
		SourceCandidateID: meta.CandidateID,
		Status:            ActiveStatus,
	}
	if err := writeJSON(filepath.Join(activeDir, "meta.json"), activeMeta); err != nil {
		return ActiveMetadata{}, err
	}
	if err := os.WriteFile(filepath.Join(activeDir, "SKILL.md"), []byte(RenderDocument(skill)), 0o644); err != nil {
		return ActiveMetadata{}, err
	}

	meta.Status = CandidateStatusPromoted
	meta.PromotedAt = &now
	if err := writeJSON(metaPath, meta); err != nil {
		return ActiveMetadata{}, err
	}
	return activeMeta, nil
}

func candidateDescription(draft CandidateDraft) string {
	if summary := strings.TrimSpace(draft.Summary); summary != "" {
		return summary
	}
	if goal := strings.TrimSpace(draft.Goal); goal != "" {
		return "Delegated run pattern for " + goal
	}
	return "Delegated run pattern"
}

func candidateBody(draft CandidateDraft) string {
	var b strings.Builder
	b.WriteString("Use this reviewed delegated pattern for work like:\n\n")
	if goal := strings.TrimSpace(draft.Goal); goal != "" {
		b.WriteString("- Goal: ")
		b.WriteString(goal)
		b.WriteString("\n")
	}
	if summary := strings.TrimSpace(draft.Summary); summary != "" {
		b.WriteString("- Successful result: ")
		b.WriteString(summary)
		b.WriteString("\n")
	}
	if len(draft.ToolNames) > 0 {
		b.WriteString("- Preferred tools:\n")
		for _, name := range draft.ToolNames {
			if name = strings.TrimSpace(name); name != "" {
				b.WriteString("  - ")
				b.WriteString(name)
				b.WriteString("\n")
			}
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func normalizeSlug(in string) string {
	in = strings.TrimSpace(strings.ToLower(in))
	if in == "" {
		return ""
	}
	var b strings.Builder
	lastDash := false
	for _, r := range in {
		switch {
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			b.WriteRune(r)
			lastDash = false
		case !lastDash:
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func writeJSON(path string, v any) error {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func readJSON(path string, out any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}
