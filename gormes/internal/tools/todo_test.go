package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestTodoStore_WriteReplacesList(t *testing.T) {
	store := NewTodoStore()
	items := store.Write([]TodoItemInput{
		{ID: "1", Content: "First task", Status: "pending"},
		{ID: "2", Content: "Second task", Status: "in_progress"},
	}, false)

	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	if items[0].ID != "1" || items[1].Status != "in_progress" {
		t.Fatalf("unexpected items = %+v", items)
	}
}

func TestTodoStore_ReadReturnsDefensiveCopy(t *testing.T) {
	store := NewTodoStore()
	store.Write([]TodoItemInput{{ID: "1", Content: "Task", Status: "pending"}}, false)

	items := store.Read()
	items[0].Content = "MUTATED"

	again := store.Read()
	if again[0].Content != "Task" {
		t.Fatalf("content = %q, want original Task", again[0].Content)
	}
}

func TestTodoStore_WriteDeduplicatesDuplicateIDs(t *testing.T) {
	store := NewTodoStore()
	got := store.Write([]TodoItemInput{
		{ID: "1", Content: "First version", Status: "pending"},
		{ID: "2", Content: "Other task", Status: "pending"},
		{ID: "1", Content: "Latest version", Status: "in_progress"},
	}, false)

	want := []TodoItem{
		{ID: "2", Content: "Other task", Status: "pending"},
		{ID: "1", Content: "Latest version", Status: "in_progress"},
	}
	if len(got) != len(want) {
		t.Fatalf("len(got) = %d, want %d; got=%+v", len(got), len(want), got)
	}
	for i, item := range want {
		if got[i] != item {
			t.Fatalf("item %d = %+v, want %+v", i, got[i], item)
		}
	}
}

func TestTodoStore_WriteNormalizesInvalidStatus(t *testing.T) {
	store := NewTodoStore()
	items := store.Write([]TodoItemInput{
		{ID: "", Content: "", Status: "garbage"},
	}, false)
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].Status != "pending" {
		t.Fatalf("status = %q, want pending fallback", items[0].Status)
	}
	if items[0].ID != "?" {
		t.Fatalf("id = %q, want ? fallback", items[0].ID)
	}
	if items[0].Content != "(no description)" {
		t.Fatalf("content = %q, want default fallback", items[0].Content)
	}
}

func TestTodoStore_HasItems(t *testing.T) {
	store := NewTodoStore()
	if store.HasItems() {
		t.Fatal("empty HasItems = true, want false")
	}
	store.Write([]TodoItemInput{{ID: "1", Content: "x", Status: "pending"}}, false)
	if !store.HasItems() {
		t.Fatal("non-empty HasItems = false, want true")
	}
}

func TestTodoStore_FormatForInjectionEmptyReturnsBlank(t *testing.T) {
	store := NewTodoStore()
	if got := store.FormatForInjection(); got != "" {
		t.Fatalf("empty FormatForInjection = %q, want \"\"", got)
	}
}

func TestTodoStore_FormatForInjectionFiltersCompletedAndCancelled(t *testing.T) {
	store := NewTodoStore()
	store.Write([]TodoItemInput{
		{ID: "1", Content: "Do thing", Status: "completed"},
		{ID: "2", Content: "Next", Status: "pending"},
		{ID: "3", Content: "Working", Status: "in_progress"},
		{ID: "4", Content: "Abandoned", Status: "cancelled"},
	}, false)

	text := store.FormatForInjection()
	if text == "" {
		t.Fatal("FormatForInjection = \"\", want non-empty")
	}
	if strings.Contains(text, "[x]") || strings.Contains(text, "Do thing") {
		t.Fatalf("completed item leaked into injection: %q", text)
	}
	if strings.Contains(text, "[~]") || strings.Contains(text, "Abandoned") {
		t.Fatalf("cancelled item leaked into injection: %q", text)
	}
	if !strings.Contains(text, "[ ]") || !strings.Contains(text, "Next") {
		t.Fatalf("pending marker missing: %q", text)
	}
	if !strings.Contains(text, "[>]") || !strings.Contains(text, "Working") {
		t.Fatalf("in_progress marker missing: %q", text)
	}
	if !strings.Contains(strings.ToLower(text), "context compression") {
		t.Fatalf("compression preamble missing: %q", text)
	}
}

func TestTodoStore_FormatForInjectionAllInactiveReturnsBlank(t *testing.T) {
	store := NewTodoStore()
	store.Write([]TodoItemInput{
		{ID: "1", Content: "Done", Status: "completed"},
		{ID: "2", Content: "Gone", Status: "cancelled"},
	}, false)
	if got := store.FormatForInjection(); got != "" {
		t.Fatalf("all-inactive FormatForInjection = %q, want \"\"", got)
	}
}

