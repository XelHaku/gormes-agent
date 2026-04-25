package progress

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// MarshalJSON emits Progress with phase keys ordered by sortedMapKeys
// (natural-numeric order) so that, e.g., "10" comes after "9" rather than
// between "1" and "2". Without this override, encoding/json would emit map
// keys alphabetically and the on-disk progress.json would drift on every
// round-trip through SaveProgress.
func (p Progress) MarshalJSON() ([]byte, error) {
	phases, err := marshalOrderedPhases(p.Phases)
	if err != nil {
		return nil, err
	}
	aux := struct {
		Meta   Meta            `json:"meta"`
		Phases json.RawMessage `json:"phases"`
	}{
		Meta:   p.Meta,
		Phases: phases,
	}
	return json.Marshal(aux)
}

// MarshalJSON emits Phase with subphase keys in natural-numeric order so
// that, e.g., "2.B.10" comes after "2.B.2" rather than before. Mirrors the
// real Phase struct so all fields round-trip; only the Subphases map's key
// ordering is overridden.
func (ph Phase) MarshalJSON() ([]byte, error) {
	subphases, err := marshalOrderedSubphases(ph.Subphases)
	if err != nil {
		return nil, err
	}
	aux := struct {
		Name           string          `json:"name"`
		Deliverable    string          `json:"deliverable"`
		DependencyNote string          `json:"dependency_note,omitempty"`
		Subphases      json.RawMessage `json:"subphases"`
	}{
		Name:           ph.Name,
		Deliverable:    ph.Deliverable,
		DependencyNote: ph.DependencyNote,
		Subphases:      subphases,
	}
	return json.Marshal(aux)
}

// marshalOrderedPhases emits a JSON object whose keys are the phase IDs of
// m in sortedMapKeys order. The result is compact (no indentation); the
// caller's surrounding json.Encoder.SetIndent reflows the output.
func marshalOrderedPhases(m map[string]Phase) (json.RawMessage, error) {
	if m == nil {
		return []byte("null"), nil
	}
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, key := range sortedMapKeys(m) {
		if i > 0 {
			buf.WriteByte(',')
		}
		k, err := json.Marshal(key)
		if err != nil {
			return nil, fmt.Errorf("marshal phase key %q: %w", key, err)
		}
		buf.Write(k)
		buf.WriteByte(':')
		v := m[key]
		body, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("marshal phase %q: %w", key, err)
		}
		buf.Write(body)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// marshalOrderedSubphases is the Subphase analogue of marshalOrderedPhases.
func marshalOrderedSubphases(m map[string]Subphase) (json.RawMessage, error) {
	if m == nil {
		return []byte("null"), nil
	}
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, key := range sortedMapKeys(m) {
		if i > 0 {
			buf.WriteByte(',')
		}
		k, err := json.Marshal(key)
		if err != nil {
			return nil, fmt.Errorf("marshal subphase key %q: %w", key, err)
		}
		buf.Write(k)
		buf.WriteByte(':')
		v := m[key]
		body, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("marshal subphase %q: %w", key, err)
		}
		buf.Write(body)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}
