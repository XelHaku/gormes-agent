package hermes

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

// ToolCallRepairError reports a provider-emitted tool call that could not be
// safely reconciled with the tool schemas advertised on the current request.
type ToolCallRepairError struct {
	ToolCallID string
	ToolName   string
	Reason     string
}

func (e *ToolCallRepairError) Error() string {
	if e == nil {
		return "hermes: tool-call argument repair failed"
	}
	name := e.ToolName
	if name == "" {
		name = "<unknown>"
	}
	if e.ToolCallID == "" {
		return fmt.Sprintf("hermes: tool-call argument repair failed for %s: %s", name, e.Reason)
	}
	return fmt.Sprintf("hermes: tool-call argument repair failed for %s (%s): %s", name, e.ToolCallID, e.Reason)
}

// SanitizeToolDescriptors returns a deep copy of descriptors with schemas
// normalized to the conservative object-shaped subset accepted by provider
// tool parsers.
func SanitizeToolDescriptors(descriptors []ToolDescriptor) []ToolDescriptor {
	if len(descriptors) == 0 {
		return nil
	}
	out := make([]ToolDescriptor, 0, len(descriptors))
	for _, d := range descriptors {
		out = append(out, ToolDescriptor{
			Name:        d.Name,
			Description: d.Description,
			Schema:      sanitizeToolSchema(d.Schema),
		})
	}
	return out
}

// RepairToolCalls repairs deterministic JSON malformations and validates the
// final arguments against the currently advertised tool descriptors.
func RepairToolCalls(calls []ToolCall, descriptors []ToolDescriptor) ([]ToolCall, error) {
	if len(calls) == 0 {
		return nil, nil
	}
	schemas, err := advertisedToolSchemas(descriptors)
	if err != nil {
		return nil, err
	}
	out := make([]ToolCall, 0, len(calls))
	for _, call := range calls {
		schema, ok := schemas[call.Name]
		if !ok {
			return nil, &ToolCallRepairError{
				ToolCallID: call.ID,
				ToolName:   call.Name,
				Reason:     "tool was not advertised on the current request",
			}
		}
		args, values, err := repairToolCallArguments(call.Arguments)
		if err != nil {
			return nil, &ToolCallRepairError{ToolCallID: call.ID, ToolName: call.Name, Reason: err.Error()}
		}
		if err := validateArgumentsAgainstSchema(values, schema); err != nil {
			return nil, &ToolCallRepairError{ToolCallID: call.ID, ToolName: call.Name, Reason: err.Error()}
		}
		call.Arguments = args
		out = append(out, call)
	}
	return out, nil
}

func sanitizeToolSchema(raw json.RawMessage) json.RawMessage {
	var node any
	if len(bytes.TrimSpace(raw)) == 0 || json.Unmarshal(raw, &node) != nil {
		return json.RawMessage(`{"type":"object","properties":{}}`)
	}
	sanitized := sanitizeSchemaNode(node)
	top, ok := sanitized.(map[string]any)
	if !ok {
		top = map[string]any{}
	}
	if typ, _ := top["type"].(string); typ != "object" {
		top["type"] = "object"
	}
	if _, ok := top["properties"].(map[string]any); !ok {
		top["properties"] = map[string]any{}
	}
	pruneRequired(top)
	if reflect.DeepEqual(node, top) {
		return append(json.RawMessage(nil), raw...)
	}
	out, err := json.Marshal(top)
	if err != nil {
		return json.RawMessage(`{"type":"object","properties":{}}`)
	}
	return out
}

func sanitizeSchemaNode(node any) any {
	switch v := node.(type) {
	case string:
		if isJSONSchemaType(v) {
			if v == "object" {
				return map[string]any{"type": "object", "properties": map[string]any{}}
			}
			return map[string]any{"type": v}
		}
		return map[string]any{"type": "object", "properties": map[string]any{}}
	case []any:
		out := make([]any, 0, len(v))
		for _, item := range v {
			out = append(out, sanitizeSchemaNode(item))
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(v)+1)
		for key, value := range v {
			switch key {
			case "type":
				out[key] = sanitizeSchemaType(value, out)
			case "properties", "$defs", "definitions":
				if children, ok := value.(map[string]any); ok {
					clean := make(map[string]any, len(children))
					for name, child := range children {
						clean[name] = sanitizeSchemaNode(child)
					}
					out[key] = clean
				} else if key == "properties" {
					out[key] = map[string]any{}
				} else {
					out[key] = value
				}
			case "items", "additionalProperties":
				if _, ok := value.(bool); ok {
					out[key] = value
				} else {
					out[key] = sanitizeSchemaNode(value)
				}
			case "anyOf", "oneOf", "allOf":
				if list, ok := value.([]any); ok {
					clean := make([]any, 0, len(list))
					for _, item := range list {
						clean = append(clean, sanitizeSchemaNode(item))
					}
					out[key] = clean
				} else {
					out[key] = value
				}
			case "required", "enum", "examples":
				out[key] = value
			default:
				switch value.(type) {
				case map[string]any, []any:
					out[key] = sanitizeSchemaNode(value)
				default:
					out[key] = value
				}
			}
		}
		if typ, _ := out["type"].(string); typ == "object" {
			if _, ok := out["properties"].(map[string]any); !ok {
				out["properties"] = map[string]any{}
			}
			pruneRequired(out)
		}
		return out
	default:
		return node
	}
}

