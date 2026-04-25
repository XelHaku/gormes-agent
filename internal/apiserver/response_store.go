package apiserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

const (
	defaultMaxStoredResponses = 100
	responseStoreBucketName   = "api_responses_v1"
	conversationBucketName    = "api_response_conversations_v1"
)

// StoredResponse is the read-model payload used for GET /v1/responses and
// previous_response_id reconstruction.
type StoredResponse struct {
	Response            ResponseObject `json:"response"`
	ConversationHistory []ChatMessage  `json:"conversation_history"`
	Instructions        string         `json:"instructions,omitempty"`
	SessionID           string         `json:"session_id,omitempty"`
}

// ResponseObject is the OpenAI Responses-compatible response envelope.
type ResponseObject struct {
	ID        string               `json:"id"`
	Object    string               `json:"object"`
	Status    string               `json:"status"`
	CreatedAt int64                `json:"created_at"`
	Model     string               `json:"model"`
	Output    []ResponseOutputItem `json:"output"`
	Usage     ResponseUsage        `json:"usage"`
}

type ResponseOutputItem struct {
	Type      string                `json:"type"`
	ID        string                `json:"id,omitempty"`
	Role      string                `json:"role,omitempty"`
	Content   []ResponseContentPart `json:"content,omitempty"`
	CallID    string                `json:"call_id,omitempty"`
	Name      string                `json:"name,omitempty"`
	Arguments string                `json:"arguments,omitempty"`
	Output    string                `json:"output,omitempty"`
}

type ResponseContentPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type ResponseUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type responseStoreRecord struct {
	Data       StoredResponse `json:"data"`
	AccessedAt int64          `json:"accessed_at"`
}

type responseStoreStats struct {
	Enabled      bool
	Size         int
	MaxSize      int
	LRUEvictions int
}

// ResponseStore is a bounded LRU store. It is in-memory by default for tests
// and can be backed by bbolt for persistence across gateway restarts.
type ResponseStore struct {
	mu            sync.Mutex
	maxSize       int
	now           func() time.Time
	db            *bolt.DB
	closeDB       bool
	mem           map[string]responseStoreRecord
	conversations map[string]string
	lruEvictions  int
}

func NewResponseStore(maxSize int) *ResponseStore {
	if maxSize <= 0 {
		maxSize = defaultMaxStoredResponses
	}
	return &ResponseStore{
		maxSize:       maxSize,
		now:           time.Now,
		mem:           make(map[string]responseStoreRecord),
		conversations: make(map[string]string),
	}
}

func OpenResponseStore(path string, maxSize int) (*ResponseStore, error) {
	if path == "" {
		return nil, errors.New("api response store: path is required")
	}
	if maxSize <= 0 {
		maxSize = defaultMaxStoredResponses
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("api response store: create parent dir for %s: %w", path, err)
	}
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: 100 * time.Millisecond})
	if err != nil {
		return nil, fmt.Errorf("api response store: open %s: %w", path, err)
	}
	store := &ResponseStore{maxSize: maxSize, now: time.Now, db: db, closeDB: true}
	if err := store.ensureBuckets(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func NewBoltResponseStore(db *bolt.DB, maxSize int) (*ResponseStore, error) {
	if db == nil {
		return nil, errors.New("api response store: nil bolt DB")
	}
	if maxSize <= 0 {
		maxSize = defaultMaxStoredResponses
	}
	store := &ResponseStore{maxSize: maxSize, now: time.Now, db: db}
	if err := store.ensureBuckets(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *ResponseStore) ensureBuckets() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists([]byte(responseStoreBucketName)); err != nil {
			return fmt.Errorf("api response store: create response bucket: %w", err)
		}
		if _, err := tx.CreateBucketIfNotExists([]byte(conversationBucketName)); err != nil {
			return fmt.Errorf("api response store: create conversation bucket: %w", err)
		}
		return nil
	})
}

