package memory

import (
	"testing"
)

func TestValidate_HappyPath(t *testing.T) {
	raw := `{"entities":[
		{"name":"Jose","type":"PERSON","description":"the user"},
		{"name":"Gormes","type":"PROJECT","description":""}
	],"relationships":[
		{"source":"Jose","target":"Gormes","predicate":"WORKS_ON","weight":0.8}
	]}`

	out, err := ValidateExtractorOutput([]byte(raw))
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if len(out.Entities) != 2 {
		t.Errorf("entities count = %d, want 2", len(out.Entities))
	}
	if len(out.Relationships) != 1 {
		t.Errorf("relationships count = %d, want 1", len(out.Relationships))
	}
	if out.Relationships[0].Predicate != "WORKS_ON" {
		t.Errorf("predicate = %q, want WORKS_ON", out.Relationships[0].Predicate)
	}
}

func TestValidate_MalformedJSONReturnsError(t *testing.T) {
	_, err := ValidateExtractorOutput([]byte("not json"))
	if err == nil {
		t.Error("err = nil, want non-nil")
	}
}

func TestValidate_DropsEmptyName(t *testing.T) {
	raw := `{"entities":[
		{"name":"","type":"PERSON","description":""},
		{"name":"Kept","type":"CONCEPT","description":""}
	],"relationships":[]}`

	out, _ := ValidateExtractorOutput([]byte(raw))
	if len(out.Entities) != 1 || out.Entities[0].Name != "Kept" {
		t.Errorf("entities = %+v, want just 'Kept'", out.Entities)
	}
}

func TestValidate_CoercesInvalidTypeToOther(t *testing.T) {
	raw := `{"entities":[
		{"name":"Nowhere","type":"BUILDING","description":""}
	],"relationships":[]}`

	out, _ := ValidateExtractorOutput([]byte(raw))
	if len(out.Entities) != 1 || out.Entities[0].Type != "OTHER" {
		t.Errorf("type = %q, want OTHER", out.Entities[0].Type)
	}
}

func TestValidate_DropsOrphanRelationships(t *testing.T) {
	raw := `{"entities":[
		{"name":"A","type":"PERSON","description":""}
	],"relationships":[
		{"source":"A","target":"B","predicate":"KNOWS","weight":1.0},
		{"source":"A","target":"A","predicate":"KNOWS","weight":1.0}
	]}`

	out, _ := ValidateExtractorOutput([]byte(raw))
	if len(out.Relationships) != 1 {
		t.Errorf("relationships = %+v, want only A->A (B is orphan)", out.Relationships)
	}
}

func TestValidate_ClampsWeight(t *testing.T) {
	raw := `{"entities":[
		{"name":"A","type":"PERSON","description":""},
		{"name":"B","type":"PERSON","description":""}
	],"relationships":[
		{"source":"A","target":"B","predicate":"KNOWS","weight":1.5},
		{"source":"A","target":"B","predicate":"LIKES","weight":-0.3}
	]}`

	out, _ := ValidateExtractorOutput([]byte(raw))
	for _, r := range out.Relationships {
		if r.Weight < 0.0 || r.Weight > 1.0 {
			t.Errorf("predicate=%s weight=%v out of [0,1]", r.Predicate, r.Weight)
		}
	}
}

func TestValidate_NormalizesPredicate(t *testing.T) {
	raw := `{"entities":[
		{"name":"A","type":"PERSON","description":""},
		{"name":"B","type":"PERSON","description":""}
	],"relationships":[
		{"source":"A","target":"B","predicate":"works on","weight":0.5}
	]}`

	out, _ := ValidateExtractorOutput([]byte(raw))
	if len(out.Relationships) != 1 || out.Relationships[0].Predicate != "WORKS_ON" {
		t.Errorf("predicate = %q, want WORKS_ON", out.Relationships[0].Predicate)
	}
}

func TestValidate_CoercesUnknownPredicateToRelatedTo(t *testing.T) {
	raw := `{"entities":[
		{"name":"A","type":"PERSON","description":""},
		{"name":"B","type":"PROJECT","description":""}
	],"relationships":[
		{"source":"A","target":"B","predicate":"BUILT","weight":1.0}
	]}`

	out, _ := ValidateExtractorOutput([]byte(raw))
	if len(out.Relationships) != 1 {
		t.Fatalf("relationships len = %d, want 1", len(out.Relationships))
	}
	if out.Relationships[0].Predicate != "RELATED_TO" {
		t.Errorf("predicate = %q, want RELATED_TO (coerced)", out.Relationships[0].Predicate)
	}
}