func sanitizeSchemaType(value any, out map[string]any) any {
	if list, ok := value.([]any); ok {
		first := ""
		for _, item := range list {
			s, ok := item.(string)
			if !ok {
				continue
			}
			if s == "null" {
				out["nullable"] = true
				continue
			}
			if first == "" {
				first = s
			}
		}
		if first == "" {
			return "object"
		}
		return first
	}
	return value
}

func pruneRequired(schema map[string]any) {
	required, ok := schema["required"].([]any)
	if !ok {
		return
	}
	props, _ := schema["properties"].(map[string]any)
	valid := make([]any, 0, len(required))
	for _, item := range required {
		name, ok := item.(string)
		if !ok {
			continue
		}
		if _, exists := props[name]; exists {
			valid = append(valid, name)
		}
	}
	if len(valid) == 0 {
		delete(schema, "required")
		return
	}
	schema["required"] = valid
}

func isJSONSchemaType(t string) bool {
	switch t {
	case "object", "string", "number", "integer", "boolean", "array", "null":
		return true
	default:
		return false
	}
}

func advertisedToolSchemas(descriptors []ToolDescriptor) (map[string]map[string]any, error) {
	out := make(map[string]map[string]any, len(descriptors))
	for _, d := range SanitizeToolDescriptors(descriptors) {
		name := strings.TrimSpace(d.Name)
		if name == "" {
			continue
		}
		var schema map[string]any
		dec := json.NewDecoder(bytes.NewReader(d.Schema))
		dec.UseNumber()
		if err := dec.Decode(&schema); err != nil {
			return nil, &ToolCallRepairError{ToolName: name, Reason: "advertised schema is invalid JSON: " + err.Error()}
		}
		out[name] = schema
	}
	return out, nil
}

func repairToolCallArguments(raw json.RawMessage) (json.RawMessage, map[string]any, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "None" {
		trimmed = "{}"
	}
	if canonical, values, err := parseJSONObject(trimmed); err == nil {
		return canonical, values, nil
	}
	candidate, ok := normalizeJSONDelimiters(trimmed)
	if !ok {
		return nil, nil, fmt.Errorf("arguments are not deterministically repairable")
	}
	candidate = stripTrailingCommas(candidate)
	if canonical, values, err := parseJSONObject(candidate); err == nil {
		return canonical, values, nil
	}
	return nil, nil, fmt.Errorf("arguments are not deterministically repairable")
}

func parseJSONObject(raw string) (json.RawMessage, map[string]any, error) {
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return nil, nil, err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, nil, fmt.Errorf("arguments contain trailing data")
	}
	obj, ok := value.(map[string]any)
	if !ok {
		return nil, nil, fmt.Errorf("arguments must be a JSON object")
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return nil, nil, err
	}
	return canonical, obj, nil
}

func normalizeJSONDelimiters(raw string) (string, bool) {
	var out strings.Builder
	stack := make([]byte, 0, 4)
	inString := false
	escaped := false

	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		if inString {
			switch {
			case escaped:
				out.WriteByte(ch)
				escaped = false
			case ch == '\\':
				out.WriteByte(ch)
				escaped = true
			case ch == '"':
				out.WriteByte(ch)
				inString = false
			case ch < 0x20:
				writeEscapedControl(&out, ch)
			default:
				out.WriteByte(ch)
			}
			continue
		}

		switch ch {
		case '"':
			inString = true
			out.WriteByte(ch)
		case '{':
			stack = append(stack, '}')
			out.WriteByte(ch)
		case '[':
			stack = append(stack, ']')
			out.WriteByte(ch)
		case '}', ']':
			if len(stack) == 0 || stack[len(stack)-1] != ch {
				continue
			}
			stack = stack[:len(stack)-1]
			out.WriteByte(ch)
		default:
			out.WriteByte(ch)
		}
	}
	if inString || escaped {
		return "", false
	}

	candidate := strings.TrimSpace(out.String())
	for strings.HasSuffix(candidate, ",") {
		candidate = strings.TrimSpace(strings.TrimSuffix(candidate, ","))
	}
	if strings.HasSuffix(candidate, ":") {
		return "", false
	}
	for i := len(stack) - 1; i >= 0; i-- {
		candidate += string(stack[i])
	}
	return candidate, true
}

