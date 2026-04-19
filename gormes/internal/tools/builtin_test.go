package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestEcho_RoundTrip(t *testing.T) {
	e := &EchoTool{}
	var _ Tool = e

	out, err := e.Execute(context.Background(), json.RawMessage(`{"text":"hello"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"hello"`) {
		t.Errorf("echo = %s, want hello", out)
	}
}

func TestEcho_EmptyArgs_Error(t *testing.T) {
	e := &EchoTool{}
	_, err := e.Execute(context.Background(), json.RawMessage(`{}`))
	if err == nil {
		t.Error("echo with missing text should error")
	}
}

func TestNow_ReturnsBothFields(t *testing.T) {
	n := &NowTool{}
	var _ Tool = n

	before := time.Now().Unix()
	out, err := n.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	after := time.Now().Unix()

	var payload struct {
		Unix int64  `json:"unix"`
		ISO  string `json:"iso"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Unix < before || payload.Unix > after {
		t.Errorf("unix = %d, want between %d and %d", payload.Unix, before, after)
	}
	if payload.ISO == "" {
		t.Error("iso is empty")
	}
}

func TestRandInt_WithinBounds(t *testing.T) {
	r := &RandIntTool{}
	var _ Tool = r

	out, err := r.Execute(context.Background(), json.RawMessage(`{"min":10,"max":20}`))
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Value int `json:"value"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Value < 10 || payload.Value > 20 {
		t.Errorf("value = %d, want in [10,20]", payload.Value)
	}
}

func TestRandInt_InvertedBounds_Error(t *testing.T) {
	r := &RandIntTool{}
	_, err := r.Execute(context.Background(), json.RawMessage(`{"min":20,"max":10}`))
	if err == nil {
		t.Error("rand_int with min>max should error")
	}
}

func TestBuiltin_DescriptorsValidJSON(t *testing.T) {
	for _, tool := range []Tool{&EchoTool{}, &NowTool{}, &RandIntTool{}} {
		var any map[string]any
		if err := json.Unmarshal(tool.Schema(), &any); err != nil {
			t.Errorf("%s schema invalid JSON: %v", tool.Name(), err)
		}
	}
}
