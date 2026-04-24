package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Todo status vocabulary. Matches the Python upstream (tools/todo_tool.py).
const (
	TodoStatusPending    = "pending"
	TodoStatusInProgress = "in_progress"
	TodoStatusCompleted  = "completed"
	TodoStatusCancelled  = "cancelled"
)

// ErrTodoStoreMissing is returned by TodoTool.Execute when no per-session
// TodoStore has been bound to the tool.
var ErrTodoStoreMissing = errors.New("todo: store not initialized")

var validTodoStatuses = map[string]struct{}{
	TodoStatusPending:    {},
	TodoStatusInProgress: {},
	TodoStatusCompleted:  {},
	TodoStatusCancelled:  {},
}

// TodoItem is the canonical stored form: all fields are normalized.
type TodoItem struct {
	ID      string `json:"id"`
	Content string `json:"content"`
	Status  string `json:"status"`
}

// TodoItemInput is the write-side payload. Empty fields trigger normalization
// (empty status becomes pending, empty id becomes "?", empty content becomes
// "(no description)"). In merge mode, empty fields are treated as "unchanged".
type TodoItemInput struct {
	ID      string `json:"id"`
	Content string `json:"content"`
	Status  string `json:"status"`
}

// TodoStore is the per-session task list. Ordered: list position is priority.
type TodoStore struct {
	mu    sync.Mutex
	items []TodoItem
}

// NewTodoStore returns an empty store.
func NewTodoStore() *TodoStore { return &TodoStore{} }

// Write replaces or merges the list. Returns a defensive copy of the result.
//
// When merge is false the entire list is replaced, preserving the last
// occurrence of duplicate ids in their latest position. When merge is true,
// existing items are updated by id (only non-empty fields are applied) and
// new items are appended in submission order.
func (s *TodoStore) Write(items []TodoItemInput, merge bool) []TodoItem {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !merge {
		deduped := dedupeByID(items)
		next := make([]TodoItem, 0, len(deduped))
		for _, raw := range deduped {
			next = append(next, normalizeTodo(raw))
		}
		s.items = next
		return cloneTodoItems(s.items)
	}

	index := make(map[string]int, len(s.items))
	for i, item := range s.items {
		index[item.ID] = i
	}
	for _, raw := range dedupeByID(items) {
		id := strings.TrimSpace(raw.ID)
		if id == "" {
			continue
		}
		if pos, ok := index[id]; ok {
			if content := strings.TrimSpace(raw.Content); content != "" {
				s.items[pos].Content = content
			}
			if status := strings.ToLower(strings.TrimSpace(raw.Status)); status != "" {
				if _, ok := validTodoStatuses[status]; ok {
					s.items[pos].Status = status
				}
			}
			continue
		}
		next := normalizeTodo(raw)
		index[next.ID] = len(s.items)
		s.items = append(s.items, next)
	}
	return cloneTodoItems(s.items)
}

// Read returns a defensive copy of the current list.
func (s *TodoStore) Read() []TodoItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneTodoItems(s.items)
}

// HasItems reports whether any todos are currently tracked.
func (s *TodoStore) HasItems() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.items) > 0
}

// FormatForInjection renders the active (pending / in_progress) items for
// post-compression context restoration. Returns an empty string when there
// are no active items.
func (s *TodoStore) FormatForInjection() string {
	s.mu.Lock()
	active := make([]TodoItem, 0, len(s.items))
	for _, item := range s.items {
		if item.Status == TodoStatusPending || item.Status == TodoStatusInProgress {
			active = append(active, item)
		}
	}
	s.mu.Unlock()
	if len(active) == 0 {
		return ""
	}
	lines := []string{"[Your active task list was preserved across context compression]"}
	for _, item := range active {
		marker := "[?]"
		switch item.Status {
		case TodoStatusPending:
			marker = "[ ]"
		case TodoStatusInProgress:
			marker = "[>]"
		}
		lines = append(lines, fmt.Sprintf("- %s %s. %s (%s)", marker, item.ID, item.Content, item.Status))
	}
	return strings.Join(lines, "\n")
}

// TodoTool is the Go-native tool surface over a TodoStore. A caller (kernel
// or gateway) binds Store to the per-session instance before dispatching.
type TodoTool struct {
	Store *TodoStore
}

