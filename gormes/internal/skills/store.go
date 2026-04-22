package skills

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

type Snapshot struct {
	Skills []Skill
}

type Store struct {
	root     string
	maxBytes int
}

type Runtime struct {
	store        *Store
	selectionCap int
	usage        *UsageLogger
}

func NewStore(root string, maxBytes int) *Store {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxDocumentBytes
	}
	return &Store{root: root, maxBytes: maxBytes}
}

func (s *Store) ActiveDir() string {
	if s == nil {
		return ""
	}
	return filepath.Join(s.root, "active")
}

func (s *Store) SnapshotActive() (Snapshot, error) {
	if s == nil {
		return Snapshot{}, nil
	}

	activeDir := s.ActiveDir()
	info, err := os.Stat(activeDir)
	switch {
	case os.IsNotExist(err):
		return Snapshot{}, nil
	case err != nil:
		return Snapshot{}, err
	case !info.IsDir():
		return Snapshot{}, fmt.Errorf("skills: active path %q is not a directory", activeDir)
	}

	var paths []string
	if err := filepath.WalkDir(activeDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Base(path) == "SKILL.md" {
			paths = append(paths, path)
		}
		return nil
	}); err != nil {
		return Snapshot{}, err
	}
	sort.Strings(paths)

	out := Snapshot{Skills: make([]Skill, 0, len(paths))}
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			return Snapshot{}, err
		}
		skill, err := Parse(raw, s.maxBytes)
		if err != nil {
			return Snapshot{}, fmt.Errorf("%s: %w", path, err)
		}
		skill.Path = path
		out.Skills = append(out.Skills, skill)
	}
	return out, nil
}

func NewRuntime(root string, maxBytes, selectionCap int, usageLogPath string) *Runtime {
	if selectionCap <= 0 {
		selectionCap = DefaultSelectionCap
	}
	return &Runtime{
		store:        NewStore(root, maxBytes),
		selectionCap: selectionCap,
		usage:        NewUsageLogger(usageLogPath),
	}
}

func (r *Runtime) BuildSkillBlock(_ context.Context, userMessage string) (string, []string, error) {
	if r == nil || r.store == nil {
		return "", nil, nil
	}
	snapshot, err := r.store.SnapshotActive()
	if err != nil {
		return "", nil, err
	}
	selected := Select(snapshot.Skills, userMessage, r.selectionCap)
	return RenderBlock(selected), skillNames(selected), nil
}

func (r *Runtime) RecordSkillUsage(ctx context.Context, skillNames []string) error {
	if r == nil || r.usage == nil {
		return nil
	}
	return r.usage.Record(ctx, skillNames)
}
