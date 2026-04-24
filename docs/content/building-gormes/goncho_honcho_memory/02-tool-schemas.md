---
title: "Tool Schemas"
weight: 2
---

# 02 — Tool Schemas (Verbatim from Upstream)

Every agent tool Honcho exposes, with its complete JSON `input_schema` copied verbatim from `src/utils/agent_tools.py`. Tools are alphabetical for lookup.

A Go port must expose the **same tool names, same descriptions, and same input schemas** so that LLMs trained on Honcho-shaped tool traces keep their calling patterns. Tool outputs are text/string; format specifics (e.g. `[id:xxx]` prefix markers) must also match because other tools downstream parse them.

Last sync: 2026-04-24 against Honcho `3.0.6`.

---

## 0. Tool-set constants

From `src/utils/agent_tools.py`:

| Constant | Used by | Contents |
|---|---|---|
| `DIALECTIC_TOOLS` | Dialectic agent, non-minimal levels | `search_memory`, `search_messages`, `get_observation_context`, `grep_messages`, `get_messages_by_date_range`, `search_messages_temporal`, `get_reasoning_chain` |
| `DIALECTIC_TOOLS_MINIMAL` | Dialectic agent, `minimal` level | `search_memory`, `search_messages` |
| `DREAMER_TOOLS` | Dreamer agent (omni / legacy scheduler path) | `extract_preferences`, `get_recent_observations`, `get_most_derived_observations`, `search_memory`, `get_peer_card`, `create_observations`, `delete_observations`, `update_peer_card`, `search_messages`, `get_observation_context`, `get_reasoning_chain`, `finish_consolidation` |
| `DEDUCTION_SPECIALIST_TOOLS` | Dreamer deduction specialist | `get_recent_observations`, `search_memory`, `search_messages`, `create_observations_deductive`, `delete_observations`, `update_peer_card` |
| `INDUCTION_SPECIALIST_TOOLS` | Dreamer induction specialist | `get_recent_observations`, `search_memory`, `search_messages`, `create_observations_inductive`, `update_peer_card` |

> **Drift note (2026-04-24):** `src/dialectic/prompts.py` still instructs the dialectic agent that it may call `create_observations_deductive`, but `src/utils/agent_tools.py::DIALECTIC_TOOLS` has that tool commented out. Goncho should preserve the current executable tool list unless the prompt/tool mismatch is intentionally fixed upstream.

### Tool executor factory

```python
async def create_tool_executor(
    workspace_name: str,
    observer: str,
    observed: str,
    session_name: str | None = None,
    current_messages: list[models.Message] | None = None,
    include_observation_ids: bool = False,          # True for dreamer paths that need ID back-refs
    history_token_limit: int = 8192,                # used by get_recent_history
    configuration: ResolvedConfiguration | None = None,
    run_id: str | None = None,
    agent_type: str | None = None,                  # "dialectic" | "deriver" | "dreamer"
    parent_category: str | None = None,             # CloudEvents parent category
) -> Callable[[str, dict[str, Any]], Any]: ...
```

Returns an async callable `execute_tool(tool_name, tool_input) -> str`. A `ToolContext` dataclass carries the parameters plus an `asyncio.Lock` shared per `(workspace, observer, observed)` tuple so concurrent observation writes serialise:

```python
@dataclass
class ToolContext:
    workspace_name: str
    observer: str
    observed: str
    session_name: str | None
    current_messages: list[models.Message] | None
    include_observation_ids: bool
    history_token_limit: int
    db_lock: asyncio.Lock          # serialises writes for the same observer/observed
    configuration: ResolvedConfiguration | None = None
    run_id: str | None = None
    agent_type: str | None = None
    parent_category: str | None = None
```

### Constants

```
MAX_PEER_CARD_FACTS = 40
```

---

## 1. `create_observations`  *(multi-level; used by dreamer path)*

**In:** DREAMER (omni)
**Implements:** `crud.create_documents()` via `_handle_create_observations → create_observations`

**Description:**
> Create observations at any level: explicit (facts), deductive (logical necessities), inductive (patterns), or contradiction (conflicting statements). For deductive, inductive, and contradiction observations, missing or empty source_ids are invalid and will be rejected.

