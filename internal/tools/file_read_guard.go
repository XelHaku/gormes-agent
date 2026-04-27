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
	FileReadStatusDedupStubBlocked   = "read_dedup_stub_blocked"
)

const fileReadDedupStatusMessage = "File unchanged since last read. The earlier read_file result in this conversation is still current; use that content instead of writing this status text."
const fileReadDedupStubBlockedMessage = "BLOCKED: repeated read_file status stub detected. The earlier read_file content is still current; stop treating this status text as file content."

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
	stubHits      map[fileReadCacheKey]int
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
		stubHits:      make(map[fileReadCacheKey]int),
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

// GuardRepeatedReadStatus keeps model-visible read_file status stubs out of
// content and escalates repeated stubs to blocked evidence.
func (g *FileReadGuard) GuardRepeatedReadStatus(result FileReadResult) FileReadResult {
	guarded := cloneFileReadResult(result)
	kind, message, ok := fileReadStatusStub(guarded)
	if !ok {
		g.resetReadStatusStub(guarded.Path)
		return guarded
	}

	if message == "" && kind == FileReadDedupStatusUnchanged {
		message = fileReadDedupStatusMessage
	}

	hits := 1
	if g != nil {
		key := g.readStatusStubKey(guarded.Path)
		g.mu.Lock()
		if g.stubHits == nil {
			g.stubHits = make(map[fileReadCacheKey]int)
		}
		hits = g.stubHits[key] + 1
		g.stubHits[key] = hits
		g.mu.Unlock()
	}

	if hits >= 2 {
		return FileReadResult{
			Path:        guarded.Path,
			DedupStatus: FileReadStatusDedupStubBlocked,
			Evidence: []FileReadEvidence{{
				Kind:    FileReadStatusDedupStubBlocked,
				Path:    guarded.Path,
				Message: fileReadDedupStubBlockedMessage,
			}},
		}
	}

	return FileReadResult{
		Path:        guarded.Path,
		DedupStatus: kind,
		Evidence: []FileReadEvidence{{
			Kind:    kind,
			Path:    guarded.Path,
			Message: message,
		}},
	}
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

func cloneFileReadResult(result FileReadResult) FileReadResult {
	result.Content = append([]byte(nil), result.Content...)
	result.Evidence = append([]FileReadEvidence(nil), result.Evidence...)
	return result
}

func fileReadStatusStub(result FileReadResult) (kind, message string, ok bool) {
	if isFileReadGuardStatusText(result.Content) {
		return FileReadDedupStatusUnchanged, strings.TrimSpace(string(result.Content)), true
	}
	if len(result.Content) != 0 {
		return "", "", false
	}
	if isFileReadStatusKind(result.DedupStatus) {
		return result.DedupStatus, fileReadEvidenceMessage(result, result.DedupStatus), true
	}
	for _, evidence := range result.Evidence {
		if isFileReadStatusKind(evidence.Kind) {
			return evidence.Kind, evidence.Message, true
		}
	}
	return "", "", false
}

func fileReadEvidenceMessage(result FileReadResult, kind string) string {
	for _, evidence := range result.Evidence {
		if evidence.Kind == kind {
			return evidence.Message
		}
	}
	return ""
}

func isFileReadStatusKind(kind string) bool {
	switch kind {
	case FileReadDedupStatusUnchanged, FileReadStatusDedupCacheDisabled, FileReadStatusGuardUnavailable:
		return true
	default:
		return false
	}
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

func (g *FileReadGuard) resetReadStatusStub(path string) {
	if g == nil {
		return
	}
	key := g.readStatusStubKey(path)
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.stubHits, key)
}

func (g *FileReadGuard) readStatusStubKey(path string) fileReadCacheKey {
	if g == nil {
		return fileReadCacheKey{path: filepath.Clean(path)}
	}
	key, err := g.cacheKey(path)
	if err == nil {
		return key
	}
	return fileReadCacheKey{
		workspaceRoot: g.workspaceRoot,
		path:          filepath.Clean(path),
	}
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
