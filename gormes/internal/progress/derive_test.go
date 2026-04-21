package progress

import "testing"

func TestSubphase_DerivedStatus(t *testing.T) {
	tests := []struct {
		name string
		sp   Subphase
		want Status
	}{
		{
			name: "explicit status, no items",
			sp:   Subphase{Status: StatusPlanned},
			want: StatusPlanned,
		},
		{
			name: "all items complete",
			sp: Subphase{Items: []Item{
				{Status: StatusComplete}, {Status: StatusComplete},
			}},
			want: StatusComplete,
		},
		{
			name: "any item complete -> in_progress",
			sp: Subphase{Items: []Item{
				{Status: StatusComplete}, {Status: StatusPlanned},
			}},
			want: StatusInProgress,
		},
		{
			name: "any item in_progress",
			sp: Subphase{Items: []Item{
				{Status: StatusInProgress}, {Status: StatusPlanned},
			}},
			want: StatusInProgress,
		},
		{
			name: "all planned",
			sp: Subphase{Items: []Item{
				{Status: StatusPlanned}, {Status: StatusPlanned},
			}},
			want: StatusPlanned,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.sp.DerivedStatus()
			if got != tc.want {
				t.Errorf("DerivedStatus() = %q, want %q", got, tc.want)
			}
		})
	}
}