```json
{
  "type": "object",
  "properties": {
    "observations": {
      "type": "array",
      "description": "List of observations to create",
      "items": {
        "type": "object",
        "properties": {
          "content": {"type": "string", "description": "The observation content"},
          "level": {
            "type": "string",
            "enum": ["explicit", "deductive", "inductive", "contradiction"],
            "description": "Level: 'explicit' for direct facts, 'deductive' for logical necessities, 'inductive' for patterns, 'contradiction' for conflicting statements"
          },
          "source_ids": {
            "type": "array",
            "items": {"type": "string"},
            "description": "Document IDs of source or premise observations. Required and must be non-empty for deductive, inductive, and contradiction observations."
          },
          "premises": {
            "type": "array",
            "items": {"type": "string"},
            "description": "(For deductive) Human-readable premise text for display"
          },
          "sources": {
            "type": "array",
            "items": {"type": "string"},
            "description": "(For inductive/contradiction) Human-readable source text for display"
          },
          "pattern_type": {
            "type": "string",
            "enum": ["preference", "behavior", "personality", "tendency", "correlation"],
            "description": "(For inductive only) Type of pattern being identified"
          },
          "confidence": {
            "type": "string",
            "enum": ["high", "medium", "low"],
            "description": "(For inductive only) Confidence level: 'high' for 5+ sources, 'medium' for 3-4, 'low' for 2"
          }
        },
        "required": ["content", "level"],
        "additionalProperties": false,
        "allOf": [
          {"if": {"properties": {"level": {"const": "deductive"}}},
           "then": {"required": ["source_ids", "premises"],
                    "properties": {
                      "source_ids": {"type": "array", "items": {"type": "string"}, "minItems": 1},
                      "premises": {"type": "array", "items": {"type": "string"}, "minItems": 1}}}},
          {"if": {"properties": {"level": {"const": "inductive"}}},
           "then": {"required": ["source_ids", "sources", "pattern_type", "confidence"],
                    "properties": {
                      "source_ids": {"type": "array", "items": {"type": "string"}, "minItems": 2},
                      "sources": {"type": "array", "items": {"type": "string"}, "minItems": 2}}}},
          {"if": {"properties": {"level": {"const": "contradiction"}}},
           "then": {"required": ["source_ids", "sources"],
                    "properties": {
                      "source_ids": {"type": "array", "items": {"type": "string"}, "minItems": 2},
                      "sources": {"type": "array", "items": {"type": "string"}, "minItems": 2}}}}
        ]
      }
    }
  },
  "required": ["observations"]
}
```

Validation rules per level (mirrored in `ObservationInput` at `src/schemas/internal.py`):
- `deductive` → `source_ids` and `premises` both non-empty.
- `inductive` → `source_ids` and `sources` each ≥ 2; `pattern_type`, `confidence` required.
- `contradiction` → `source_ids` and `sources` each ≥ 2.

---

## 2. `create_observations_deductive`  *(deduction specialist only)*

**In:** DEDUCTION_SPECIALIST

**Description:**
> Create new deductive observations discovered while answering the query. Every observation must include non-empty source_ids and premise text. Use this only for novel deductions grounded in existing observations.

```json
{
  "type": "object",
  "properties": {
    "observations": {
      "type": "array",
      "description": "List of new deductive observations to create",
      "items": {
        "type": "object",
        "properties": {
          "content": {"type": "string", "description": "The deductive conclusion as a self-contained statement"},
          "source_ids": {"type": "array", "items": {"type": "string"}, "minItems": 1,
                         "description": "Required non-empty list of source observation IDs supporting the deduction"},
          "premises": {"type": "array", "items": {"type": "string"}, "minItems": 1,
                       "description": "Required human-readable premise text matching the source observations"}
        },
        "required": ["content", "source_ids", "premises"],
        "additionalProperties": false
      }
    }
  },
  "required": ["observations"]
}
```

---

## 3. `create_observations_inductive`  *(induction specialist only)*

**In:** INDUCTION_SPECIALIST

**Description:**
> Create new inductive observations discovered while answering the query. Every observation must include source_ids, source text, pattern_type, and confidence. Use this only for patterns supported by multiple observations.

