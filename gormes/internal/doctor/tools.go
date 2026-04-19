package doctor

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/XelHaku/golang-hermes-agent/gormes/internal/tools"
)

// CheckTools validates every Tool registered in reg. Each tool's Schema()
// must be valid JSON with top-level `"type": "object"` (OpenAI tool-calling
// convention). A nil registry returns a FAIL result; an empty registry
// returns a WARN.
//
// The result's Items list contains one entry per tool, sorted by name
// (matching registry.Descriptors ordering). On tool schema failure the Note
// carries the specific validation error; on success it carries the tool's
// Description.
func CheckTools(reg *tools.Registry) CheckResult {
	r := CheckResult{Name: "Toolbox"}

	if reg == nil {
		r.Status = StatusFail
		r.Summary = "no tool registry configured"
		return r
	}

	descs := reg.Descriptors()
	if len(descs) == 0 {
		r.Status = StatusWarn
		r.Summary = "no tools registered"
		return r
	}

	// Stable order already — Descriptors returns sorted by name.
	items := make([]ItemInfo, 0, len(descs))
	invalid := 0
	invalidNames := make([]string, 0)

	for _, d := range descs {
		if err := validateToolSchema(d.Schema); err != nil {
			invalid++
			invalidNames = append(invalidNames, d.Name)
			items = append(items, ItemInfo{
				Name:   d.Name,
				Status: StatusFail,
				Note:   "schema: " + err.Error(),
			})
			continue
		}
		items = append(items, ItemInfo{
			Name:   d.Name,
			Status: StatusPass,
			Note:   d.Description,
		})
	}
	r.Items = items

	if invalid == 0 {
		r.Status = StatusPass
		names := make([]string, len(descs))
		for i, d := range descs {
			names[i] = d.Name
		}
		sort.Strings(names)
		r.Summary = fmt.Sprintf("%d tool%s registered (%s)",
			len(descs), plural(len(descs)), joinComma(names))
	} else {
		r.Status = StatusFail
		r.Summary = fmt.Sprintf("%d of %d tool%s have invalid schemas (%s)",
			invalid, len(descs), plural(len(descs)), joinComma(invalidNames))
	}
	return r
}

// validateToolSchema performs lightweight OpenAI-compatible JSON-Schema
// validation: valid JSON, top-level "type":"object", and "properties" must
// be an object (empty is fine). This is NOT a full JSON Schema validator —
// it's an OpenAI tool-calling format gate.
func validateToolSchema(raw json.RawMessage) error {
	if len(raw) == 0 {
		return fmt.Errorf("schema is empty")
	}
	var top map[string]any
	if err := json.Unmarshal(raw, &top); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	typ, ok := top["type"]
	if !ok {
		return fmt.Errorf(`missing "type"`)
	}
	typStr, ok := typ.(string)
	if !ok {
		return fmt.Errorf(`"type" must be a string, got %T`, typ)
	}
	if typStr != "object" {
		return fmt.Errorf(`"type" must be "object", got %q`, typStr)
	}
	if props, ok := top["properties"]; ok {
		if _, isObj := props.(map[string]any); !isObj {
			return fmt.Errorf(`"properties" must be an object, got %T`, props)
		}
	}
	// "required" is optional; if present it must be an array of strings.
	if req, ok := top["required"]; ok {
		arr, isArr := req.([]any)
		if !isArr {
			return fmt.Errorf(`"required" must be an array, got %T`, req)
		}
		for i, v := range arr {
			if _, isStr := v.(string); !isStr {
				return fmt.Errorf(`"required"[%d] must be a string, got %T`, i, v)
			}
		}
	}
	return nil
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func joinComma(ss []string) string {
	if len(ss) == 0 {
		return ""
	}
	out := ss[0]
	for _, s := range ss[1:] {
		out += ", " + s
	}
	return out
}
