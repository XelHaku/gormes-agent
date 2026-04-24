package autoloop

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeCandidatesSkipsCompleteAndSortsActiveFirst(t *testing.T) {
	path := writeProgressJSON(t, `{
		"phases": {
			"3": {
				"subphases": {
					"3.E.6": {
						"items": [
							{"item_name": "planned candidate", "status": "planned"},
							{"item_name": "complete candidate", "status": "complete"},
							{"item_name": "active candidate", "status": "IN_PROGRESS"}
						]
					}
				}
			}
		}
	}`)

	got, err := NormalizeCandidates(path, CandidateOptions{ActiveFirst: true})
	if err != nil {
		t.Fatalf("NormalizeCandidates() error = %v", err)
	}

	want := []Candidate{
		{PhaseID: "3", SubphaseID: "3.E.6", ItemName: "active candidate", Status: "in_progress"},
		{PhaseID: "3", SubphaseID: "3.E.6", ItemName: "planned candidate", Status: "planned"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NormalizeCandidates() = %#v, want %#v", got, want)
	}
}

func TestNormalizeCandidatesPriorityBoostWins(t *testing.T) {
	path := writeProgressJSON(t, `{
		"phases": {
			"3": {
				"subphases": {
					"3.E.7": {
						"items": [
							{"name": "boosted planned candidate", "status": "planned"}
						]
					},
					"3.E.6": {
						"items": [
							{"name": "normal active candidate", "status": "in_progress"}
						]
					}
				}
			}
		}
	}`)

	got, err := NormalizeCandidates(path, CandidateOptions{
		ActiveFirst:   true,
		PriorityBoost: []string{" 3.e.7 "},
	})
	if err != nil {
		t.Fatalf("NormalizeCandidates() error = %v", err)
	}

	want := []Candidate{
		{PhaseID: "3", SubphaseID: "3.E.7", ItemName: "boosted planned candidate", Status: "planned"},
		{PhaseID: "3", SubphaseID: "3.E.6", ItemName: "normal active candidate", Status: "in_progress"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NormalizeCandidates() = %#v, want %#v", got, want)
	}
}

func TestNormalizeCandidatesPreservesExecutionMetadataAndSkipsBlockedUmbrella(t *testing.T) {
	path := writeProgressJSON(t, `{
		"phases": {
			"3": {
				"subphases": {
					"3.E.6": {
						"items": [
							{
								"item_name": "ready candidate",
								"status": "planned",
								"priority": " P0 ",
								"contract": " Control plane handoff ",
								"contract_status": " DRAFT ",
								"slice_size": " Small ",
								"execution_owner": " Agent ",
								"trust_class": " T1 ",
								"degraded_mode": " use legacy path ",
								"fixture": " progress_fixture.json ",
								"source_refs": [" docs/plan.md ", " "],
								"blocked_by": [],
								"unblocks": [" task 4 ", ""],
								"ready_when": [" tests pass ", " "],
								"not_ready_when": [" schema unsettled "],
								"acceptance": [" metadata retained "],
								"write_scope": [" internal/autoloop "],
								"test_commands": [" go test ./internal/autoloop "],
								"done_signal": " commit merged ",
								"note": " Keep human note casing. "
							},
							{
								"item_name": "blocked candidate",
								"status": "planned",
								"blocked_by": ["task 2"]
							},
							{
								"item_name": "umbrella candidate",
								"status": "planned",
								"slice_size": "umbrella"
							}
						]
					}
				}
			}
		}
	}`)

	got, err := NormalizeCandidates(path, CandidateOptions{})
	if err != nil {
		t.Fatalf("NormalizeCandidates() error = %v", err)
	}

	want := []Candidate{
		{
			PhaseID:        "3",
			SubphaseID:     "3.E.6",
			ItemName:       "ready candidate",
			Status:         "planned",
			Priority:       "P0",
			Contract:       "Control plane handoff",
			ContractStatus: "draft",
			SliceSize:      "small",
			ExecutionOwner: "agent",
			TrustClass:     "T1",
			DegradedMode:   "use legacy path",
			Fixture:        "progress_fixture.json",
			SourceRefs:     []string{"docs/plan.md"},
			BlockedBy:      nil,
			Unblocks:       []string{"task 4"},
			ReadyWhen:      []string{"tests pass"},
			NotReadyWhen:   []string{"schema unsettled"},
			Acceptance:     []string{"metadata retained"},
			WriteScope:     []string{"internal/autoloop"},
			TestCommands:   []string{"go test ./internal/autoloop"},
			DoneSignal:     "commit merged",
			Note:           "Keep human note casing.",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NormalizeCandidates() = %#v, want %#v", got, want)
	}
	if got[0].SelectionReason() != "P0 handoff" {
		t.Fatalf("SelectionReason() = %q, want %q", got[0].SelectionReason(), "P0 handoff")
	}
}

func TestNormalizeCandidatesUsesExecutionBuckets(t *testing.T) {
	path := writeProgressJSON(t, `{
		"phases": {
			"3": {
				"subphases": {
					"3.E.6": {
						"items": [
							{"item_name": "draft candidate", "status": "planned", "contract_status": "draft"},
							{"item_name": "fixture candidate", "status": "planned", "fixture": "ready.json"},
							{"item_name": "active candidate", "status": "in_progress"},
							{"item_name": "p0 candidate", "status": "planned", "priority": "P0"},
							{"item_name": "unblocking candidate", "status": "planned", "unblocks": ["task 4"]}
						]
					}
				}
			}
		}
	}`)

	got, err := NormalizeCandidates(path, CandidateOptions{})
	if err != nil {
		t.Fatalf("NormalizeCandidates() error = %v", err)
	}

	var gotNames []string
	for _, candidate := range got {
		gotNames = append(gotNames, candidate.ItemName)
	}
	wantNames := []string{"p0 candidate", "active candidate", "fixture candidate", "unblocking candidate", "draft candidate"}
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("candidate order = %#v, want %#v", gotNames, wantNames)
	}
}

func TestNormalizeCandidatesHonorsMaxPhase(t *testing.T) {
	path := writeProgressJSON(t, `{
		"phases": {
			"3": {
				"subphases": {
					"3.E": {
						"items": [
							{"name": "phase 3 candidate", "status": "planned"}
						]
					}
				}
			},
			"4": {
				"subphases": {
					"4.A": {
						"items": [
							{"name": "phase 4 candidate", "status": "in_progress"}
						]
					}
				}
			}
		}
	}`)

	got, err := NormalizeCandidates(path, CandidateOptions{ActiveFirst: true, MaxPhase: 3})
	if err != nil {
		t.Fatalf("NormalizeCandidates() error = %v", err)
	}

	want := []Candidate{
		{PhaseID: "3", SubphaseID: "3.E", ItemName: "phase 3 candidate", Status: "planned"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NormalizeCandidates() = %#v, want %#v", got, want)
	}
}

func TestNormalizeCandidatesUsesSubPhasesFallback(t *testing.T) {
	path := writeProgressJSON(t, `{
		"phases": {
			"4": {
				"sub_phases": {
					"4.A.1": {
						"items": [
							{"item_name": "fallback candidate", "status": "planned"}
						]
					}
				}
			}
		}
	}`)

	got, err := NormalizeCandidates(path, CandidateOptions{})
	if err != nil {
		t.Fatalf("NormalizeCandidates() error = %v", err)
	}

	want := []Candidate{
		{PhaseID: "4", SubphaseID: "4.A.1", ItemName: "fallback candidate", Status: "planned"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NormalizeCandidates() = %#v, want %#v", got, want)
	}
}

func TestNormalizeCandidatesPrefersSubphasesWhenBothKeysExist(t *testing.T) {
	path := writeProgressJSON(t, `{
		"phases": {
			"4": {
				"subphases": {
					"4.A.1": {
						"items": [
							{"item_name": "preferred candidate", "status": "planned"}
						]
					}
				},
				"sub_phases": {
					"4.A.2": {
						"items": [
							{"item_name": "fallback candidate", "status": "planned"}
						]
					}
				}
			}
		}
	}`)

	got, err := NormalizeCandidates(path, CandidateOptions{})
	if err != nil {
		t.Fatalf("NormalizeCandidates() error = %v", err)
	}

	want := []Candidate{
		{PhaseID: "4", SubphaseID: "4.A.1", ItemName: "preferred candidate", Status: "planned"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NormalizeCandidates() = %#v, want %#v", got, want)
	}
}

func TestNormalizeCandidatesItemNameFallbacksAndUnknownStatus(t *testing.T) {
	path := writeProgressJSON(t, `{
		"phases": {
			"5": {
				"subphases": {
					"5.B.1": {
						"items": [
							{"item_name": "item-name candidate", "name": "ignored name", "status": "planned"},
							{"item_name": " ", "name": "name candidate", "title": "ignored title", "status": "planned"},
							{"name": " ", "title": "title candidate", "id": "ignored id", "status": "planned"},
							{"title": " ", "id": "id candidate"},
							{"item_name": " "}
						]
					}
				}
			}
		}
	}`)

	got, err := NormalizeCandidates(path, CandidateOptions{})
	if err != nil {
		t.Fatalf("NormalizeCandidates() error = %v", err)
	}

	want := []Candidate{
		{PhaseID: "5", SubphaseID: "5.B.1", ItemName: "item-name candidate", Status: "planned"},
		{PhaseID: "5", SubphaseID: "5.B.1", ItemName: "name candidate", Status: "planned"},
		{PhaseID: "5", SubphaseID: "5.B.1", ItemName: "title candidate", Status: "planned"},
		{PhaseID: "5", SubphaseID: "5.B.1", ItemName: "id candidate", Status: "unknown"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NormalizeCandidates() = %#v, want %#v", got, want)
	}
}

func TestNormalizeCandidatesDeduplicatesByPhaseSubphaseAndItemName(t *testing.T) {
	path := writeProgressJSON(t, `{
		"phases": {
			"6": {
				"subphases": {
					"6.C.1": {
						"items": [
							{"item_name": "duplicate candidate", "status": "planned"},
							{"item_name": "duplicate candidate", "status": "in_progress"}
						]
					}
				}
			}
		}
	}`)

	got, err := NormalizeCandidates(path, CandidateOptions{ActiveFirst: true})
	if err != nil {
		t.Fatalf("NormalizeCandidates() error = %v", err)
	}

	want := []Candidate{
		{PhaseID: "6", SubphaseID: "6.C.1", ItemName: "duplicate candidate", Status: "planned"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NormalizeCandidates() = %#v, want %#v", got, want)
	}
}

func TestNormalizeCandidatesReturnsMalformedJSONError(t *testing.T) {
	path := writeProgressJSON(t, `{"phases":`)

	_, err := NormalizeCandidates(path, CandidateOptions{})
	if err == nil {
		t.Fatal("NormalizeCandidates() error = nil, want error")
	}
}

func TestNormalizeCandidatesReturnsMissingFileError(t *testing.T) {
	_, err := NormalizeCandidates(filepath.Join(t.TempDir(), "missing.json"), CandidateOptions{})
	if err == nil {
		t.Fatal("NormalizeCandidates() error = nil, want error")
	}

	if !strings.Contains(err.Error(), "missing.json") {
		t.Fatalf("NormalizeCandidates() error = %q, want missing filename", err)
	}
}

func writeProgressJSON(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "progress.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	return path
}