func (s *ResponseStore) Get(responseID string) (StoredResponse, bool, error) {
	if s == nil {
		return StoredResponse{}, false, nil
	}
	if s.db != nil {
		return s.getBolt(responseID)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.mem[responseID]
	if !ok {
		return StoredResponse{}, false, nil
	}
	rec.AccessedAt = s.now().UnixNano()
	s.mem[responseID] = rec
	return rec.Data, true, nil
}

func (s *ResponseStore) getBolt(responseID string) (StoredResponse, bool, error) {
	var (
		out StoredResponse
		ok  bool
	)
	err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(responseStoreBucketName))
		if b == nil {
			return errors.New("api response store: response bucket missing")
		}
		raw := b.Get([]byte(responseID))
		if raw == nil {
			return nil
		}
		var rec responseStoreRecord
		if err := json.Unmarshal(raw, &rec); err != nil {
			return fmt.Errorf("api response store: decode %s: %w", responseID, err)
		}
		rec.AccessedAt = s.now().UnixNano()
		encoded, err := json.Marshal(rec)
		if err != nil {
			return fmt.Errorf("api response store: encode %s: %w", responseID, err)
		}
		if err := b.Put([]byte(responseID), encoded); err != nil {
			return err
		}
		out = rec.Data
		ok = true
		return nil
	})
	return out, ok, err
}

func (s *ResponseStore) Put(responseID string, data StoredResponse) error {
	if s == nil {
		return errors.New("api response store: store disabled")
	}
	if s.db != nil {
		return s.putBolt(responseID, data)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mem[responseID] = responseStoreRecord{Data: data, AccessedAt: s.now().UnixNano()}
	s.evictMemoryLocked()
	return nil
}

func (s *ResponseStore) putBolt(responseID string, data StoredResponse) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(responseStoreBucketName))
		cb := tx.Bucket([]byte(conversationBucketName))
		if b == nil || cb == nil {
			return errors.New("api response store: buckets missing")
		}
		rec := responseStoreRecord{Data: data, AccessedAt: s.now().UnixNano()}
		encoded, err := json.Marshal(rec)
		if err != nil {
			return fmt.Errorf("api response store: encode %s: %w", responseID, err)
		}
		if err := b.Put([]byte(responseID), encoded); err != nil {
			return err
		}
		return s.evictBoltLocked(b, cb)
	})
}

func (s *ResponseStore) Delete(responseID string) (bool, error) {
	if s == nil {
		return false, nil
	}
	if s.db != nil {
		return s.deleteBolt(responseID)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.mem[responseID]
	delete(s.mem, responseID)
	if ok {
		s.removeConversationPointersLocked(responseID)
	}
	return ok, nil
}

func (s *ResponseStore) deleteBolt(responseID string) (bool, error) {
	var deleted bool
	err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(responseStoreBucketName))
		cb := tx.Bucket([]byte(conversationBucketName))
		if b == nil || cb == nil {
			return errors.New("api response store: buckets missing")
		}
		if b.Get([]byte(responseID)) != nil {
			deleted = true
			if err := b.Delete([]byte(responseID)); err != nil {
				return err
			}
			return deleteConversationPointers(cb, responseID)
		}
		return nil
	})
	return deleted, err
}

func (s *ResponseStore) ListSessions(limit, offset int, now time.Time) ([]DashboardSessionInfo, int, error) {
	if s == nil {
		return nil, 0, nil
	}
	if limit <= 0 {
		limit = dashboardDefaultSessionLimit
	}
	if offset < 0 {
		offset = 0
	}
	if now.IsZero() {
		now = time.Now()
	}
	records, err := s.allStoredResponses()
	if err != nil {
		return nil, 0, err
	}
	sessions := dashboardSessionsFromResponses(records, now)
	total := len(sessions)
	if offset >= total {
		return []DashboardSessionInfo{}, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return append([]DashboardSessionInfo(nil), sessions[offset:end]...), total, nil
}

func (s *ResponseStore) allStoredResponses() ([]StoredResponse, error) {
	if s == nil {
		return nil, nil
	}
	if s.db != nil {
		var out []StoredResponse
		err := s.db.View(func(tx *bolt.Tx) error {
			b := tx.Bucket([]byte(responseStoreBucketName))
			if b == nil {
				return errors.New("api response store: response bucket missing")
			}
			return b.ForEach(func(_, v []byte) error {
				var rec responseStoreRecord
				if err := json.Unmarshal(v, &rec); err != nil {
					return fmt.Errorf("api response store: decode response for sessions: %w", err)
				}
				out = append(out, rec.Data)
				return nil
			})
		})
		return out, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]StoredResponse, 0, len(s.mem))
	for _, rec := range s.mem {
		out = append(out, rec.Data)
	}
	return out, nil
}

