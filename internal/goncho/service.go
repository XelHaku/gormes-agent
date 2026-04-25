package goncho

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/TrebuchetDynamics/gormes-agent/internal/memory"
)

// Service is the first in-binary Goncho domain facade. It sits directly on
// top of the SQLite store used by Gormes today.
type Service struct {
	db          *sql.DB
	workspaceID string
	observer    string
	recentLimit int
	sessions    SessionDirectory
	log         *slog.Logger
}

// NewService constructs a Goncho service with conservative defaults.
func NewService(db *sql.DB, cfg Config, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	workspaceID := strings.TrimSpace(cfg.WorkspaceID)
	if workspaceID == "" {
		workspaceID = "default"
	}
	observer := strings.TrimSpace(cfg.ObserverPeerID)
	if observer == "" {
		observer = "gormes"
	}
	recentLimit := cfg.RecentMessages
	if recentLimit <= 0 {
		recentLimit = 4
	}
	return &Service{
		db:          db,
		workspaceID: workspaceID,
		observer:    observer,
		recentLimit: recentLimit,
		sessions:    cfg.SessionDirectory,
		log:         log,
	}
}

func (s *Service) SetProfile(ctx context.Context, peer string, card []string) error {
	peer = strings.TrimSpace(peer)
	if peer == "" {
		return fmt.Errorf("goncho: peer is required")
	}
	return upsertPeerCard(ctx, s.db, s.workspaceID, peer, card)
}

func (s *Service) Profile(ctx context.Context, peer string) (ProfileResult, error) {
	peer = strings.TrimSpace(peer)
	if peer == "" {
		return ProfileResult{}, fmt.Errorf("goncho: peer is required")
	}
	card, err := getPeerCard(ctx, s.db, s.workspaceID, peer)
	if err != nil {
		return ProfileResult{}, err
	}
	return ProfileResult{
		WorkspaceID: s.workspaceID,
		Peer:        peer,
		Card:        card,
	}, nil
}

func (s *Service) Conclude(ctx context.Context, params ConcludeParams) (ConcludeResult, error) {
	peer := strings.TrimSpace(params.Peer)
	if peer == "" {
		return ConcludeResult{}, fmt.Errorf("goncho: peer is required")
	}
	if params.DeleteID > 0 {
		deleted, err := deleteConclusion(ctx, s.db, s.workspaceID, s.observer, peer, params.DeleteID)
		if err != nil {
			return ConcludeResult{}, err
		}
		if !deleted {
			return ConcludeResult{}, fmt.Errorf("goncho: conclusion %d not found", params.DeleteID)
		}
		return ConcludeResult{
			WorkspaceID: s.workspaceID,
			Peer:        peer,
			ID:          params.DeleteID,
			Status:      "processed",
			Deleted:     true,
		}, nil
	}

	conclusion := strings.TrimSpace(params.Conclusion)
	if conclusion == "" {
		return ConcludeResult{}, fmt.Errorf("goncho: conclusion is required when delete_id is absent")
	}

	idempotencyKey := makeIdempotencyKey(s.workspaceID, s.observer, peer, params.SessionKey, conclusion)
	id, status, err := upsertConclusion(ctx, s.db, conclusionRow{
		WorkspaceID:    s.workspaceID,
		ObserverPeerID: s.observer,
		PeerID:         peer,
		SessionKey:     params.SessionKey,
		Content:        conclusion,
		Kind:           "manual",
		Status:         "processed",
		Source:         "manual",
		IdempotencyKey: idempotencyKey,
		EvidenceJSON:   "[]",
	})
	if err != nil {
		return ConcludeResult{}, err
	}

	return ConcludeResult{
		WorkspaceID: s.workspaceID,
		Peer:        peer,
		ID:          id,
		Status:      status,
	}, nil
}

func (s *Service) Search(ctx context.Context, params SearchParams) (SearchResultSet, error) {
	peer := strings.TrimSpace(params.Peer)
	if peer == "" {
		return SearchResultSet{}, fmt.Errorf("goncho: peer is required")
	}
	results, err := findConclusions(ctx, s.db, s.workspaceID, s.observer, peer, params.Query, params.SessionKey, 12)
	if err != nil {
		return SearchResultSet{}, err
	}
	if len(results) == 0 && strings.TrimSpace(params.Query) != "" {
		results, err = findConclusions(ctx, s.db, s.workspaceID, s.observer, peer, "", params.SessionKey, 12)
		if err != nil {
			return SearchResultSet{}, err
		}
	}

	if len(results) == 0 {
		fallback, err := s.searchTurnFallback(ctx, params)
		if err != nil {
			return SearchResultSet{}, err
		}
		results = fallback
	}
	results = limitHitsByTokens(results, params.MaxTokens)

	return SearchResultSet{
		WorkspaceID: s.workspaceID,
		Peer:        peer,
		Query:       params.Query,
		Results:     results,
	}, nil
}