var _ Tool = (*TodoTool)(nil)

func (*TodoTool) Name() string { return "todo" }

func (*TodoTool) Description() string {
	return "Manage your task list for the current session. Use for complex tasks with 3+ steps or when the user provides multiple tasks. Call with no parameters to read the current list. Provide 'todos' to create or update items; merge=false (default) replaces the entire list, merge=true updates existing items by id and appends new ones. Always returns the full current list with per-status counts."
}

func (*TodoTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{
			"todos":{
				"type":"array",
				"description":"Task items to write. Omit to read the current list.",
				"items":{
					"type":"object",
					"properties":{
						"id":{"type":"string","description":"Unique item identifier"},
						"content":{"type":"string","description":"Task description"},
						"status":{
							"type":"string",
							"enum":["pending","in_progress","completed","cancelled"],
							"description":"Current status"
						}
					},
					"required":["id","content","status"]
				}
			},
			"merge":{
				"type":"boolean",
				"description":"true: update existing items by id, add new ones. false (default): replace the entire list.",
				"default":false
			}
		},
		"required":[]
	}`)
}

func (*TodoTool) Timeout() time.Duration { return 0 }

// todoExecuteArgs is the wire-level decoder. Todos is a pointer-to-slice so
// the caller can distinguish "omitted" (read mode) from "empty list" (clear).
type todoExecuteArgs struct {
	Todos *[]TodoItemInput `json:"todos"`
	Merge bool             `json:"merge"`
}

func (t *TodoTool) Execute(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
	if t.Store == nil {
		return nil, ErrTodoStoreMissing
	}
	var in todoExecuteArgs
	if len(args) > 0 {
		if err := json.Unmarshal(args, &in); err != nil {
			return nil, fmt.Errorf("todo: invalid args: %w", err)
		}
	}
	var items []TodoItem
	if in.Todos != nil {
		items = t.Store.Write(*in.Todos, in.Merge)
	} else {
		items = t.Store.Read()
	}
	return json.Marshal(struct {
		Todos   []TodoItem  `json:"todos"`
		Summary todoSummary `json:"summary"`
	}{
		Todos:   items,
		Summary: summarizeTodos(items),
	})
}

type todoSummary struct {
	Total      int `json:"total"`
	Pending    int `json:"pending"`
	InProgress int `json:"in_progress"`
	Completed  int `json:"completed"`
	Cancelled  int `json:"cancelled"`
}

func summarizeTodos(items []TodoItem) todoSummary {
	s := todoSummary{Total: len(items)}
	for _, item := range items {
		switch item.Status {
		case TodoStatusPending:
			s.Pending++
		case TodoStatusInProgress:
			s.InProgress++
		case TodoStatusCompleted:
			s.Completed++
		case TodoStatusCancelled:
			s.Cancelled++
		}
	}
	return s
}

func normalizeTodo(raw TodoItemInput) TodoItem {
	id := strings.TrimSpace(raw.ID)
	if id == "" {
		id = "?"
	}
	content := strings.TrimSpace(raw.Content)
	if content == "" {
		content = "(no description)"
	}
	status := strings.ToLower(strings.TrimSpace(raw.Status))
	if _, ok := validTodoStatuses[status]; !ok {
		status = TodoStatusPending
	}
	return TodoItem{ID: id, Content: content, Status: status}
}

// dedupeByID collapses duplicate ids, keeping the last occurrence in its
// position. Empty ids are treated as the sentinel "?" so multiple empties
// collapse together, matching the Python upstream behavior.
func dedupeByID(items []TodoItemInput) []TodoItemInput {
	lastIndex := make(map[string]int, len(items))
	for i, item := range items {
		key := strings.TrimSpace(item.ID)
		if key == "" {
			key = "?"
		}
		lastIndex[key] = i
	}
	positions := make([]int, 0, len(lastIndex))
	for _, idx := range lastIndex {
		positions = append(positions, idx)
	}
	sort.Ints(positions)
	out := make([]TodoItemInput, 0, len(positions))
	for _, idx := range positions {
		out = append(out, items[idx])
	}
	return out
}

func cloneTodoItems(items []TodoItem) []TodoItem {
	out := make([]TodoItem, len(items))
	copy(out, items)
	return out
}
