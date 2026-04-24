package architectureplanner

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/progress"
)

type SourceRoot struct {
	Name      string   `json:"name"`
	Path      string   `json:"path"`
	Exists    bool     `json:"exists"`
	FileCount int      `json:"file_count"`
	Samples   []string `json:"samples,omitempty"`
}

type ContextBundle struct {
	GeneratedUTC            string                  `json:"generated_utc"`
	RepoRoot                string                  `json:"repo_root"`
	ProgressJSON            string                  `json:"progress_json"`
	ProgressStats           ProgressInfo            `json:"progress_stats"`
	SourceRoots             []SourceRoot            `json:"source_roots"`
	SyncResults             []RepoSyncResult        `json:"sync_results,omitempty"`
	ImplementationInventory ImplementationInventory `json:"implementation_inventory"`
}

type ProgressInfo struct {
	Items      int `json:"items"`
	Planned    int `json:"planned"`
	InProgress int `json:"in_progress"`
	Complete   int `json:"complete"`
}

type ImplementationInventory struct {
	Commands         []string   `json:"commands"`
	InternalPackages []string   `json:"internal_packages"`
	BuildingDocs     []string   `json:"building_docs"`
	LandingSite      SourceRoot `json:"landing_site"`
	HugoDocs         SourceRoot `json:"hugo_docs"`
}

func CollectContext(cfg Config, now time.Time) (ContextBundle, error) {
	progressInfo := ProgressInfo{}
	if p, err := progress.Load(cfg.ProgressJSON); err == nil {
		stats := p.Stats()
		progressInfo = ProgressInfo{
			Items:      stats.Items.Total,
			Planned:    stats.Items.Planned,
			InProgress: stats.Items.InProgress,
			Complete:   stats.Items.Complete,
		}
	} else {
		return ContextBundle{}, err
	}

	roots := cfg.SourceRoots()
	for i := range roots {
		if err := enrichSourceRoot(&roots[i]); err != nil {
			return ContextBundle{}, err
		}
	}

	inventory, err := collectImplementationInventory(cfg)
	if err != nil {
		return ContextBundle{}, err
	}

	return ContextBundle{
		GeneratedUTC:            now.UTC().Format(time.RFC3339),
		RepoRoot:                cfg.RepoRoot,
		ProgressJSON:            cfg.ProgressJSON,
		ProgressStats:           progressInfo,
		SourceRoots:             roots,
		ImplementationInventory: inventory,
	}, nil
}

func writeContext(path string, bundle ContextBundle) error {
	data, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func enrichSourceRoot(root *SourceRoot) error {
	info, err := os.Stat(root.Path)
	if err != nil {
		if os.IsNotExist(err) {
			root.Exists = false
			return nil
		}
		return err
	}
	if !info.IsDir() {
		root.Exists = false
		return nil
	}

	root.Exists = true
	var samples []string
	err = filepath.WalkDir(root.Path, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".codex", ".worktrees", "node_modules", "dist", "build":
				if path != root.Path {
					return filepath.SkipDir
				}
			}
			return nil
		}
		root.FileCount++
		if len(samples) < 12 && sampleFile(path) {
			rel, err := filepath.Rel(root.Path, path)
			if err != nil {
				return err
			}
			samples = append(samples, rel)
		}
		return nil
	})
	if err != nil {
		return err
	}
	sort.Strings(samples)
	root.Samples = samples
	return nil
}

func sampleFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".css", ".go", ".html", ".js", ".json", ".md", ".py", ".tmpl", ".toml", ".ts", ".yaml", ".yml":
		return true
	default:
		return false
	}
}

func collectImplementationInventory(cfg Config) (ImplementationInventory, error) {
	landingSite := SourceRoot{Name: "www.gormes.ai", Path: filepath.Join(cfg.RepoRoot, "www.gormes.ai")}
	if err := enrichSourceRoot(&landingSite); err != nil {
		return ImplementationInventory{}, err
	}

	hugoDocs := SourceRoot{Name: "Hugo docs", Path: filepath.Join(cfg.RepoRoot, "docs")}
	if err := enrichSourceRoot(&hugoDocs); err != nil {
		return ImplementationInventory{}, err
	}

	return ImplementationInventory{
		Commands:         collectImmediateDirs(filepath.Join(cfg.RepoRoot, "cmd")),
		InternalPackages: collectImmediateDirs(filepath.Join(cfg.RepoRoot, "internal")),
		BuildingDocs:     collectImmediateFiles(filepath.Join(cfg.RepoRoot, "docs", "content", "building-gormes"), ".md"),
		LandingSite:      landingSite,
		HugoDocs:         hugoDocs,
	}, nil
}

func collectImmediateDirs(root string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var dirs []string
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, entry.Name())
		}
	}
	sort.Strings(dirs)
	return dirs
}

func collectImmediateFiles(root, ext string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ext {
			files = append(files, entry.Name())
		}
	}
	sort.Strings(files)
	return files
}
