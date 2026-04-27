package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	sessionSearchDefaultLimit = 3
	sessionSearchMaxLimit     = 5
)

// SessionSearchTool exposes the model-facing session_search descriptor. This
// slice intentionally validates arguments only; execution is wired separately.
type SessionSearchTool struct{}

// SessionSearchArgs is the normalized argument shape for session_search.
type SessionSearchArgs struct {
	Query            string   `json:"query,omitempty"`
	Scope            string   `json:"scope"`
	Sources          []string `json:"sources,omitempty"`
	Mode             string   `json:"mode"`
	Limit            int      `json:"limit"`
	CurrentSessionID string   `json:"current_session_id,omitempty"`
}

// SessionSearchEvidence is degraded-mode evidence returned for invalid input.
type SessionSearchEvidence struct {
	Status string `json:"status"`
	Field  string `json:"field,omitempty"`
	Reason string `json:"reason"`
}

type sessionSearchResult struct {
	Success  bool                    `json:"success"`
	Args     *SessionSearchArgs      `json:"args,omitempty"`
	Evidence []SessionSearchEvidence `json:"evidence,omitempty"`
}

func (*SessionSearchTool) Name() string { return "session_search" }

func (*SessionSearchTool) Description() string {
	return "Search prior session transcripts or browse recent sessions using explicit same-chat or user-scoped recall controls."
}

func (*SessionSearchTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","description":"Optional keyword, phrase, or boolean search query. Omit to browse recent sessions."},"scope":{"type":"string","enum":["same-chat","user"],"default":"same-chat","description":"Recall scope. same-chat is the safe default; user may widen only when later execution can prove the current session binding."},"sources":{"type":"array","items":{"type":"string"},"description":"Optional source allowlist such as discord, telegram, slack, or matrix."},"mode":{"type":"string","enum":["default","recent","search"],"default":"default","description":"default chooses recent or search behavior from query presence; recent browses sessions; search runs keyword recall."},"limit":{"type":"integer","default":3,"minimum":0,"maximum":5,"description":"Maximum sessions to return. Defaults to 3 and is capped at 5."},"current_session_id":{"type":"string","description":"Current chat/session identifier used by later execution to prove same-chat or user-scope boundaries."}},"required":[]}`)
}

func (*SessionSearchTool) Timeout() time.Duration { return 5 * time.Second }

func (*SessionSearchTool) Execute(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
	normalized, evidence := ValidateSessionSearchArgs(args)
	if evidence != nil {
		return json.Marshal(sessionSearchResult{
			Success:  false,
			Evidence: []SessionSearchEvidence{*evidence},
		})
	}

	return json.Marshal(sessionSearchResult{
		Success: false,
		Args:    &normalized,
		Evidence: []SessionSearchEvidence{{
			Status: "session_search_unavailable",
			Reason: "session_search execution wrapper is not registered in this descriptor-only slice",
		}},
	})
}

// ValidateSessionSearchArgs normalizes safe defaults and rejects arguments that
// would make a later execution wrapper widen recall without explicit evidence.
func ValidateSessionSearchArgs(raw json.RawMessage) (SessionSearchArgs, *SessionSearchEvidence) {
	args := SessionSearchArgs{
		Scope:   "same-chat",
		Sources: []string{},
		Mode:    "default",
		Limit:   sessionSearchDefaultLimit,
	}

	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return args, nil
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &fields); err != nil {
		return args, invalidSessionSearchArgs("args", "arguments must be a JSON object")
	}
	if fields == nil {
		return args, invalidSessionSearchArgs("args", "arguments must be a JSON object")
	}

	if value, ok := fields["query"]; ok {
		query, err := sessionSearchString(value)
		if err != nil {
			return args, invalidSessionSearchArgs("query", err.Error())
		}
		args.Query = strings.TrimSpace(query)
	}

	if value, ok := fields["scope"]; ok {
		scope, err := sessionSearchString(value)
		if err != nil {
			return args, invalidSessionSearchArgs("scope", err.Error())
		}
		args.Scope = normalizeSessionSearchLabel(scope)
		if args.Scope == "" {
			args.Scope = "same-chat"
		}
		switch args.Scope {
		case "same-chat", "user":
		default:
			return args, invalidSessionSearchArgs("scope", fmt.Sprintf("unsupported scope %q; supported scopes are same-chat and user", scope))
		}
	}

	if value, ok := fields["sources"]; ok {
		var sources []string
		if err := json.Unmarshal(value, &sources); err != nil {
			return args, invalidSessionSearchArgs("sources", "sources must be an array of strings")
		}
		for _, source := range sources {
			source = normalizeSessionSearchLabel(source)
			if source == "" {
				return args, invalidSessionSearchArgs("sources", "sources must not contain empty values")
			}
			args.Sources = append(args.Sources, source)
		}
	}

	if value, ok := fields["mode"]; ok {
		mode, err := sessionSearchString(value)
		if err != nil {
			return args, invalidSessionSearchArgs("mode", err.Error())
		}
		args.Mode = normalizeSessionSearchLabel(mode)
		if args.Mode == "" {
			args.Mode = "default"
		}
		switch args.Mode {
		case "default", "recent", "search":
		default:
			return args, invalidSessionSearchArgs("mode", fmt.Sprintf("unsupported mode %q; supported modes are default, recent, and search", mode))
		}
	}

	if value, ok := fields["limit"]; ok {
		var limit int
		if err := json.Unmarshal(value, &limit); err != nil {
			return args, invalidSessionSearchArgs("limit", "limit must be an integer")
		}
		switch {
		case limit < 0:
			return args, invalidSessionSearchArgs("limit", "limit must be non-negative")
		case limit == 0:
			args.Limit = sessionSearchDefaultLimit
		case limit > sessionSearchMaxLimit:
			return args, invalidSessionSearchArgs("limit", fmt.Sprintf("limit must be <= %d", sessionSearchMaxLimit))
		default:
			args.Limit = limit
		}
	}

	if value, ok := fields["current_session_id"]; ok {
		currentSessionID, err := sessionSearchString(value)
		if err != nil {
			return args, invalidSessionSearchArgs("current_session_id", err.Error())
		}
		args.CurrentSessionID = strings.TrimSpace(currentSessionID)
	}

	return args, nil
}

func sessionSearchString(raw json.RawMessage) (string, error) {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("value must be a string")
	}
	return value, nil
}

func normalizeSessionSearchLabel(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func invalidSessionSearchArgs(field, reason string) *SessionSearchEvidence {
	return &SessionSearchEvidence{
		Status: "session_search_invalid_args",
		Field:  field,
		Reason: reason,
	}
}