```json
{
  "type": "object",
  "properties": {
    "observations": {
      "type": "array",
      "description": "List of new inductive observations to create",
      "items": {
        "type": "object",
        "properties": {
          "content": {"type": "string", "description": "The inductive pattern or generalization as a self-contained statement"},
          "source_ids": {"type": "array", "items": {"type": "string"}, "minItems": 2,
                         "description": "Required list of at least two source observation IDs supporting the pattern"},
          "sources": {"type": "array", "items": {"type": "string"}, "minItems": 2,
                      "description": "Required human-readable evidence text matching the source observations"},
          "pattern_type": {"type": "string",
                           "enum": ["preference", "behavior", "personality", "tendency", "correlation"],
                           "description": "Required pattern category"},
          "confidence": {"type": "string", "enum": ["high", "medium", "low"],
                         "description": "Required confidence level based on evidence count"}
        },
        "required": ["content", "source_ids", "sources", "pattern_type", "confidence"],
        "additionalProperties": false
      }
    }
  },
  "required": ["observations"]
}
```

---

## 4. `delete_observations`

**In:** DREAMER (omni), DEDUCTION_SPECIALIST

**Description:**
> Delete observations by their IDs. Use the exact ID shown in [id:xxx] format from search results. Example: if observation shows '[id:abc123XYZ]', pass 'abc123XYZ' to delete it.

```json
{
  "type": "object",
  "properties": {
    "observation_ids": {
      "type": "array",
      "items": {"type": "string"},
      "description": "List of observation IDs to delete (use the exact ID from [id:xxx] in search results)"
    }
  },
  "required": ["observation_ids"]
}
```

---

## 5. `extract_preferences`

**In:** DREAMER (omni)

**Description:**
> Extract user preferences and standing instructions from conversation history. This tool performs both semantic and text searches for preferences, instructions, and communication style preferences, then returns them for adding to the peer card. Call this FIRST during consolidation.

```json
{"type": "object", "properties": {}}
```

Zero inputs. Handler runs a fixed set of internal searches and returns a text summary.

---

## 6. `finish_consolidation`

**In:** DREAMER (omni)

**Description:**
> Signal that consolidation is complete. Call this when you have finished your consolidation work and are ready to stop. You MUST call this tool when done - do not keep exploring indefinitely.

```json
{
  "type": "object",
  "properties": {
    "summary": {
      "type": "string",
      "description": "Brief summary of what was accomplished (peer card updates, observations consolidated, observations deleted)"
    }
  },
  "required": ["summary"]
}
```

This is the omni-dream specialist's exit signal. The deduction/induction specialists use `max_iterations` exhaustion plus natural tool-loop termination instead; there is no specialist equivalent of this tool on their tool sets today.

---

## 7. `get_messages_by_date_range`

**In:** DIALECTIC

**Description:**
> Get messages from a specific date range. Use this to find what was discussed during a particular time period, or to compare information before vs after a date. Essential for knowledge update questions.

```json
{
  "type": "object",
  "properties": {
    "after_date": {"type": "string", "description": "Start date (ISO format, e.g., '2024-01-15'). Returns messages after this date."},
    "before_date": {"type": "string", "description": "End date (ISO format). Returns messages before this date."},
    "limit": {"type": "integer", "default": 20, "description": "Maximum messages to return (default: 20, max: 50)"},
    "order": {"type": "string", "enum": ["asc", "desc"], "default": "desc",
              "description": "Sort order: 'asc' for oldest first, 'desc' for newest first (default: desc)"}
  }
}
```

---

## 8. `get_most_derived_observations`

**In:** DREAMER (omni)

**Description:**
> Get observations that have been reinforced most frequently across conversations. These represent the most established facts about the peer.

```json
{
  "type": "object",
  "properties": {
    "limit": {"type": "integer", "default": 10, "description": "Maximum number of observations to return (default: 10)"}
  }
}
```

Implementation reads `documents.times_derived DESC`.

---

## 9. `get_observation_context`

**In:** DIALECTIC, DREAMER (omni)

