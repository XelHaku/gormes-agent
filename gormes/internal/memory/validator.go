package memory

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// entityTypes is the CHECK whitelist from migration3aTo3b's entities table.
var entityTypes = map[string]struct{}{
	"PERSON": {}, "PROJECT": {}, "CONCEPT": {}, "PLACE": {},
	"ORGANIZATION": {}, "TOOL": {}, "OTHER": {},
}

// predicateWhitelist is the CHECK whitelist from migration3aTo3b's
// relationships table. Map-membership check; order irrelevant.
var predicateWhitelist = map[string]struct{}{
	"WORKS_ON": {}, "KNOWS": {}, "LIKES": {}, "DISLIKES": {},
	"HAS_SKILL": {}, "LOCATED_IN": {}, "PART_OF": {}, "RELATED_TO": {},
}

// ValidatedOutput is the cleaned, whitelist-conformant result of
// validating raw LLM extractor output. Every field is safe to pass to
// the graph upsert layer without further sanitation.
type ValidatedOutput struct {
	Entities      []ValidatedEntity
	Relationships []ValidatedRelationship
}

type ValidatedEntity struct {
	Name        string
	Type        string
	Description string
}

type ValidatedRelationship struct {
	Source    string
	Target    string
	Predicate string
	Weight    float64
}

// extractorOutput is the raw wire shape the LLM is instructed to emit.
type extractorOutput struct {
	Entities      []extractedEntity       `json:"entities"`
	Relationships []extractedRelationship `json:"relationships"`
}

type extractedEntity struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

type extractedRelationship struct {
	Source    string  `json:"source"`
	Target    string  `json:"target"`
	Predicate string  `json:"predicate"`
	Weight    float64 `json:"weight"`
}

// ValidateExtractorOutput parses + sanitizes raw LLM JSON into a
// ValidatedOutput. Malformed JSON returns an error; everything else
// coerces silently (invalid types -> OTHER, unknown predicates ->
// RELATED_TO, orphan relationships dropped, etc.).
func ValidateExtractorOutput(raw []byte) (ValidatedOutput, error) {
	var wire extractorOutput
	if err := json.Unmarshal(raw, &wire); err != nil {
		return ValidatedOutput{}, fmt.Errorf("memory: extractor JSON: %w", err)
	}

	seenEntity := make(map[string]struct{}, len(wire.Entities))
	out := ValidatedOutput{}

	for _, e := range wire.Entities {
		name := strings.TrimSpace(e.Name)
		if name == "" {
			continue
		}
		if len(name) > 255 {
			name = name[:255]
		}
		typ := strings.ToUpper(strings.TrimSpace(e.Type))
		if _, ok := entityTypes[typ]; !ok {
			typ = "OTHER"
		}
		desc := strings.TrimSpace(e.Description)
		if len(desc) > 512 {
			desc = desc[:512]
		}
		key := name + "\x00" + typ
		if _, dup := seenEntity[key]; dup {
			continue
		}
		seenEntity[key] = struct{}{}
		out.Entities = append(out.Entities, ValidatedEntity{
			Name: name, Type: typ, Description: desc,
		})
	}

	// Entity name set (any type) for orphan-check of relationships.
	knownNames := make(map[string]struct{}, len(out.Entities))
	for _, e := range out.Entities {
		knownNames[e.Name] = struct{}{}
	}

	seenRel := make(map[string]struct{})
	for _, r := range wire.Relationships {
		src := strings.TrimSpace(r.Source)
		tgt := strings.TrimSpace(r.Target)
		if src == "" || tgt == "" {
			continue
		}
		if _, ok := knownNames[src]; !ok {
			continue
		}
		if _, ok := knownNames[tgt]; !ok {
			continue
		}

		pred := normalizePredicate(r.Predicate)
		if _, ok := predicateWhitelist[pred]; !ok {
			pred = "RELATED_TO"
		}

		w := r.Weight
		if math.IsNaN(w) || w < 0 {
			w = 1.0
		}
		if w > 1.0 {
			w = 1.0
		}

		key := src + "\x00" + tgt + "\x00" + pred
		if _, dup := seenRel[key]; dup {
			continue
		}
		seenRel[key] = struct{}{}
		out.Relationships = append(out.Relationships, ValidatedRelationship{
			Source: src, Target: tgt, Predicate: pred, Weight: w,
		})
	}

	return out, nil
}

// normalizePredicate uppercases and replaces non-alphanumerics with '_'.
// "works on" -> "WORKS_ON". Collapses consecutive underscores and trims
// leading/trailing underscores. Empty input returns "".
func normalizePredicate(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := b.String()
	for strings.Contains(out, "__") {
		out = strings.ReplaceAll(out, "__", "_")
	}
	return strings.Trim(out, "_")
}