func (s *ResponseStore) DeleteSession(sessionID string) (bool, error) {
	if s == nil {
		return false, nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false, nil
	}
	if s.db != nil {
		return s.deleteSessionBolt(sessionID)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	deleted := false
	for responseID, rec := range s.mem {
		if rec.Data.SessionID != sessionID {
			continue
		}
		delete(s.mem, responseID)
		s.removeConversationPointersLocked(responseID)
		deleted = true
	}
	return deleted, nil
}

func (s *ResponseStore) deleteSessionBolt(sessionID string) (bool, error) {
	var deleted bool
	err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(responseStoreBucketName))
		cb := tx.Bucket([]byte(conversationBucketName))
		if b == nil || cb == nil {
			return errors.New("api response store: buckets missing")
		}
		var responseIDs [][]byte
		if err := b.ForEach(func(k, v []byte) error {
			var rec responseStoreRecord
			if err := json.Unmarshal(v, &rec); err != nil {
				return fmt.Errorf("api response store: decode response during session delete: %w", err)
			}
			if rec.Data.SessionID == sessionID {
				responseIDs = append(responseIDs, append([]byte(nil), k...))
			}
			return nil
		}); err != nil {
			return err
		}
		for _, responseID := range responseIDs {
			if err := b.Delete(responseID); err != nil {
				return err
			}
			if err := deleteConversationPointers(cb, string(responseID)); err != nil {
				return err
			}
		}
		deleted = len(responseIDs) > 0
		return nil
	})
	return deleted, err
}

func (s *ResponseStore) GetConversation(name string) (string, bool, error) {
	if s == nil {
		return "", false, nil
	}
	if s.db != nil {
		var out string
		err := s.db.View(func(tx *bolt.Tx) error {
			b := tx.Bucket([]byte(conversationBucketName))
			if b == nil {
				return errors.New("api response store: conversation bucket missing")
			}
			if raw := b.Get([]byte(name)); raw != nil {
				out = string(raw)
			}
			return nil
		})
		return out, out != "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.conversations[name]
	return out, out != "", nil
}

func (s *ResponseStore) SetConversation(name, responseID string) error {
	if s == nil {
		return errors.New("api response store: store disabled")
	}
	if s.db != nil {
		return s.db.Update(func(tx *bolt.Tx) error {
			b := tx.Bucket([]byte(conversationBucketName))
			if b == nil {
				return errors.New("api response store: conversation bucket missing")
			}
			return b.Put([]byte(name), []byte(responseID))
		})
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.conversations[name] = responseID
	return nil
}

func (s *ResponseStore) Len() (int, error) {
	if s == nil {
		return 0, nil
	}
	if s.db != nil {
		n := 0
		err := s.db.View(func(tx *bolt.Tx) error {
			b := tx.Bucket([]byte(responseStoreBucketName))
			if b == nil {
				return errors.New("api response store: response bucket missing")
			}
			return b.ForEach(func(_, _ []byte) error {
				n++
				return nil
			})
		})
		return n, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.mem), nil
}

func (s *ResponseStore) Stats() responseStoreStats {
	if s == nil {
		return responseStoreStats{}
	}
	n, _ := s.Len()
	s.mu.Lock()
	evictions := s.lruEvictions
	s.mu.Unlock()
	return responseStoreStats{Enabled: true, Size: n, MaxSize: s.maxSize, LRUEvictions: evictions}
}

func (s *ResponseStore) Close() error {
	if s == nil || s.db == nil || !s.closeDB {
		return nil
	}
	err := s.db.Close()
	s.db = nil
	return err
}

func (s *ResponseStore) evictMemoryLocked() {
	over := len(s.mem) - s.maxSize
	if over <= 0 {
		return
	}
	type candidate struct {
		id         string
		accessedAt int64
	}
	items := make([]candidate, 0, len(s.mem))
	for id, rec := range s.mem {
		items = append(items, candidate{id: id, accessedAt: rec.AccessedAt})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].accessedAt != items[j].accessedAt {
			return items[i].accessedAt < items[j].accessedAt
		}
		return items[i].id < items[j].id
	})
	for _, item := range items[:over] {
		delete(s.mem, item.id)
		s.removeConversationPointersLocked(item.id)
		s.lruEvictions++
	}
}

func (s *ResponseStore) evictBoltLocked(b, cb *bolt.Bucket) error {
	count := 0
	type candidate struct {
		id         string
		accessedAt int64
	}
	var items []candidate
	if err := b.ForEach(func(k, v []byte) error {
		count++
		var rec responseStoreRecord
		if err := json.Unmarshal(v, &rec); err != nil {
			return fmt.Errorf("api response store: decode during eviction: %w", err)
		}
		items = append(items, candidate{id: string(k), accessedAt: rec.AccessedAt})
		return nil
	}); err != nil {
		return err
	}
	over := count - s.maxSize
	if over <= 0 {
		return nil
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].accessedAt != items[j].accessedAt {
			return items[i].accessedAt < items[j].accessedAt
		}
		return items[i].id < items[j].id
	})
	for _, item := range items[:over] {
		if err := b.Delete([]byte(item.id)); err != nil {
			return err
		}
		if err := deleteConversationPointers(cb, item.id); err != nil {
			return err
		}
		s.mu.Lock()
		s.lruEvictions++
		s.mu.Unlock()
	}
	return nil
}