func TestTodoStore_MergeUpdatesExistingByID(t *testing.T) {
	store := NewTodoStore()
	store.Write([]TodoItemInput{
		{ID: "1", Content: "Original", Status: "pending"},
	}, false)
	store.Write([]TodoItemInput{
		{ID: "1", Status: "completed"},
	}, true)

	items := store.Read()
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].Status != "completed" {
		t.Fatalf("status = %q, want completed", items[0].Status)
	}
	if items[0].Content != "Original" {
		t.Fatalf("content = %q, want preserved Original", items[0].Content)
	}
}

func TestTodoStore_MergeAppendsNew(t *testing.T) {
	store := NewTodoStore()
	store.Write([]TodoItemInput{{ID: "1", Content: "First", Status: "pending"}}, false)
	store.Write([]TodoItemInput{{ID: "2", Content: "Second", Status: "pending"}}, true)

	items := store.Read()
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	if items[0].ID != "1" || items[1].ID != "2" {
		t.Fatalf("order = [%s %s], want [1 2]", items[0].ID, items[1].ID)
	}
}

func TestTodoTool_NameAndSchema(t *testing.T) {
	tool := &TodoTool{Store: NewTodoStore()}
	if tool.Name() != "todo" {
		t.Fatalf("Name() = %q, want todo", tool.Name())
	}
	var schema map[string]any
	if err := json.Unmarshal(tool.Schema(), &schema); err != nil {
		t.Fatalf("Schema() parse error = %v", err)
	}
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema.properties missing: %v", schema)
	}
	if _, ok := props["todos"]; !ok {
		t.Fatal("schema missing todos property")
	}
	if _, ok := props["merge"]; !ok {
		t.Fatal("schema missing merge property")
	}
}

func TestTodoTool_ExecuteReadMode(t *testing.T) {
	tool := &TodoTool{Store: NewTodoStore()}
	tool.Store.Write([]TodoItemInput{{ID: "1", Content: "Task", Status: "pending"}}, false)

	raw, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Execute() err = %v", err)
	}
	var payload struct {
		Todos   []TodoItem `json:"todos"`
		Summary struct {
			Total      int `json:"total"`
			Pending    int `json:"pending"`
			InProgress int `json:"in_progress"`
			Completed  int `json:"completed"`
			Cancelled  int `json:"cancelled"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("Unmarshal err = %v", err)
	}
	if payload.Summary.Total != 1 || payload.Summary.Pending != 1 {
		t.Fatalf("summary = %+v, want total=1 pending=1", payload.Summary)
	}
	if len(payload.Todos) != 1 || payload.Todos[0].ID != "1" {
		t.Fatalf("todos = %+v", payload.Todos)
	}
}

func TestTodoTool_ExecuteWriteModeReplace(t *testing.T) {
	tool := &TodoTool{Store: NewTodoStore()}
	raw, err := tool.Execute(context.Background(), json.RawMessage(`{
		"todos":[
			{"id":"1","content":"New","status":"in_progress"},
			{"id":"2","content":"Next","status":"pending"}
		]
	}`))
	if err != nil {
		t.Fatalf("Execute() err = %v", err)
	}
	var payload struct {
		Summary struct {
			Total      int `json:"total"`
			InProgress int `json:"in_progress"`
			Pending    int `json:"pending"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("Unmarshal err = %v", err)
	}
	if payload.Summary.Total != 2 {
		t.Fatalf("total = %d, want 2", payload.Summary.Total)
	}
	if payload.Summary.InProgress != 1 {
		t.Fatalf("in_progress = %d, want 1", payload.Summary.InProgress)
	}
	if payload.Summary.Pending != 1 {
		t.Fatalf("pending = %d, want 1", payload.Summary.Pending)
	}
}

func TestTodoTool_ExecuteWriteModeMergeUpdatesExisting(t *testing.T) {
	tool := &TodoTool{Store: NewTodoStore()}
	tool.Store.Write([]TodoItemInput{{ID: "1", Content: "Original", Status: "pending"}}, false)

	if _, err := tool.Execute(context.Background(), json.RawMessage(`{
		"todos":[{"id":"1","status":"completed"}],
		"merge":true
	}`)); err != nil {
		t.Fatalf("Execute() err = %v", err)
	}
	items := tool.Store.Read()
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].Status != "completed" || items[0].Content != "Original" {
		t.Fatalf("merged item = %+v, want content preserved and status=completed", items[0])
	}
}

func TestTodoTool_ExecuteWithoutStore(t *testing.T) {
	tool := &TodoTool{}
	_, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("Execute() err = nil, want no-store failure")
	}
	if !errors.Is(err, ErrTodoStoreMissing) {
		t.Fatalf("err = %v, want ErrTodoStoreMissing", err)
	}
}

func TestTodoTool_ExecuteInvalidArgs(t *testing.T) {
	tool := &TodoTool{Store: NewTodoStore()}
	_, err := tool.Execute(context.Background(), json.RawMessage(`{"todos": "not-a-list"}`))
	if err == nil {
		t.Fatal("Execute() err = nil, want invalid-args failure")
	}
}
