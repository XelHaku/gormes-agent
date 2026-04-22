// gormes/internal/subagent/blocked.go
package subagent

import "strings"

const (
	// MaxDepth bounds the subagent depth tree. Parent depth=0; a Spawn at
	// depth >= MaxDepth returns ErrMaxDepth. Default policy: parent → child OK,
	// grandchild rejected.
	MaxDepth = 2

	// DefaultMaxConcurrent is SpawnBatch's default semaphore size when the
	// caller passes maxConcurrent <= 0.
	DefaultMaxConcurrent = 3

	// DefaultMaxIterations is the per-subagent iteration budget applied at
	// Spawn time when SubagentConfig.MaxIterations <= 0. The StubRunner
	// ignores this; LLMRunner (2.E.7) will honour it.
	DefaultMaxIterations = 50
)

// BlockedTools is the forward-looking list of tool names that subagents
// must not be allowed to invoke. Of these names, only delegate_task exists
// in the current Gormes tool surface; the others are placeholders for
// tools that will be added in later phases.
var BlockedTools = map[string]bool{
	"delegate_task": true,
	"clarify":       true,
	"memory":        true,
	"send_message":  true,
	"execute_code":  true,
}

func blockedToolRequest(enabled []string) string {
	for _, name := range enabled {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if BlockedTools[name] {
			return name
		}
	}
	return ""
}

func toolAllowlisted(enabled []string, name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	if len(enabled) == 0 {
		return !BlockedTools[name]
	}
	for _, allowed := range enabled {
		if strings.TrimSpace(allowed) == name {
			return !BlockedTools[name]
		}
	}
	return false
}