func (s *ResponseStore) removeConversationPointersLocked(responseID string) {
	for name, id := range s.conversations {
		if id == responseID {
			delete(s.conversations, name)
		}
	}
}

func deleteConversationPointers(b *bolt.Bucket, responseID string) error {
	var names [][]byte
	if err := b.ForEach(func(k, v []byte) error {
		if string(v) == responseID {
			names = append(names, append([]byte(nil), k...))
		}
		return nil
	}); err != nil {
		return err
	}
	for _, name := range names {
		if err := b.Delete(name); err != nil {
			return err
		}
	}
	return nil
}

func dashboardSessionsFromResponses(records []StoredResponse, now time.Time) []DashboardSessionInfo {
	type accumulator struct {
		info DashboardSessionInfo
	}
	bySession := make(map[string]*accumulator)
	source := "api"
	for _, rec := range records {
		sessionID := strings.TrimSpace(rec.SessionID)
		if sessionID == "" {
			continue
		}
		acc := bySession[sessionID]
		if acc == nil {
			acc = &accumulator{
				info: DashboardSessionInfo{
					ID:        sessionID,
					Source:    stringPtr(source),
					StartedAt: rec.Response.CreatedAt,
				},
			}
			bySession[sessionID] = acc
		}
		if rec.Response.CreatedAt > 0 && (acc.info.StartedAt == 0 || rec.Response.CreatedAt < acc.info.StartedAt) {
			acc.info.StartedAt = rec.Response.CreatedAt
		}
		if rec.Response.CreatedAt > acc.info.LastActive {
			acc.info.LastActive = rec.Response.CreatedAt
		}
		if rec.Response.Model != "" {
			acc.info.Model = stringPtr(rec.Response.Model)
		}
		acc.info.InputTokens += rec.Response.Usage.InputTokens
		acc.info.OutputTokens += rec.Response.Usage.OutputTokens
		acc.info.MessageCount += len(rec.ConversationHistory)
		acc.info.ToolCallCount += countDashboardToolCalls(rec)
		if acc.info.Title == nil {
			if title := firstDashboardUserText(rec.ConversationHistory); title != "" {
				acc.info.Title = stringPtr(dashboardSnippet(title, 80))
			}
		}
		if preview := dashboardResponsePreview(rec.Response); preview != "" {
			acc.info.Preview = stringPtr(dashboardSnippet(preview, 160))
		}
	}
	out := make([]DashboardSessionInfo, 0, len(bySession))
	for _, acc := range bySession {
		if acc.info.MessageCount == 0 && acc.info.Preview != nil {
			acc.info.MessageCount = 1
		}
		acc.info.IsActive = acc.info.LastActive > 0 && now.Unix()-acc.info.LastActive < 300
		out = append(out, acc.info)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].LastActive != out[j].LastActive {
			return out[i].LastActive > out[j].LastActive
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func countDashboardToolCalls(rec StoredResponse) int {
	total := 0
	for _, msg := range rec.ConversationHistory {
		total += len(msg.ToolCalls)
		if msg.Role == "tool" || msg.ToolCallID != "" {
			total++
		}
	}
	for _, item := range rec.Response.Output {
		if item.Type == "function_call" || item.Type == "function_call_output" {
			total++
		}
	}
	return total
}

func firstDashboardUserText(messages []ChatMessage) string {
	for _, msg := range messages {
		if msg.Role == "user" && strings.TrimSpace(msg.Content) != "" {
			return strings.TrimSpace(msg.Content)
		}
	}
	return ""
}

func dashboardResponsePreview(response ResponseObject) string {
	for i := len(response.Output) - 1; i >= 0; i-- {
		item := response.Output[i]
		for j := len(item.Content) - 1; j >= 0; j-- {
			if text := strings.TrimSpace(item.Content[j].Text); text != "" {
				return text
			}
		}
		if text := strings.TrimSpace(item.Output); text != "" {
			return text
		}
	}
	return ""
}

func dashboardSnippet(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if max <= 0 || len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}
