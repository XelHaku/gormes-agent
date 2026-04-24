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
		{PhaseID: "5", SubphaseID: "5.B.1", ItemName: "id candidate", Status: "unknown"},
		{PhaseID: "5", SubphaseID: "5.B.1", ItemName: "item-name candidate", Status: "planned"},
		{PhaseID: "5", SubphaseID: "5.B.1", ItemName: "name candidate", Status: "planned"},
		{PhaseID: "5", SubphaseID: "5.B.1", ItemName: "title candidate", Status: "planned"},
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

func TestNormalizeCandidatesPreservesExecutionMetadataAndSkipsBlockedUmbrella(t *testing.T) {
	path := writeProgressJSON(t, `{
		"phases": {
			"7": {
				"subphases": {
					"7.A": {
						"items": [
							{
								"name": "ready candidate",
								"status": "planned",
								"priority": "P0",
								"contract": "Provider-neutral transcript contract",
								"contract_status": "fixture_ready",
								"slice_size": "medium",
								"execution_owner": "provider",
								"trust_class": ["system"],
								"degraded_mode": "provider status reports missing fixtures",
								"fixture": "internal/hermes/testdata/provider_transcripts",
								"source_refs": ["docs/content/upstream-hermes/source-study.md"],
								"ready_when": ["fixtures replay"],
								"not_ready_when": ["live provider call required"],
								"acceptance": ["go test ./internal/hermes passes"],
								"write_scope": ["internal/hermes/"],
								"test_commands": ["go test ./internal/hermes -count=1"],
								"done_signal": ["provider transcript replay passes"],
								"unblocks": ["Bedrock adapter"],
								"note": "Use captured transcript fixtures."
							},
							{
								"name": "blocked candidate",
								"status": "planned",
								"contract": "blocked",
								"slice_size": "small",
								"blocked_by": ["ready candidate"],
								"ready_when": ["ready candidate completes"]
							},
							{
								"name": "umbrella candidate",
								"status": "planned",
								"contract": "umbrella",
								"slice_size": "umbrella"
							}
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
	if gotLen := len(got); gotLen != 1 {
		t.Fatalf("NormalizeCandidates() length = %d, want 1: %#v", gotLen, got)
	}

	candidate := got[0]
	if candidate.ItemName != "ready candidate" {
		t.Fatalf("ItemName = %q, want ready candidate", candidate.ItemName)
	}
	for _, want := range []string{
		candidate.Priority,
		candidate.Contract,
		candidate.ContractStatus,
		candidate.SliceSize,
		candidate.ExecutionOwner,
		candidate.DegradedMode,
		candidate.Fixture,
		candidate.Note,
	} {
		if strings.TrimSpace(want) == "" {
			t.Fatalf("candidate lost scalar metadata: %#v", candidate)
		}
	}
	if !reflect.DeepEqual(candidate.TrustClass, []string{"system"}) {
		t.Fatalf("TrustClass = %#v, want system", candidate.TrustClass)
	}
	if !reflect.DeepEqual(candidate.WriteScope, []string{"internal/hermes/"}) {
		t.Fatalf("WriteScope = %#v, want internal/hermes/", candidate.WriteScope)
	}
	if !reflect.DeepEqual(candidate.TestCommands, []string{"go test ./internal/hermes -count=1"}) {
		t.Fatalf("TestCommands = %#v, want provider test", candidate.TestCommands)
	}
	if got, want := candidate.SelectionReason(), "P0 handoff"; got != want {
		t.Fatalf("SelectionReason() = %q, want %q", got, want)
	}
}

func TestNormalizeCandidatesUsesExecutionBuckets(t *testing.T) {
	path := writeProgressJSON(t, `{
		"phases": {
			"8": {
				"subphases": {
					"8.A": {
						"items": [
							{"name": "draft candidate", "status": "planned", "contract": "draft", "contract_status": "draft", "slice_size": "small"},
							{"name": "fixture candidate", "status": "planned", "contract": "fixture", "contract_status": "fixture_ready", "slice_size": "small"},
							{"name": "active candidate", "status": "in_progress", "contract": "active", "contract_status": "draft", "slice_size": "small"},
							{"name": "p0 candidate", "status": "planned", "priority": "P0", "contract": "p0", "contract_status": "draft", "slice_size": "small"},
							{"name": "unblocking candidate", "status": "planned", "contract": "unblock", "contract_status": "missing", "slice_size": "small", "unblocks": ["next row"]}
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

	var names []string
	for _, candidate := range got {
		names = append(names, candidate.ItemName)
	}
	want := []string{
		"p0 candidate",
		"active candidate",
		"fixture candidate",
		"unblocking candidate",
		"draft candidate",
	}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("candidate order = %#v, want %#v", names, want)
	}
}

func TestNormalizeCandidatesUsesSubphasePriorityAsTieBreak(t *testing.T) {
	path := writeProgressJSON(t, `{
		"phases": {
			"2": {
				"subphases": {
					"2.B.11": {
						"priority": "P3",
						"items": [
							{"name": "discord forum follow-up", "status": "planned"}
						]
					},
					"2.B.4": {
						"priority": "P1",
						"items": [
							{"name": "whatsapp runtime closeout", "status": "planned"}
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

	var names []string
	var priorities []string
	for _, candidate := range got {
		names = append(names, candidate.ItemName)
		priorities = append(priorities, candidate.Priority)
	}
	if want := []string{"whatsapp runtime closeout", "discord forum follow-up"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("candidate order = %#v, want %#v", names, want)
	}
	if want := []string{"P1", "P3"}; !reflect.DeepEqual(priorities, want) {
		t.Fatalf("candidate priorities = %#v, want %#v", priorities, want)
	}
}

func TestNormalizeCandidatesIncludesRowsWhenBlockersAreComplete(t *testing.T) {
	path := writeProgressJSON(t, `{
		"phases": {
			"2": {
				"subphases": {
					"2.B.3": {
						"items": [
							{"name": "parser wiring", "status": "complete"},
							{"name": "channel shim", "status": "planned", "blocked_by": ["parser wiring"]},
							{"name": "config registration", "status": "planned", "blocked_by": ["channel shim"]}
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

	if len(got) != 1 {
		t.Fatalf("NormalizeCandidates() length = %d, want 1: %#v", len(got), got)
	}
	if got[0].ItemName != "channel shim" {
		t.Fatalf("selected candidate = %q, want channel shim", got[0].ItemName)
	}
	if !reflect.DeepEqual(got[0].BlockedBy, []string{"parser wiring"}) {
		t.Fatalf("BlockedBy = %#v, want parser wiring", got[0].BlockedBy)
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
