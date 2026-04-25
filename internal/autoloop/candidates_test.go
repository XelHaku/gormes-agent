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
								{"item_name": "planned candidate", "status": "planned", "contract": "planned contract", "contract_status": "draft"},
								{"item_name": "complete candidate", "status": "complete", "contract": "complete contract", "contract_status": "draft"},
								{"item_name": "active candidate", "status": "IN_PROGRESS", "contract": "active contract"}
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

	wantNames := []string{"active candidate", "planned candidate"}
	if gotNames := candidateNames(got); !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("candidate names = %#v, want %#v", gotNames, wantNames)
	}
}

func TestNormalizeCandidatesPriorityBoostWins(t *testing.T) {
	path := writeProgressJSON(t, `{
		"phases": {
				"3": {
					"subphases": {
					"3.E.7": {
							"items": [
								{"name": "boosted planned candidate", "status": "planned", "contract": "boosted contract", "contract_status": "draft"}
							]
						},
					"3.E.6": {
							"items": [
								{"name": "normal active candidate", "status": "in_progress", "contract": "active contract"}
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

	wantNames := []string{"boosted planned candidate", "normal active candidate"}
	if gotNames := candidateNames(got); !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("candidate names = %#v, want %#v", gotNames, wantNames)
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
								"trust_class": ["system"],
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
									"done_signal": ["provider transcript replay passes"],
									"note": " Keep human note casing. "
								},
								{
									"item_name": "blocked candidate",
									"status": "planned",
									"contract": "blocked contract",
									"contract_status": "draft",
									"blocked_by": ["task 2"]
								},
								{
									"item_name": "umbrella candidate",
									"status": "planned",
									"contract": "umbrella contract",
									"contract_status": "draft",
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
			TrustClass:     []string{"system"},
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
			DoneSignal:     []string{"provider transcript replay passes"},
			Note:           "Keep human note casing.",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NormalizeCandidates() = %#v, want %#v", got, want)
	}
	if got[0].SelectionReason() != "P0 handoff" {
		t.Fatalf("SelectionReason() = %q, want %q", got[0].SelectionReason(), "P0 handoff")
	}
	if !reflect.DeepEqual(got[0].TrustClass, []string{"system"}) {
		t.Fatalf("TrustClass = %#v, want %#v", got[0].TrustClass, []string{"system"})
	}
	if !reflect.DeepEqual(got[0].DoneSignal, []string{"provider transcript replay passes"}) {
		t.Fatalf("DoneSignal = %#v, want %#v", got[0].DoneSignal, []string{"provider transcript replay passes"})
	}
}

func TestNormalizeCandidatesUsesAgentQueueEligibility(t *testing.T) {
	path := writeProgressJSON(t, `{
		"phases": {
			"3": {
				"subphases": {
					"3.E.6": {
						"items": [
							{"item_name": "p0 contract row", "status": "planned", "priority": "P0", "contract": "p0 handoff", "contract_status": "draft", "slice_size": "small"},
							{"item_name": "active contract row", "status": "in_progress", "contract": "active handoff", "contract_status": "draft", "slice_size": "small"},
							{"item_name": "draft contract row", "status": "planned", "contract": "draft handoff", "contract_status": "draft", "slice_size": "small"},
							{"item_name": "missing contract row", "status": "planned", "priority": "P0", "contract_status": "draft", "slice_size": "small"},
							{"item_name": "generic contract row", "status": "planned", "contract": "generic handoff", "slice_size": "small"},
							{"item_name": "blocked contract row", "status": "planned", "priority": "P0", "contract": "blocked handoff", "contract_status": "draft", "slice_size": "small", "blocked_by": ["dependency"]},
							{"item_name": "umbrella contract row", "status": "planned", "priority": "P0", "contract": "umbrella handoff", "contract_status": "draft", "slice_size": "umbrella"},
							{"item_name": "complete contract row", "status": "complete", "priority": "P0", "contract": "complete handoff", "contract_status": "draft", "slice_size": "small"}
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

	wantNames := []string{"p0 contract row", "active contract row", "draft contract row"}
	if gotNames := candidateNames(got); !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("candidate names = %#v, want %#v", gotNames, wantNames)
	}
}

func TestNormalizeCandidatesUsesExecutionBuckets(t *testing.T) {
	path := writeProgressJSON(t, `{
		"phases": {
				"3": {
					"subphases": {
					"3.E.6": {
							"items": [
								{"item_name": "draft candidate", "status": "planned", "contract": "draft contract", "contract_status": "draft", "fixture": "draft.json"},
								{"item_name": "fixture candidate", "status": "planned", "contract": "fixture contract", "contract_status": "fixture_ready", "fixture": "ready.json"},
								{"item_name": "active candidate", "status": "in_progress", "contract": "active contract"},
								{"item_name": "p0 candidate", "status": "planned", "priority": "P0", "contract": "p0 contract", "contract_status": "draft"},
								{"item_name": "unblocking candidate", "status": "planned", "contract": "unblocking contract", "unblocks": ["task 4"]}
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

	wantNames := []string{"p0 candidate", "active candidate", "fixture candidate", "unblocking candidate", "draft candidate"}
	if gotNames := candidateNames(got); !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("candidate order = %#v, want %#v", gotNames, wantNames)
	}
	if got[4].SelectionReason() != "draft contract" {
		t.Fatalf("draft candidate SelectionReason() = %q, want %q", got[4].SelectionReason(), "draft contract")
	}
}

func TestNormalizeCandidatesActiveFirstFalseSortsByKeyAfterPriorityBoost(t *testing.T) {
	path := writeProgressJSON(t, `{
		"phases": {
				"3": {
					"subphases": {
					"3.E.6": {
							"items": [
								{"item_name": "a draft candidate", "status": "planned", "contract": "draft contract", "contract_status": "draft"},
								{"item_name": "z p0 candidate", "status": "planned", "priority": "P0", "contract": "p0 contract", "contract_status": "draft"}
							]
						},
					"3.E.7": {
							"items": [
								{"item_name": "boosted planned candidate", "status": "planned", "contract": "boosted contract", "contract_status": "draft"}
							]
						}
					}
				}
			}
		}`)

	got, err := NormalizeCandidates(path, CandidateOptions{
		ActiveFirst:   false,
		PriorityBoost: []string{"3.E.7"},
	})
	if err != nil {
		t.Fatalf("NormalizeCandidates() error = %v", err)
	}

	wantNames := []string{"boosted planned candidate", "a draft candidate", "z p0 candidate"}
	if gotNames := candidateNames(got); !reflect.DeepEqual(gotNames, wantNames) {
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
								{"name": "phase 3 candidate", "status": "planned", "contract": "phase 3 contract", "contract_status": "draft"}
							]
						}
					}
				},
				"4": {
					"subphases": {
					"4.A": {
							"items": [
								{"name": "phase 4 candidate", "status": "in_progress", "contract": "phase 4 contract"}
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

	wantNames := []string{"phase 3 candidate"}
	if gotNames := candidateNames(got); !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("candidate names = %#v, want %#v", gotNames, wantNames)
	}
}

func TestNormalizeCandidatesUsesSubPhasesFallback(t *testing.T) {
	path := writeProgressJSON(t, `{
		"phases": {
				"4": {
					"sub_phases": {
					"4.A.1": {
							"items": [
								{"item_name": "fallback candidate", "status": "planned", "contract": "fallback contract", "contract_status": "draft"}
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

	wantNames := []string{"fallback candidate"}
	if gotNames := candidateNames(got); !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("candidate names = %#v, want %#v", gotNames, wantNames)
	}
}

func TestNormalizeCandidatesPrefersSubphasesWhenBothKeysExist(t *testing.T) {
	path := writeProgressJSON(t, `{
		"phases": {
				"4": {
					"subphases": {
					"4.A.1": {
							"items": [
								{"item_name": "preferred candidate", "status": "planned", "contract": "preferred contract", "contract_status": "draft"}
							]
						}
					},
					"sub_phases": {
						"4.A.2": {
							"items": [
								{"item_name": "fallback candidate", "status": "planned", "contract": "fallback contract", "contract_status": "draft"}
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

	wantNames := []string{"preferred candidate"}
	if gotNames := candidateNames(got); !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("candidate names = %#v, want %#v", gotNames, wantNames)
	}
}

func TestNormalizeCandidatesItemNameFallbacksAndUnknownStatus(t *testing.T) {
	path := writeProgressJSON(t, `{
		"phases": {
				"5": {
					"subphases": {
					"5.B.1": {
							"items": [
								{"item_name": "item-name candidate", "name": "ignored name", "status": "planned", "contract": "item-name contract", "contract_status": "draft"},
								{"item_name": " ", "name": "name candidate", "title": "ignored title", "status": "planned", "contract": "name contract", "contract_status": "draft"},
								{"name": " ", "title": "title candidate", "id": "ignored id", "status": "planned", "contract": "title contract", "contract_status": "draft"},
								{"title": " ", "id": "id candidate", "contract": "id contract", "contract_status": "draft"},
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

	wantNames := []string{"id candidate", "item-name candidate", "name candidate", "title candidate"}
	if gotNames := candidateNames(got); !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("candidate names = %#v, want %#v", gotNames, wantNames)
	}
	if got[0].Status != "unknown" {
		t.Fatalf("first candidate Status = %q, want unknown", got[0].Status)
	}
}

func TestNormalizeCandidatesDeduplicatesByPhaseSubphaseAndItemName(t *testing.T) {
	path := writeProgressJSON(t, `{
		"phases": {
				"6": {
					"subphases": {
					"6.C.1": {
							"items": [
								{"item_name": "duplicate candidate", "status": "planned", "contract": "planned contract", "contract_status": "draft"},
								{"item_name": "duplicate candidate", "status": "in_progress", "contract": "active contract"}
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

	wantNames := []string{"duplicate candidate"}
	if gotNames := candidateNames(got); !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("candidate names = %#v, want %#v", gotNames, wantNames)
	}
	if got[0].Status != "planned" {
		t.Fatalf("deduplicated candidate Status = %q, want planned", got[0].Status)
	}
}

func candidateNames(candidates []Candidate) []string {
	var names []string
	for _, candidate := range candidates {
		names = append(names, candidate.ItemName)
	}
	return names
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
