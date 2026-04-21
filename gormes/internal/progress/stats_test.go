package progress

import "testing"

func TestStats_SingleSubphase(t *testing.T) {
	p := &Progress{
		Phases: map[string]Phase{
			"1": {
				Subphases: map[string]Subphase{
					"1.A": {Items: []Item{{Status: StatusComplete}, {Status: StatusPlanned}}},
				},
			},
		},
	}
	s := p.Stats()
	if s.Subphases.Total != 1 {
		t.Errorf("Subphases.Total = %d, want 1", s.Subphases.Total)
	}
	if s.Subphases.InProgress != 1 {
		t.Errorf("Subphases.InProgress = %d, want 1", s.Subphases.InProgress)
	}
	if s.Items.Total != 2 || s.Items.Complete != 1 || s.Items.Planned != 1 {
		t.Errorf("Items = %+v, want total=2 complete=1 planned=1", s.Items)
	}
}

func TestStats_MixedPhases(t *testing.T) {
	p := &Progress{
		Phases: map[string]Phase{
			"1": {Subphases: map[string]Subphase{
				"1.A": {Items: []Item{{Status: StatusComplete}, {Status: StatusComplete}}},
				"1.B": {Items: []Item{{Status: StatusComplete}}},
			}},
			"2": {Subphases: map[string]Subphase{
				"2.A": {Status: StatusPlanned},
				"2.B": {Items: []Item{{Status: StatusInProgress}}},
			}},
		},
	}
	s := p.Stats()
	if s.Subphases.Total != 4 {
		t.Errorf("Subphases.Total = %d, want 4", s.Subphases.Total)
	}
	if s.Subphases.Complete != 2 {
		t.Errorf("Subphases.Complete = %d, want 2", s.Subphases.Complete)
	}
	if s.Subphases.InProgress != 1 {
		t.Errorf("Subphases.InProgress = %d, want 1", s.Subphases.InProgress)
	}
	if s.Subphases.Planned != 1 {
		t.Errorf("Subphases.Planned = %d, want 1", s.Subphases.Planned)
	}
}
