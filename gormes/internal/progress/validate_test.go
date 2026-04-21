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

func TestValidate_AccumulatesMultipleErrors(t *testing.T) {
	p := &Progress{
		Meta: Meta{Version: "2.0"},
		Phases: map[string]Phase{
			"1": {Subphases: map[string]Subphase{
				"1.A": {Items: []Item{{Name: "bad", Status: "nope"}}},
				"1.B": {Items: []Item{{Name: "x", Status: StatusComplete}}, Status: StatusComplete}, // both
			}},
			"2": {Subphases: map[string]Subphase{
				"2.A": {}, // neither
			}},
		},
	}
	err := Validate(p)
	if err == nil {
		t.Fatalf("Validate() = nil, want multiple errors")
	}
	// errors.Join formats as one error per line separated by \n.
	msg := err.Error()
	for _, want := range []string{
		"phase 1 subphase 1.A", // bad item status
		"phase 1 subphase 1.B", // both items and status
		"phase 2 subphase 2.A", // neither
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error missing %q:\n%s", want, msg)
		}
	}
}
