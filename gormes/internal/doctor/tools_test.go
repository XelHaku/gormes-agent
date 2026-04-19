package doctor

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/XelHaku/golang-hermes-agent/gormes/internal/tools"
)

// brokenSchemaTool intentionally returns a schema that fails validation.
type brokenSchemaTool struct {
	name   string
	schema json.RawMessage
}

func (b *brokenSchemaTool) Name() string            { return b.name }
func (b *brokenSchemaTool) Description() string     { return "broken on purpose" }
func (b *brokenSchemaTool) Schema() json.RawMessage { return b.schema }
func (b *brokenSchemaTool) Timeout() time.Duration  { return 0 }
func (b *brokenSchemaTool) Execute(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
	return nil, nil
}

func TestCheckTools_NilRegistryFails(t *testing.T) {
	r := CheckTools(nil)
	if r.Status != StatusFail {
		t.Errorf("Status = %v, want FAIL", r.Status)
	}
}

func TestCheckTools_EmptyRegistryWarns(t *testing.T) {
	reg := tools.NewRegistry()
	r := CheckTools(reg)
	if r.Status != StatusWarn {
		t.Errorf("Status = %v, want WARN", r.Status)
	}
}

func TestCheckTools_AllBuiltinsPass(t *testing.T) {
	reg := tools.NewRegistry()
	reg.MustRegister(&tools.EchoTool{})
	reg.MustRegister(&tools.NowTool{})
	reg.MustRegister(&tools.RandIntTool{})

	r := CheckTools(reg)
	if r.Status != StatusPass {
		t.Errorf("Status = %v, want PASS (all built-ins should validate)", r.Status)
	}
	if len(r.Items) != 3 {
		t.Errorf("Items len = %d, want 3", len(r.Items))
	}
	// Every item PASS.
	for _, it := range r.Items {
		if it.Status != StatusPass {
			t.Errorf("item %q status = %v, want PASS", it.Name, it.Status)
		}
	}
	// Summary mentions all three names.
	for _, want := range []string{"echo", "now", "rand_int"} {
		if !strings.Contains(r.Summary, want) {
			t.Errorf("Summary = %q, want to contain %q", r.Summary, want)
		}
	}
}

func TestCheckTools_BrokenSchemaFails(t *testing.T) {
	reg := tools.NewRegistry()
	reg.MustRegister(&tools.EchoTool{}) // one good tool
	reg.MustRegister(&brokenSchemaTool{
		name:   "busted",
		schema: json.RawMessage(`{"type":"array"}`), // not "object"
	})

	r := CheckTools(reg)
	if r.Status != StatusFail {
		t.Errorf("Status = %v, want FAIL (one broken tool)", r.Status)
	}
	if !strings.Contains(r.Summary, "busted") {
		t.Errorf("Summary = %q, want to name the broken tool", r.Summary)
	}
	// The good tool is still reported as PASS; the broken one as FAIL.
	var echoItem, bustedItem *ItemInfo
	for i := range r.Items {
		if r.Items[i].Name == "echo" {
			echoItem = &r.Items[i]
		}
		if r.Items[i].Name == "busted" {
			bustedItem = &r.Items[i]
		}
	}
	if echoItem == nil || echoItem.Status != StatusPass {
		t.Errorf("echo item missing or not PASS: %+v", echoItem)
	}
	if bustedItem == nil || bustedItem.Status != StatusFail {
		t.Errorf("busted item missing or not FAIL: %+v", bustedItem)
	}
}

func TestCheckTools_InvalidJSONFails(t *testing.T) {
	reg := tools.NewRegistry()
	reg.MustRegister(&brokenSchemaTool{
		name:   "garbage",
		schema: json.RawMessage(`{not valid json`),
	})
	r := CheckTools(reg)
	if r.Status != StatusFail {
		t.Errorf("Status = %v, want FAIL for invalid JSON", r.Status)
	}
}

func TestCheckTools_MissingTypeFails(t *testing.T) {
	reg := tools.NewRegistry()
	reg.MustRegister(&brokenSchemaTool{
		name:   "typeless",
		schema: json.RawMessage(`{"properties":{}}`),
	})
	r := CheckTools(reg)
	if r.Status != StatusFail {
		t.Errorf("Status = %v, want FAIL for missing type", r.Status)
	}
}

func TestCheckResult_Format_IncludesItems(t *testing.T) {
	reg := tools.NewRegistry()
	reg.MustRegister(&tools.EchoTool{})
	r := CheckTools(reg)
	out := r.Format()
	if !strings.Contains(out, "Toolbox") {
		t.Errorf("Format missing 'Toolbox': %s", out)
	}
	if !strings.Contains(out, "echo") {
		t.Errorf("Format missing item 'echo': %s", out)
	}
}
