package tools

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	FileReadDedupStatusUnchanged     = "unchanged"
	FileReadStatusDedupCacheDisabled = "read_dedup_cache_disabled"
	FileReadStatusGuardUnavailable   = "guard_unavailable"
)

const fileReadDedupStatusMessage = "File unchanged since last read. The earlier read_file result in this conversation is still current; use that content instead of writing this status text."

// ErrFileReadGuardStatusContent reports an attempt to persist internal status text.
var ErrFileReadGuardStatusContent = errors.New("file read guard: internal read_file status text cannot be file content")

// FileReadGuardOptions configures the read dedup cache scope.
type FileReadGuardOptions struct {
	WorkspaceRoot string
}

// FileReadGuard keeps read-cache evidence separate from returned file bytes.
type FileReadGuard struct {
	mu            sync.Mutex
	workspaceRoot string
	cache         map[fileReadCacheKey][]byte
}

// FileReadResult is the read model returned by the file-read helper.
type FileReadResult struct {
	Path        string
	Content     []byte
	DedupStatus string
	Evidence    []FileReadEvidence
}

// FileReadEvidence reports non-content file-read status.
type FileReadEvidence struct {
	Kind    string
	Path    string
	Message string
}

type fileReadCacheKey struct {
	workspaceRoot string
	path          string
}

// NewFileReadGuard returns a workspace-scoped read cache.
func NewFileReadGuard(opts FileReadGuardOptions) *FileReadGuard {
	root := opts.WorkspaceRoot
	if root != "" {
		if abs, err := filepath.Abs(root); err == nil {
			root = abs
		}
		root = filepath.Clean(root)
	}
	return &FileReadGuard{
		workspaceRoot: root,
		cache:         make(map[fileReadCacheKey][]byte),
	}
}

// ReadFile reads path while tracking duplicate-read evidence separately.
func (g *FileReadGuard) ReadFile(path string) (FileReadResult, error) {
	return g.ReadFileWith(path, os.ReadFile)
}

// ReadFileWith is the test seam for the underlying file read operation.
func (g *FileReadGuard) ReadFileWith(path string, read func(string) ([]byte, error)) (FileReadResult, error) {
	if read == nil {
		read = os.ReadFile
	}
	if g == nil {
		return readFileWithoutCache(path, path, read, FileReadStatusGuardUnavailable)
	}
	key, err := g.cacheKey(path)
	if err != nil {
		return FileReadResult{}, err
	}
	if g.cache == nil {
		return readFileWithoutCache(path, key.path, read, FileReadStatusDedupCacheDisabled)
	}

	g.mu.Lock()
	if cached, ok := g.cache[key]; ok {
		g.mu.Unlock()
		return FileReadResult{
			Path:        path,
			Content:     append([]byte(nil), cached...),
			DedupStatus: FileReadDedupStatusUnchanged,
			Evidence: []FileReadEvidence{{
				Kind:    FileReadDedupStatusUnchanged,
				Path:    path,
				Message: fileReadDedupStatusMessage,
			}},
		}, nil
	}
	g.mu.Unlock()

	content, err := read(key.path)
	if err != nil {
		return FileReadResult{}, err
	}

	g.mu.Lock()
	g.cache[key] = append([]byte(nil), content...)
	g.mu.Unlock()

	return FileReadResult{
		Path:    path,
		Content: append([]byte(nil), content...),
	}, nil
}

func readFileWithoutCache(path, resolved string, read func(string) ([]byte, error), status string) (FileReadResult, error) {
	content, err := read(resolved)
	if err != nil {
		return FileReadResult{}, err
	}
	return FileReadResult{
		Path:        path,
		Content:     append([]byte(nil), content...),
		DedupStatus: status,
		Evidence: []FileReadEvidence{{
			Kind: status,
			Path: path,
		}},
	}, nil
}

// WriteFile runs write and invalidates cached reads for the written path.
func (g *FileReadGuard) WriteFile(path string, content []byte, write func(string, []byte) error) error {
	if isFileReadGuardStatusText(content) {
		return ErrFileReadGuardStatusContent
	}
	key, err := g.cacheKey(path)
	if err != nil {
		return err
	}
	if err := write(key.path, append([]byte(nil), content...)); err != nil {
		return err
	}
	g.invalidatePath(key)
	return nil
}

// PatchFile runs apply and invalidates cached reads for the patched path.
func (g *FileReadGuard) PatchFile(path string, apply func(string) error) error {
	key, err := g.cacheKey(path)
	if err != nil {
		return err
	}
	if err := apply(key.path); err != nil {
		return err
	}
	g.invalidatePath(key)
	return nil
}

func isFileReadGuardStatusText(content []byte) bool {
	stripped := strings.TrimSpace(string(content))
	if stripped == "" {
		return false
	}
	if stripped == fileReadDedupStatusMessage {
		return true
	}
	return strings.Contains(stripped, fileReadDedupStatusMessage) &&
		len(stripped) <= 2*len(fileReadDedupStatusMessage)
}

func (g *FileReadGuard) invalidatePath(key fileReadCacheKey) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for cachedKey := range g.cache {
		if cachedKey.workspaceRoot == key.workspaceRoot && cachedKey.path == key.path {
			delete(g.cache, cachedKey)
		}
	}
}

func (g *FileReadGuard) cacheKey(path string) (fileReadCacheKey, error) {
	resolved := path
	if !filepath.IsAbs(resolved) && g.workspaceRoot != "" {
		resolved = filepath.Join(g.workspaceRoot, resolved)
	}
	abs, err := filepath.Abs(resolved)
	if err != nil {
		return fileReadCacheKey{}, err
	}
	return fileReadCacheKey{
		workspaceRoot: g.workspaceRoot,
		path:          filepath.Clean(abs),
	}, nil
}