func writeEscapedControl(out *strings.Builder, ch byte) {
	switch ch {
	case '\n':
		out.WriteString(`\n`)
	case '\r':
		out.WriteString(`\r`)
	case '\t':
		out.WriteString(`\t`)
	default:
		out.WriteString(fmt.Sprintf(`\u%04x`, ch))
	}
}

func stripTrailingCommas(raw string) string {
	var out strings.Builder
	inString := false
	escaped := false
	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		if inString {
			out.WriteByte(ch)
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
			} else if ch == '"' {
				inString = false
			}
			continue
		}
		if ch == '"' {
			inString = true
			out.WriteByte(ch)
			continue
		}
		if ch == ',' {
			j := i + 1
			for j < len(raw) && isJSONWhitespace(raw[j]) {
				j++
			}
			if j < len(raw) && (raw[j] == '}' || raw[j] == ']') {
				continue
			}
		}
		out.WriteByte(ch)
	}
	return out.String()
}

func isJSONWhitespace(ch byte) bool {
	return ch == ' ' || ch == '\n' || ch == '\r' || ch == '\t'
}

func validateArgumentsAgainstSchema(args map[string]any, schema map[string]any) error {
	for _, name := range schemaRequired(schema) {
		if _, ok := args[name]; !ok {
			return fmt.Errorf("missing required argument %q", name)
		}
	}

	props, _ := schema["properties"].(map[string]any)
	for name, value := range args {
		prop, ok := props[name]
		if !ok {
			if allowsAdditionalProperties(schema, name, value) {
				continue
			}
			return fmt.Errorf("unexpected argument %q", name)
		}
		propSchema, _ := prop.(map[string]any)
		if err := validateValueAgainstSchema(name, value, propSchema); err != nil {
			return err
		}
	}
	return nil
}

func schemaRequired(schema map[string]any) []string {
	raw, ok := schema["required"].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

func allowsAdditionalProperties(schema map[string]any, name string, value any) bool {
	additional, ok := schema["additionalProperties"]
	if !ok {
		return true
	}
	if allowed, ok := additional.(bool); ok {
		return allowed
	}
	additionalSchema, ok := additional.(map[string]any)
	if !ok {
		return true
	}
	return validateValueAgainstSchema(name, value, additionalSchema) == nil
}

func validateValueAgainstSchema(path string, value any, schema map[string]any) error {
	if len(schema) == 0 {
		return nil
	}
	if value == nil {
		if nullable, _ := schema["nullable"].(bool); nullable {
			return nil
		}
		if typ, _ := schema["type"].(string); typ == "null" || typ == "" {
			return nil
		}
		return fmt.Errorf("argument %q must not be null", path)
	}
	if enum, ok := schema["enum"].([]any); ok && !enumContains(enum, value) {
		return fmt.Errorf("argument %q is not an allowed value", path)
	}
	typ, _ := schema["type"].(string)
	switch typ {
	case "", "null":
		return nil
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("argument %q must be a string", path)
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("argument %q must be a boolean", path)
		}
	case "number":
		if !isJSONNumber(value) {
			return fmt.Errorf("argument %q must be a number", path)
		}
	case "integer":
		if !isJSONInteger(value) {
			return fmt.Errorf("argument %q must be an integer", path)
		}
	case "object":
		child, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("argument %q must be an object", path)
		}
		if err := validateArgumentsAgainstSchema(child, schema); err != nil {
			return err
		}
	case "array":
		items, ok := value.([]any)
		if !ok {
			return fmt.Errorf("argument %q must be an array", path)
		}
		itemSchema, _ := schema["items"].(map[string]any)
		for i, item := range items {
			if err := validateValueAgainstSchema(fmt.Sprintf("%s[%d]", path, i), item, itemSchema); err != nil {
				return err
			}
		}
	default:
		return nil
	}
	return nil
}

func enumContains(enum []any, value any) bool {
	valueJSON, err := json.Marshal(value)
	if err != nil {
		return false
	}
	for _, item := range enum {
		itemJSON, err := json.Marshal(item)
		if err == nil && bytes.Equal(itemJSON, valueJSON) {
			return true
		}
	}
	return false
}

func isJSONNumber(value any) bool {
	switch n := value.(type) {
	case json.Number:
		_, err := n.Float64()
		return err == nil
	case float64:
		return true
	default:
		return false
	}
}

func isJSONInteger(value any) bool {
	switch n := value.(type) {
	case json.Number:
		if _, err := n.Int64(); err == nil {
			return true
		}
		_, err := strconv.ParseInt(n.String(), 10, 64)
		return err == nil
	case float64:
		return n == float64(int64(n))
	default:
		return false
	}
}
