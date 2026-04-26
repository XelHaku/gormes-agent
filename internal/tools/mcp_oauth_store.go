package tools

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// MCPOAuthState labels the operator-visible state of an MCP OAuth token slot
// without leaking secret material. Values are stable strings consumed by status
// surfaces and degraded-mode reporting.
type MCPOAuthState = string

const (
	MCPOAuthStateAbsent                 MCPOAuthState = "absent"
	MCPOAuthStateValid                  MCPOAuthState = "valid"
	MCPOAuthStateExpired                MCPOAuthState = "expired"
	MCPOAuthStateNoninteractiveRequired MCPOAuthState = "noninteractive_required"
)

// Evidence labels documented alongside each state. They appear in operator
// surfaces and structured logs; secret material is never used as evidence.
const (
	mcpOAuthEvidenceNoToken                = "no_token"
	mcpOAuthEvidenceOK                     = "ok"
	mcpOAuthEvidenceTokenExpired           = "token_expired"
	mcpOAuthEvidenceNoninteractiveRequired = "noninteractive_auth_unavailable"
)

// ErrMCPOAuthNoninteractiveRequired is returned by callers when the store is
// configured for non-interactive mode and a token is missing or otherwise
// unrecoverable without user interaction.
var ErrMCPOAuthNoninteractiveRequired = errors.New("mcp oauth: noninteractive auth unavailable")

// MCPOAuthToken is the in-memory credential record for a single MCP server.
// AccessToken and RefreshToken are secret material; the store boundary is
// responsible for keeping them out of any operator-visible output.
type MCPOAuthToken struct {
	AccessToken  string
	RefreshToken string
	Scope        string
	Issuer       string
	ExpiresAt    time.Time
}

// MCPOAuthStatus is the redacted, operator-visible read of one server's OAuth
// state. Server, State, and Evidence are safe to log and render; no secret
// material ever appears in this struct.
type MCPOAuthStatus struct {
	Server   string
	State    MCPOAuthState
	Evidence string
}

// String renders a stable single-line view safe for operator surfaces. It
// only references redacted fields so callers cannot accidentally log token
// material via fmt.Sprintf("%v", status).
func (s MCPOAuthStatus) String() string {
	parts := []string{
		"server=" + s.Server,
		"state=" + s.State,
	}
	if s.Evidence != "" {
		parts = append(parts, "evidence="+s.Evidence)
	}
	return "mcp_oauth " + strings.Join(parts, " ")
}

// MCPOAuthStore is a pure in-memory state store for MCP OAuth tokens. It does
// not persist to disk, contact OAuth issuers, or open transports; refresh and
// recovery are layered on top by other components.
type MCPOAuthStore struct {
	mu             sync.RWMutex
	tokens         map[string]MCPOAuthToken
	noninteractive bool
}

// NewMCPOAuthStore returns an empty store in interactive mode.
func NewMCPOAuthStore() *MCPOAuthStore {
	return &MCPOAuthStore{tokens: map[string]MCPOAuthToken{}}
}

// WithNoninteractive toggles the noninteractive policy and returns the store
// so call sites can chain configuration on construction.
func (s *MCPOAuthStore) WithNoninteractive(enabled bool) *MCPOAuthStore {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	s.noninteractive = enabled
	s.mu.Unlock()
	return s
}

// Get returns the stored token for server, if any.
func (s *MCPOAuthStore) Get(server string) (MCPOAuthToken, bool) {
	if s == nil {
		return MCPOAuthToken{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	tok, ok := s.tokens[server]
	return tok, ok
}

// Set stores tok under server. Server name must be non-empty so the store
// cannot accumulate anonymous slots.
func (s *MCPOAuthStore) Set(server string, tok MCPOAuthToken) error {
	if s == nil {
		return fmt.Errorf("mcp oauth: nil store")
	}
	if strings.TrimSpace(server) == "" {
		return fmt.Errorf("mcp oauth: server name required")
	}
	s.mu.Lock()
	s.tokens[server] = tok
	s.mu.Unlock()
	return nil
}

// Clear removes any stored token for server. Calling Clear on an unknown
// server is a no-op so callers can use it as an unconditional reset.
func (s *MCPOAuthStore) Clear(server string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	delete(s.tokens, server)
	s.mu.Unlock()
}

// StatusFor returns the redacted status of server at the given instant. The
// returned struct never embeds AccessToken or RefreshToken material.
func (s *MCPOAuthStore) StatusFor(server string, now time.Time) MCPOAuthStatus {
	status := MCPOAuthStatus{Server: server, State: MCPOAuthStateAbsent, Evidence: mcpOAuthEvidenceNoToken}
	if s == nil {
		return status
	}
	s.mu.RLock()
	tok, ok := s.tokens[server]
	noninteractive := s.noninteractive
	s.mu.RUnlock()

	if !ok {
		if noninteractive {
			status.State = MCPOAuthStateNoninteractiveRequired
			status.Evidence = mcpOAuthEvidenceNoninteractiveRequired
		}
		return status
	}
	if !tok.ExpiresAt.IsZero() && !now.Before(tok.ExpiresAt) {
		status.State = MCPOAuthStateExpired
		status.Evidence = mcpOAuthEvidenceTokenExpired
		return status
	}
	status.State = MCPOAuthStateValid
	status.Evidence = mcpOAuthEvidenceOK
	return status
}
