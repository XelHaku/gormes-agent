package progress

import (
	"strings"
	"testing"
)

func TestValidate_OK(t *testing.T) {
	p := &Progress{
		Meta: Meta{Version: "2.0"},
		Phases: map[string]Phase{
			"1": {Subphases: map[string]Subphase{
				"1.A": {Items: []Item{{Name: "x", Status: StatusComplete}}},
				"1.B": {Status: StatusPlanned},
			}},
		},
	}
	if err := Validate(p); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}

func TestValidate_RejectsBadVersion(t *testing.T) {
	p := &Progress{Meta: Meta{Version: "1.0"}}
	err := Validate(p)
	if err == nil || !strings.Contains(err.Error(), "version") {
		t.Errorf("Validate() = %v, want version error", err)
	}
}

func TestValidate_RejectsBadStatus(t *testing.T) {
	p := &Progress{
		Meta: Meta{Version: "2.0"},
		Phases: map[string]Phase{"1": {Subphases: map[string]Subphase{
			"1.A": {Items: []Item{{Name: "x", Status: "done"}}},
		}}},
	}
	err := Validate(p)
	if err == nil || !strings.Contains(err.Error(), "status") {
		t.Errorf("Validate() = %v, want status error", err)
	}
}

func TestValidate_RejectsBothItemsAndStatus(t *testing.T) {
	p := &Progress{
		Meta: Meta{Version: "2.0"},
		Phases: map[string]Phase{"1": {Subphases: map[string]Subphase{
			"1.A": {
				Items:  []Item{{Name: "x", Status: StatusComplete}},
				Status: StatusComplete,
			},
		}}},
	}
	err := Validate(p)
	if err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Errorf("Validate() = %v, want exactly-one error", err)
	}
}

func TestValidate_RejectsNeither(t *testing.T) {
	p := &Progress{
		Meta: Meta{Version: "2.0"},
		Phases: map[string]Phase{"1": {Subphases: map[string]Subphase{
			"1.A": {}, // no items, no status
		}}},
	}
	err := Validate(p)
	if err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Errorf("Validate() = %v, want exactly-one error", err)
	}
}