**Description:**
> Retrieve messages for given message IDs along with surrounding context. Takes message IDs (from an observation's message_ids field) and retrieves those messages plus the messages immediately before and after each one to provide conversation context.

```json
{
  "type": "object",
  "properties": {
    "message_ids": {
      "type": "array",
      "items": {"type": "string"},
      "description": "List of message IDs to retrieve (get these from observation.message_ids in search results)"
    }
  },
  "required": ["message_ids"]
}
```

---

## 10. `get_peer_card`

**In:** DREAMER (omni)

**Description:**
> Get the peer card containing known biographical information about the peer (name, age, location, etc.).

```json
{"type": "object", "properties": {}}
```

---

## 11. `get_recent_history`

**In:** (not in any list-level constant; wired conditionally into certain agents) — handler is in `_TOOL_HANDLERS`.

**Description:**
> Retrieve recent conversation history to get more context about the conversation.

```json
{"type": "object", "properties": {}}
```

Token budget = `ToolContext.history_token_limit` (default 8192).

---

## 12. `get_recent_observations`

**In:** DREAMER (omni), DEDUCTION_SPECIALIST, INDUCTION_SPECIALIST

**Description:**
> Get the most recent observations about the peer. Useful for understanding what's been learned recently.

```json
{
  "type": "object",
  "properties": {
    "limit": {"type": "integer", "default": 10, "description": "Maximum number of observations to return (default: 10)"},
    "session_only": {"type": "boolean", "default": false,
                     "description": "If true, only return observations from the current session (default: false)"}
  }
}
```

---

## 13. `get_reasoning_chain`

**In:** DIALECTIC, DREAMER (omni)

**Description:**
> Get the reasoning chain for an observation - traverse the tree to find premises (for deductive) or sources (for inductive), and/or find conclusions derived from this observation. Use this to understand how an observation was derived or what conclusions depend on it.

```json
{
  "type": "object",
  "properties": {
    "observation_id": {"type": "string", "description": "The document ID of the observation to get the reasoning chain for"},
    "direction": {"type": "string", "enum": ["premises", "conclusions", "both"], "default": "both",
                  "description": "'premises' to get what this observation is based on, 'conclusions' to get what depends on it, 'both' for full context"}
  },
  "required": ["observation_id"]
}
```

Implementation walks `documents.source_ids` (JSONB GIN-indexed) in either direction.

---

## 14. `get_session_summary`

**In:** DREAMER (omni)

**Description:**
> Get the session summary (short or long form). Useful for understanding the overall conversation context.

```json
{
  "type": "object",
  "properties": {
    "summary_type": {"type": "string", "enum": ["short", "long"], "default": "short",
                     "description": "Type of summary to retrieve (default: short)"}
  }
}
```

---

## 15. `grep_messages`

**In:** DIALECTIC

**Description:**
> Search for messages containing specific text (case-insensitive). Unlike semantic search, this finds EXACT text matches. Use for finding specific names, dates, phrases, or keywords mentioned in conversations. Returns messages with surrounding context.

```json
{
  "type": "object",
  "properties": {
    "text": {"type": "string", "description": "Text to search for (case-insensitive substring match)"},
    "limit": {"type": "integer", "default": 10, "description": "Maximum messages to return (default: 10, max: 30)"},
    "context_window": {"type": "integer", "default": 2, "description": "Number of messages before/after each match to include (default: 2)"}
  },
  "required": ["text"]
}
```

Backed by ILIKE on `messages.content` (escaped pattern). Goncho equivalent: FTS5 MATCH or LIKE depending on backend.

---

## 16. `search_memory`

**In:** DIALECTIC, DIALECTIC_MINIMAL, DREAMER (omni), DEDUCTION_SPECIALIST, INDUCTION_SPECIALIST

**Description:**
> Search for observations in memory using semantic similarity. Use this to find relevant information about the peer when you need to recall specific details.

```json
{
  "type": "object",
  "properties": {
    "query": {"type": "string", "description": "Search query text"},
    "top_k": {"type": "integer", "default": 20, "description": "(Optional) number of results to return (default: 20, max: 40)"}
  },
  "required": ["query"]
}
```

Backed by the vector-store query path (pgvector HNSW by default, Turbopuffer or LanceDB if configured).

---

## 17. `search_messages`

**In:** DIALECTIC, DIALECTIC_MINIMAL, DREAMER (omni), DEDUCTION_SPECIALIST, INDUCTION_SPECIALIST

**Description:**
> Search for messages using semantic similarity and retrieve conversation snippets. Returns matching messages with surrounding context (2 messages before and after). Nearby matches within the same session are merged into a single snippet to avoid repetition.

```json
{
  "type": "object",
  "properties": {
    "query": {"type": "string", "description": "Search query text to find relevant messages"},
    "limit": {"type": "integer", "default": 10, "description": "Maximum number of matching messages to return (default: 10, max: 20)"}
  },
  "required": ["query"]
}
```

Implementation merges adjacent snippets in the same session before returning.

---

## 18. `search_messages_temporal`

**In:** DIALECTIC

**Description:**
> Semantic search for messages with optional date filtering. Combines the power of semantic search with time constraints. Use after_date to find recent mentions of a topic, or before_date to find what was said about something before a certain point. Best for knowledge update questions where you need to find the MOST RECENT discussion of a topic.

```json
{
  "type": "object",
  "properties": {
    "query": {"type": "string", "description": "Semantic search query"},
    "after_date": {"type": "string", "description": "Only return messages after this date (ISO format, e.g., '2024-01-15')"},
    "before_date": {"type": "string", "description": "Only return messages before this date (ISO format)"},
    "limit": {"type": "integer", "default": 10, "description": "Maximum messages to return (default: 10, max: 20)"},
    "context_window": {"type": "integer", "default": 2, "description": "Messages before/after each match (default: 2)"}
  },
  "required": ["query"]
}
```

---

## 19. `update_peer_card`

**In:** DREAMER (omni), DEDUCTION_SPECIALIST, INDUCTION_SPECIALIST

**Description:**
> Update the peer card with durable profile facts about the observed peer. Only include stable biographical facts, standing instructions, and long-lived preferences/traits. Do not include one-off conclusions, temporary events, or duplicate entries.

```json
{
  "type": "object",
  "properties": {
    "content": {
      "type": "array",
      "description": "Complete deduplicated peer card list (max 40 entries). Each entry should be a concise standalone profile fact.",
      "items": {"type": "string"}
    }
  },
  "required": ["content"]
}
```

Handler truncates to `MAX_PEER_CARD_FACTS = 40` and persists via `crud.set_peer_card(observer, observed, content)` into `peers.internal_metadata[{observed}_peer_card]` (or `peer_card` when self).

---

## 20. Tool handler map (`_TOOL_HANDLERS`)

Reference for the full dispatch table inside `src/utils/agent_tools.py`:

```python
_TOOL_HANDLERS = {
    "create_observations":            _handle_create_observations,
    "create_observations_deductive":  _handle_create_observations_deductive,
    "create_observations_inductive":  _handle_create_observations_inductive,
    "update_peer_card":               _handle_update_peer_card,
    "get_recent_history":             _handle_get_recent_history,
    "search_memory":                  _handle_search_memory,
    "get_observation_context":        _handle_get_observation_context,
    "search_messages":                _handle_search_messages,
    "grep_messages":                  _handle_grep_messages,
    "get_messages_by_date_range":     _handle_get_messages_by_date_range,
    "search_messages_temporal":       _handle_search_messages_temporal,
    "get_recent_observations":        _handle_get_recent_observations,
    "get_most_derived_observations":  _handle_get_most_derived_observations,
    "get_session_summary":            _handle_get_session_summary,
    "get_peer_card":                  _handle_get_peer_card,
    "delete_observations":            _handle_delete_observations,
    "finish_consolidation":           _handle_finish_consolidation,
    "extract_preferences":            _handle_extract_preferences,
    "get_reasoning_chain":            _handle_get_reasoning_chain,
}
```

---

## 21. Coverage / TODO

- [ ] Copy each handler's **output-format contract** (e.g. "results rendered as `[id:xxx] [timestamp] content\\n`") — the LLM relies on those markers.
- [ ] Capture maximum-token-output truncation rules used in `_handle_*` functions when a tool returns lots of rows.
- [ ] Enumerate error-shape responses (what a tool returns when DB/vector-store calls fail — LLMs are trained on those error strings).
- [ ] Add `03-data-types.md` for Pydantic request/response and tool output shapes.
- [ ] Add a Go tool-schema fixture file (`internal/goncho/testdata/tool_schemas.json`) that this doc can diff against in CI.

---

**See also:** [`01-prompts.md`](../01-prompts) — prompts that call these tools.