func (s *Service) Context(ctx context.Context, params ContextParams) (ContextResult, error) {
	peer := strings.TrimSpace(params.Peer)
	if peer == "" {
		return ContextResult{}, fmt.Errorf("goncho: peer is required")
	}

	card, err := getPeerCard(ctx, s.db, s.workspaceID, peer)
	if err != nil {
		return ContextResult{}, err
	}

	searchResult, err := s.Search(ctx, SearchParams{
		Peer:       peer,
		Query:      params.Query,
		MaxTokens:  params.MaxTokens,
		SessionKey: params.SessionKey,
		Scope:      params.Scope,
		Sources:    params.Sources,
	})
	if err != nil {
		return ContextResult{}, err
	}

	conclusions := make([]string, 0, len(searchResult.Results))
	for _, hit := range searchResult.Results {
		if hit.Source != "conclusion" {
			continue
		}
		conclusions = append(conclusions, hit.Content)
	}

	recentMessages := []MessageSlice{}
	if strings.TrimSpace(params.SessionKey) != "" {
		recentMessages, err = recentTurns(ctx, s.db, params.SessionKey, s.recentLimit)
		if err != nil {
			return ContextResult{}, err
		}
	}

	return ContextResult{
		WorkspaceID:    s.workspaceID,
		Peer:           peer,
		SessionKey:     strings.TrimSpace(params.SessionKey),
		PeerCard:       card,
		Representation: buildRepresentation(peer, card, conclusions),
		Summary:        "",
		Conclusions:    conclusions,
		RecentMessages: recentMessages,
	}, nil
}

func buildRepresentation(peer string, card, conclusions []string) string {
	if len(card) == 0 && len(conclusions) == 0 {
		return "No stored representation for " + peer + "."
	}

	var b strings.Builder
	if len(card) > 0 {
		b.WriteString("Profile facts:")
		for _, item := range card {
			b.WriteString("\n- ")
			b.WriteString(item)
		}
	}
	if len(conclusions) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("Current conclusions:")
		for _, item := range conclusions {
			b.WriteString("\n- ")
			b.WriteString(item)
		}
	}
	return b.String()
}

func makeIdempotencyKey(workspaceID, observer, peer, sessionKey, conclusion string) string {
	normalized := strings.ToLower(strings.TrimSpace(conclusion))
	sum := sha256.Sum256([]byte(strings.Join([]string{
		workspaceID,
		observer,
		peer,
		strings.TrimSpace(sessionKey),
		normalized,
	}, "\x1f")))
	return hex.EncodeToString(sum[:])
}

func (s *Service) searchTurnFallback(ctx context.Context, params SearchParams) ([]SearchHit, error) {
	if strings.EqualFold(strings.TrimSpace(params.Scope), "user") && s.sessions != nil {
		userID := strings.TrimSpace(params.Peer)
		metas, err := s.sessions.ListMetadataByUserID(ctx, userID)
		if err != nil {
			return nil, err
		}
		hits, err := memory.SearchMessages(ctx, s.db, metas, memory.SearchFilter{
			UserID:           userID,
			Sources:          params.Sources,
			Query:            params.Query,
			CurrentSessionID: params.SessionKey,
			CurrentChatKey:   params.SessionKey,
		}, 6)
		if errors.Is(err, memory.ErrUserScopeDenied) {
			return findTurns(ctx, s.db, params.Query, params.SessionKey, 6)
		}
		if err != nil {
			return nil, err
		}
		out := make([]SearchHit, 0, len(hits))
		for _, hit := range hits {
			out = append(out, SearchHit{
				Source:       "turn",
				OriginSource: hit.Source,
				Content:      hit.Content,
				SessionKey:   hit.SessionID,
			})
		}
		return out, nil
	}

	if strings.TrimSpace(params.SessionKey) == "" {
		return nil, nil
	}
	return findTurns(ctx, s.db, params.Query, params.SessionKey, 6)
}

func limitHitsByTokens(hits []SearchHit, maxTokens int) []SearchHit {
	if maxTokens <= 0 || len(hits) == 0 {
		return hits
	}

	used := 0
	out := make([]SearchHit, 0, len(hits))
	for _, hit := range hits {
		cost := approxTokens(hit.Content)
		if used+cost > maxTokens && len(out) > 0 {
			break
		}
		out = append(out, hit)
		used += cost
	}
	return out
}

func approxTokens(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 1
	}
	if n := len(strings.Fields(text)); n > 0 {
		return n
	}
	return 1
}
