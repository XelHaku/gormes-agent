package skills

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Hub struct {
	store *Store
}

type HubInstallMetadata struct {
	Source      string    `json:"source"`
	Ref         string    `json:"ref"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	InstalledAt time.Time `json:"installed_at"`
}

type InstalledSkill struct {
	Name        string
	Description string
	Ref         string
	Path        string
}

func NewHub(root string, maxBytes int) *Hub {
	return &Hub{store: NewStore(root, maxBytes)}
}

func (h *Hub) CatalogsDir() string {
	if h == nil || h.store == nil {
		return ""
	}
	return filepath.Join(h.store.root, ".hub", "catalogs")
}

func (h *Hub) CacheDir() string {
	if h == nil || h.store == nil {
		return ""
	}
	return filepath.Join(h.store.root, ".hub", "cache")
}

func (h *Hub) LockPath() string {
	if h == nil || h.store == nil {
		return ""
	}
	return filepath.Join(h.store.root, ".hub", "lock.json")
}

func (h *Hub) SyncLocalCatalogs(ctx context.Context, generatedAt time.Time) (HubLockFile, error) {
	registry, err := h.localCatalogRegistry()
	if err != nil {
		return HubLockFile{}, err
	}
	return h.Sync(ctx, registry, generatedAt)
}

func (h *Hub) Sync(ctx context.Context, registry *SourceRegistry, generatedAt time.Time) (HubLockFile, error) {
	if h == nil || h.store == nil {
		return HubLockFile{}, nil
	}
	if registry == nil {
		var err error
		registry, err = NewSourceRegistry()
		if err != nil {
			return HubLockFile{}, err
		}
	}

	lock, err := registry.Lock(ctx, generatedAt)
	if err != nil {
		return HubLockFile{}, err
	}

	if err := os.RemoveAll(h.CacheDir()); err != nil {
		return HubLockFile{}, err
	}
	if err := os.MkdirAll(filepath.Dir(h.LockPath()), 0o755); err != nil {
		return HubLockFile{}, err
	}

	for i := range lock.Skills {
		if strings.TrimSpace(lock.Skills[i].Path) == "" {
			continue
		}
		dst := filepath.Join(h.CacheDir(), filepath.FromSlash(lock.Skills[i].Ref), "SKILL.md")
		if err := copySkillFile(lock.Skills[i].Path, dst); err != nil {
			return HubLockFile{}, err
		}
		lock.Skills[i].Path = dst
	}

	if err := writeJSON(h.LockPath(), lock); err != nil {
		return HubLockFile{}, err
	}
	return lock, nil
}

func (h *Hub) LoadLock() (HubLockFile, error) {
	if h == nil || h.store == nil {
		return HubLockFile{}, nil
	}
	var lock HubLockFile
	if err := readJSON(h.LockPath(), &lock); err != nil {
		if os.IsNotExist(err) {
			return HubLockFile{}, nil
		}
		return HubLockFile{}, err
	}
	sortSkillMeta(lock.Skills)
	sortSkillBundles(lock.Bundles)
	return lock, nil
}

func (h *Hub) ListHub() ([]SkillMeta, error) {
	lock, err := h.LoadLock()
	if err != nil {
		return nil, err
	}
	out := append([]SkillMeta(nil), lock.Skills...)
	sortSkillMeta(out)
	return out, nil
}

func (h *Hub) Install(ref string) (HubInstallMetadata, error) {
	if h == nil || h.store == nil {
		return HubInstallMetadata{}, fmt.Errorf("skills: nil hub")
	}
	lock, err := h.LoadLock()
	if err != nil {
		return HubInstallMetadata{}, err
	}
	if len(lock.Skills) == 0 {
		return HubInstallMetadata{}, fmt.Errorf("skills: hub is empty; run sync first")
	}

	ref = strings.TrimSpace(ref)
	var selected *SkillMeta
	for i := range lock.Skills {
		if lock.Skills[i].Ref == ref {
			selected = &lock.Skills[i]
			break
		}
	}
	if selected == nil {
		return HubInstallMetadata{}, fmt.Errorf("skills: hub ref %q not found", ref)
	}
	if strings.TrimSpace(selected.Path) == "" {
		return HubInstallMetadata{}, fmt.Errorf("skills: hub ref %q is metadata-only and cannot be installed yet", ref)
	}

	raw, err := os.ReadFile(selected.Path)
	if err != nil {
		return HubInstallMetadata{}, err
	}
	skill, err := Parse(raw, h.store.maxBytes)
	if err != nil {
		return HubInstallMetadata{}, err
	}

	rel, err := installRelativePath(*selected)
	if err != nil {
		return HubInstallMetadata{}, err
	}
	activeDir := filepath.Join(h.store.ActiveDir(), filepath.FromSlash(rel))
	if _, err := os.Stat(activeDir); err == nil {
		return HubInstallMetadata{}, fmt.Errorf("skills: active skill path %q already exists", activeDir)
	} else if !os.IsNotExist(err) {
		return HubInstallMetadata{}, err
	}
	if err := os.MkdirAll(activeDir, 0o755); err != nil {
		return HubInstallMetadata{}, err
	}

	skillPath := filepath.Join(activeDir, "SKILL.md")
	if err := os.WriteFile(skillPath, raw, 0o644); err != nil {
		return HubInstallMetadata{}, err
	}

	meta := HubInstallMetadata{
		Source:      selected.Source,
		Ref:         selected.Ref,
		Name:        skill.Name,
		Description: skill.Description,
		InstalledAt: time.Now().UTC(),
	}
	if err := writeJSON(filepath.Join(activeDir, "hub.json"), meta); err != nil {
		return HubInstallMetadata{}, err
	}
	return meta, nil
}

func (h *Hub) ListInstalled() ([]InstalledSkill, error) {
	if h == nil || h.store == nil {
		return nil, nil
	}
	snapshot, err := h.store.SnapshotActive()
	if err != nil {
		return nil, err
	}
	out := make([]InstalledSkill, 0, len(snapshot.Skills))
	for _, skill := range snapshot.Skills {
		item := InstalledSkill{
			Name:        skill.Name,
			Description: skill.Description,
			Path:        skill.Path,
		}
		var meta HubInstallMetadata
		if err := readJSON(filepath.Join(filepath.Dir(skill.Path), "hub.json"), &meta); err == nil {
			item.Ref = meta.Ref
		} else if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Ref != out[j].Ref {
			return out[i].Ref < out[j].Ref
		}
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].Path < out[j].Path
	})
	return out, nil
}

func (h *Hub) localCatalogRegistry() (*SourceRegistry, error) {
	if h == nil || h.store == nil {
		return NewSourceRegistry()
	}

	sources := make([]SkillSource, 0, len(BuiltinSkillRegistryBundles()))
	for _, bundle := range BuiltinSkillRegistryBundles() {
		readiness := bundle.Readiness
		if readiness == "" {
			if bundle.Name == "bundled" {
				readiness = SkillReadinessReady
			} else {
				readiness = SkillReadinessAvailable
			}
		}
		sources = append(sources, NewFilesystemSource(FilesystemSourceConfig{
			Name:             bundle.Name,
			Title:            bundle.Title,
			Prefix:           bundle.Name,
			Root:             filepath.Join(h.CatalogsDir(), bundle.Name),
			Optional:         bundle.Optional,
			Readiness:        readiness,
			MaxDocumentBytes: h.store.maxBytes,
		}))
	}
	return NewSourceRegistry(sources...)
}

func installRelativePath(meta SkillMeta) (string, error) {
	ref := strings.TrimSpace(meta.Ref)
	source := strings.TrimSpace(meta.Source)
	if ref == "" {
		return "", fmt.Errorf("skills: install ref is required")
	}
	if source == "" {
		parts := strings.Split(ref, "/")
		if len(parts) < 2 {
			return "", fmt.Errorf("skills: install ref %q must include source and skill path", ref)
		}
		source = parts[0]
	}
	prefix := source + "/"
	if !strings.HasPrefix(ref, prefix) {
		return "", fmt.Errorf("skills: install ref %q does not match source %q", ref, source)
	}
	rel := strings.TrimPrefix(ref, prefix)
	if strings.TrimSpace(rel) == "" {
		return "", fmt.Errorf("skills: install ref %q has no skill path", ref)
	}
	return rel, nil
}

func copySkillFile(src, dst string) error {
	raw, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, raw, 0o644)
}
