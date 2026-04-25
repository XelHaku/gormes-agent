package gateway

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestPairingApproval_GeneratesHermesCompatibleCodesAndApprovesValidCode(t *testing.T) {
	store := newPairingApprovalTestStore(t)
	now := time.Date(2026, 4, 25, 21, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	issued := make([]PairingCodeResult, 0, maxPendingPairingCodesPerPlatform)
	for _, user := range []string{"ada", "grace", "linus"} {
		result, err := store.GeneratePairingCode(context.Background(), PairingCodeRequest{
			Platform: "telegram",
			UserID:   user,
			UserName: strings.ToUpper(user[:1]) + user[1:],
		})
		if err != nil {
			t.Fatalf("GeneratePairingCode(%s): %v", user, err)
		}
		if result.Status != PairingCodeIssued {
			t.Fatalf("GeneratePairingCode(%s) status = %q, want %q", user, result.Status, PairingCodeIssued)
		}
		assertHermesPairingCode(t, result.Code)
		if !result.ExpiresAt.Equal(now.Add(pairingCodeTTL)) {
			t.Fatalf("ExpiresAt = %s, want %s", result.ExpiresAt, now.Add(pairingCodeTTL))
		}
		issued = append(issued, result)
	}
	if issued[0].Code == issued[1].Code || issued[0].Code == issued[2].Code || issued[1].Code == issued[2].Code {
		t.Fatalf("generated codes are not unique enough for the fixture: %#v", []string{issued[0].Code, issued[1].Code, issued[2].Code})
	}

	status, err := store.ReadPairingStatus(context.Background())
	if err != nil {
		t.Fatalf("ReadPairingStatus: %v", err)
	}
	if got := platformKeys(status.Platforms); !reflect.DeepEqual(got, []string{"telegram/unpaired/3/0"}) {
		t.Fatalf("platforms before approval = %v", got)
	}

	approved, err := store.ApprovePairingCode(context.Background(), "telegram", "  "+strings.ToLower(issued[0].Code)+"  ")
	if err != nil {
		t.Fatalf("ApprovePairingCode: %v", err)
	}
	if approved.Status != PairingApprovalApproved {
		t.Fatalf("approval status = %q, want %q", approved.Status, PairingApprovalApproved)
	}
	if approved.UserID != "ada" || approved.UserName != "Ada" {
		t.Fatalf("approved identity = %#v, want ada/Ada", approved)
	}

	status, err = store.ReadPairingStatus(context.Background())
	if err != nil {
		t.Fatalf("ReadPairingStatus after approval: %v", err)
	}
	if got := pendingCodes(status.Pending); containsString(got, issued[0].Code) {
		t.Fatalf("approved code %q remained pending: %v", issued[0].Code, got)
	}
	if got := approvedKeys(status.Approved); !reflect.DeepEqual(got, []string{"telegram/ada"}) {
		t.Fatalf("approved records = %v", got)
	}
}

func TestPairingApproval_EnforcesPendingLimitAndExpiry(t *testing.T) {
	store := newPairingApprovalTestStore(t)
	now := time.Date(2026, 4, 25, 22, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	var firstCode string
	for i := 0; i < maxPendingPairingCodesPerPlatform; i++ {
		result, err := store.GeneratePairingCode(context.Background(), PairingCodeRequest{
			Platform: "telegram",
			UserID:   formatInt(i),
		})
		if err != nil {
			t.Fatalf("GeneratePairingCode(%d): %v", i, err)
		}
		if result.Status != PairingCodeIssued {
			t.Fatalf("GeneratePairingCode(%d) status = %q, want %q", i, result.Status, PairingCodeIssued)
		}
		if i == 0 {
			firstCode = result.Code
		}
	}

	blocked, err := store.GeneratePairingCode(context.Background(), PairingCodeRequest{
		Platform: "telegram",
		UserID:   "over-limit",
	})
	if err != nil {
		t.Fatalf("GeneratePairingCode(over-limit): %v", err)
	}
	if blocked.Status != PairingCodeMaxPending || blocked.Code != "" {
		t.Fatalf("over-limit result = %#v, want max-pending with no code", blocked)
	}
	assertPairingEvidence(t, store, PairingDegradedMaxPending)

	discord, err := store.GeneratePairingCode(context.Background(), PairingCodeRequest{
		Platform: "discord",
		UserID:   "discord-user",
	})
	if err != nil {
		t.Fatalf("GeneratePairingCode(discord): %v", err)
	}
	if discord.Status != PairingCodeIssued {
		t.Fatalf("discord status = %q, want %q", discord.Status, PairingCodeIssued)
	}

	now = now.Add(pairingCodeTTL + time.Second)
	expired, err := store.ApprovePairingCode(context.Background(), "telegram", firstCode)
	if err != nil {
		t.Fatalf("ApprovePairingCode(expired): %v", err)
	}
	if expired.Status != PairingApprovalExpired {
		t.Fatalf("expired approval status = %q, want %q", expired.Status, PairingApprovalExpired)
	}
	assertPairingEvidence(t, store, PairingDegradedExpired)

	status, err := store.ReadPairingStatus(context.Background())
	if err != nil {
		t.Fatalf("ReadPairingStatus: %v", err)
	}
	if got := pendingKeys(status.Pending); len(got) != 0 {
		t.Fatalf("pending after one-hour expiry = %v, want no active pending codes", got)
	}
}

func TestPairingApproval_EnforcesRateLimitAndFailedApprovalLockout(t *testing.T) {
	store := newPairingApprovalTestStore(t)
	now := time.Date(2026, 4, 25, 23, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	first, err := store.GeneratePairingCode(context.Background(), PairingCodeRequest{
		Platform: "telegram",
		UserID:   "ada",
	})
	if err != nil {
		t.Fatalf("GeneratePairingCode(first): %v", err)
	}
	if first.Status != PairingCodeIssued {
		t.Fatalf("first status = %q, want %q", first.Status, PairingCodeIssued)
	}

	second, err := store.GeneratePairingCode(context.Background(), PairingCodeRequest{
		Platform: "telegram",
		UserID:   "ada",
	})
	if err != nil {
		t.Fatalf("GeneratePairingCode(second): %v", err)
	}
	if second.Status != PairingCodeRateLimited || !second.RetryAt.Equal(now.Add(pairingRequestRateLimit)) {
		t.Fatalf("second result = %#v, want rate-limited until %s", second, now.Add(pairingRequestRateLimit))
	}
	assertPairingEvidence(t, store, PairingDegradedRateLimited)

	now = now.Add(pairingRequestRateLimit + time.Second)
	third, err := store.GeneratePairingCode(context.Background(), PairingCodeRequest{
		Platform: "telegram",
		UserID:   "ada",
	})
	if err != nil {
		t.Fatalf("GeneratePairingCode(after rate limit): %v", err)
	}
	if third.Status != PairingCodeIssued {
		t.Fatalf("third status = %q, want %q", third.Status, PairingCodeIssued)
	}

	for i := 0; i < maxPairingApprovalFailures; i++ {
		result, err := store.ApprovePairingCode(context.Background(), "slack", "WRONG")
		if err != nil {
			t.Fatalf("ApprovePairingCode invalid %d: %v", i, err)
		}
		if i < maxPairingApprovalFailures-1 && result.Status != PairingApprovalInvalid {
			t.Fatalf("invalid attempt %d status = %q, want %q", i, result.Status, PairingApprovalInvalid)
		}
		if i == maxPairingApprovalFailures-1 {
			if result.Status != PairingApprovalLockedOut || !result.LockedOutUntil.Equal(now.Add(pairingLockoutDuration)) {
				t.Fatalf("lockout attempt result = %#v, want locked out until %s", result, now.Add(pairingLockoutDuration))
			}
		}
	}

	locked, err := store.GeneratePairingCode(context.Background(), PairingCodeRequest{
		Platform: "slack",
		UserID:   "new-user",
	})
	if err != nil {
		t.Fatalf("GeneratePairingCode(locked): %v", err)
	}
	if locked.Status != PairingCodeLockedOut || !locked.RetryAt.Equal(now.Add(pairingLockoutDuration)) {
		t.Fatalf("locked result = %#v, want locked-out until %s", locked, now.Add(pairingLockoutDuration))
	}
	assertPairingEvidence(t, store, PairingDegradedLockedOut)

	now = now.Add(pairingLockoutDuration + time.Second)
	unlocked, err := store.GeneratePairingCode(context.Background(), PairingCodeRequest{
		Platform: "slack",
		UserID:   "new-user",
	})
	if err != nil {
		t.Fatalf("GeneratePairingCode(after lockout): %v", err)
	}
	if unlocked.Status != PairingCodeIssued {
		t.Fatalf("unlocked status = %q, want %q", unlocked.Status, PairingCodeIssued)
	}
}

func TestPairingApproval_ResolvesTelegramDMFallbackAndRecordsDeniedOrUnresolvedAttempts(t *testing.T) {
	store := newPairingApprovalTestStore(t)
	now := time.Date(2026, 4, 26, 0, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	dmRequest := PairingCodeRequestFromInbound(InboundEvent{
		Platform: "telegram",
		ChatID:   "424242",
		ChatType: "private",
		UserName: "Private Chat",
	}, false)
	if dmRequest.UserID != "424242" {
		t.Fatalf("DM fallback UserID = %q, want chat ID", dmRequest.UserID)
	}

	dmCode, err := store.GeneratePairingCode(context.Background(), dmRequest)
	if err != nil {
		t.Fatalf("GeneratePairingCode(DM fallback): %v", err)
	}
	if dmCode.Status != PairingCodeIssued {
		t.Fatalf("DM fallback status = %q, want %q", dmCode.Status, PairingCodeIssued)
	}

	groupRequest := PairingCodeRequestFromInbound(InboundEvent{
		Platform: "telegram",
		ChatID:   "-100",
		ChatType: "group",
	}, false)
	if groupRequest.UserID != "" {
		t.Fatalf("group fallback UserID = %q, want empty", groupRequest.UserID)
	}
	unresolved, err := store.GeneratePairingCode(context.Background(), groupRequest)
	if err != nil {
		t.Fatalf("GeneratePairingCode(unresolved): %v", err)
	}
	if unresolved.Status != PairingCodeUnresolvedUser || unresolved.Code != "" {
		t.Fatalf("unresolved result = %#v, want unresolved-user with no code", unresolved)
	}
	assertPairingEvidence(t, store, PairingDegradedUnresolvedUser)

	denied, err := store.GeneratePairingCode(context.Background(), PairingCodeRequest{
		Platform:        "telegram",
		UserID:          "allowlist-denied",
		AllowlistDenied: true,
	})
	if err != nil {
		t.Fatalf("GeneratePairingCode(allowlist denied): %v", err)
	}
	if denied.Status != PairingCodeAllowlistDenied || denied.Code != "" {
		t.Fatalf("allowlist denied result = %#v, want allowlist-denied with no code", denied)
	}
	assertPairingEvidence(t, store, PairingDegradedAllowlistDenied)

	for i := 0; i < maxPairingApprovalFailures; i++ {
		if _, err := store.ApprovePairingCode(context.Background(), "telegram", "DENIED"); err != nil {
			t.Fatalf("ApprovePairingCode denied invalid %d: %v", i, err)
		}
	}
	assertPairingEvidence(t, store, PairingDegradedLockedOut)
}

func newPairingApprovalTestStore(t *testing.T) *PairingStore {
	t.Helper()
	return NewPairingStore(filepath.Join(t.TempDir(), "pairing.json"))
}

func assertHermesPairingCode(t *testing.T, code string) {
	t.Helper()
	if len(code) != pairingCodeLength {
		t.Fatalf("len(%q) = %d, want %d", code, len(code), pairingCodeLength)
	}
	for _, c := range code {
		if !strings.ContainsRune(pairingCodeAlphabet, c) {
			t.Fatalf("code %q contains %q outside Hermes alphabet %q", code, c, pairingCodeAlphabet)
		}
	}
}

func assertPairingEvidence(t *testing.T, store *PairingStore, reason PairingDegradedReason) {
	t.Helper()
	status, err := store.ReadPairingStatus(context.Background())
	if err != nil {
		t.Fatalf("ReadPairingStatus: %v", err)
	}
	for _, evidence := range status.Degraded {
		if evidence.Reason == reason {
			return
		}
	}
	t.Fatalf("degraded evidence = %+v, want reason %q", status.Degraded, reason)
}

func pendingCodes(records []PairingPendingRecord) []string {
	codes := make([]string, 0, len(records))
	for _, record := range records {
		codes = append(codes, record.Code)
	}
	return codes
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
