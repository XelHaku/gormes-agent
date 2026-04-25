package gateway

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/session"
)

const defaultExpiryFinalizeAttempts = 3

// SessionExpiryConfig wires the gateway's expiry finalization sweep. The
// scanner decides which sessions are expired; the manager owns finalization,
// retry evidence, and persisted state.
type SessionExpiryConfig struct {
	Scanner             SessionExpiryScanner
	Finalizer           SessionExpiryFinalizer
	MaxFinalizeAttempts int
}

// SessionExpiryScanner is the fakeable boundary for expiry policy. Production
// scanners can use real reset rules; tests can return temp metadata directly.
type SessionExpiryScanner interface {
	ExpiredSessions(context.Context, time.Time) ([]SessionExpiryCandidate, error)
}

// SessionExpiryCandidate carries the metadata needed to finalize one expired
// session without depending on live channel transports.
type SessionExpiryCandidate struct {
	SessionKey string
	Metadata   session.Metadata
}

// SessionExpiryFinalizer owns the side effects that happen once for a
// successfully expired session: the plugin-style finalize hook and cached-agent
// resource cleanup.
type SessionExpiryFinalizer interface {
	FinalizeExpiredSession(context.Context, SessionExpiryEvent) error
	CleanupCachedAgent(context.Context, SessionExpiryEvent) error
}

// SessionExpiryEvent is the stable payload passed to finalization hooks.
type SessionExpiryEvent struct {
	SessionKey  string
	SessionID   string
	Platform    string
	ChatID      string
	UserID      string
	Attempt     int
	MaxAttempts int
	At          time.Time
}

func (m *Manager) FinalizeExpiredSessions(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	scanner := m.cfg.SessionExpiry.Scanner
	if scanner == nil {
		return nil
	}
	writer, ok := m.cfg.SessionMap.(sessionMetadataWriter)
	if !ok {
		return errors.New("gateway: session expiry finalization requires metadata writer")
	}

	now := m.now()
	candidates, err := scanner.ExpiredSessions(ctx, now)
	if err != nil {
		return err
	}
	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return err
		}
		meta := candidate.Metadata
		if meta.SessionID == "" || meta.ExpiryFinalized || meta.ExpiryFinalizeStatus == session.ExpiryFinalizeStatusGaveUp {
			continue
		}
		m.writeExpiryFinalizeEvidence(ctx, expiryFinalizeEvidence(candidate, session.ExpiryFinalizeStatusPending, meta.ExpiryFinalizeAttempts, "", now))

		attempt := meta.ExpiryFinalizeAttempts + 1
		ev := sessionExpiryEvent(candidate, attempt, m.expiryFinalizeMaxAttempts(), now)
		if err := m.finalizeExpiredSession(ctx, ev); err != nil {
			status := session.ExpiryFinalizeStatusFailed
			if attempt >= ev.MaxAttempts {
				status = session.ExpiryFinalizeStatusGaveUp
			}
			update := session.Metadata{
				SessionID:                    meta.SessionID,
				Source:                       meta.Source,
				ChatID:                       meta.ChatID,
				UserID:                       meta.UserID,
				ExpiryFinalized:              status == session.ExpiryFinalizeStatusGaveUp,
				ExpiryFinalizeStatus:         status,
				ExpiryFinalizeAttempts:       attempt,
				ExpiryFinalizeLastError:      err.Error(),
				ExpiryFinalizeLastEvidenceAt: now.Unix(),
				UpdatedAt:                    now.Unix(),
			}
			if writeErr := writer.PutMetadata(ctx, update); writeErr != nil {
				return fmt.Errorf("persist expiry finalization failure for %q: %w", meta.SessionID, writeErr)
			}
			m.writeExpiryFinalizeEvidence(ctx, expiryFinalizeEvidence(candidate, status, attempt, err.Error(), now))
			continue
		}

		update := session.Metadata{
			SessionID:                    meta.SessionID,
			Source:                       meta.Source,
			ChatID:                       meta.ChatID,
			UserID:                       meta.UserID,
			ExpiryFinalized:              true,
			ExpiryFinalizeStatus:         session.ExpiryFinalizeStatusFinalized,
			ExpiryFinalizeAttempts:       attempt,
			ExpiryFinalizeLastEvidenceAt: now.Unix(),
			UpdatedAt:                    now.Unix(),
		}
		if writeErr := writer.PutMetadata(ctx, update); writeErr != nil {
			return fmt.Errorf("persist expiry finalization for %q: %w", meta.SessionID, writeErr)
		}
		m.writeExpiryFinalizedEvidence(ctx, RuntimeExpiryFinalizedEvidence{
			SessionID:             meta.SessionID,
			Source:                meta.Source,
			ChatID:                meta.ChatID,
			UserID:                meta.UserID,
			ExpiryFinalized:       true,
			MigratedMemoryFlushed: meta.MigratedMemoryFlushed,
		})
		m.writeExpiryFinalizeEvidence(ctx, expiryFinalizeEvidence(candidate, session.ExpiryFinalizeStatusFinalized, attempt, "", now))
	}
	return nil
}

func (m *Manager) expiryFinalizeMaxAttempts() int {
	if m.cfg.SessionExpiry.MaxFinalizeAttempts > 0 {
		return m.cfg.SessionExpiry.MaxFinalizeAttempts
	}
	return defaultExpiryFinalizeAttempts
}

func (m *Manager) finalizeExpiredSession(ctx context.Context, ev SessionExpiryEvent) error {
	finalizer := m.cfg.SessionExpiry.Finalizer
	if finalizer == nil {
		return nil
	}
	if err := finalizer.FinalizeExpiredSession(ctx, ev); err != nil {
		return err
	}
	return finalizer.CleanupCachedAgent(ctx, ev)
}

func sessionExpiryEvent(candidate SessionExpiryCandidate, attempt, maxAttempts int, at time.Time) SessionExpiryEvent {
	meta := candidate.Metadata
	return SessionExpiryEvent{
		SessionKey:  candidate.SessionKey,
		SessionID:   meta.SessionID,
		Platform:    meta.Source,
		ChatID:      meta.ChatID,
		UserID:      meta.UserID,
		Attempt:     attempt,
		MaxAttempts: maxAttempts,
		At:          at,
	}
}

func expiryFinalizeEvidence(candidate SessionExpiryCandidate, status session.ExpiryFinalizeStatus, attempts int, errText string, at time.Time) RuntimeExpiryFinalizeEvidence {
	meta := candidate.Metadata
	return RuntimeExpiryFinalizeEvidence{
		SessionKey: candidate.SessionKey,
		SessionID:  meta.SessionID,
		Source:     meta.Source,
		ChatID:     meta.ChatID,
		UserID:     meta.UserID,
		Status:     string(status),
		Attempts:   attempts,
		Error:      errText,
		At:         at.Format(time.RFC3339Nano),
	}
}
