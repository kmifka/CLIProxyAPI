package auth

import (
	"testing"
	"time"
)

func TestLegacyTransportRecoveryRestrictions(t *testing.T) {
	now := time.Date(2026, 10, 13, 18, 9, 13, 0, time.UTC)
	for _, status := range []int{0, 401, 402, 403, 429} {
		a := &Auth{ID: "fixture", Provider: "codex", Status: StatusError, Unavailable: true, UpdatedAt: now, LastError: &Error{HTTPStatus: status}}
		if blocked, _, next := isAuthBlockedForModel(a, "", now.Add(time.Hour)); !blocked || !next.IsZero() {
			t.Fatalf("restriction %d changed", status)
		}
	}
	a := &Auth{ID: "fixture", Provider: "codex", Disabled: true, Unavailable: true, UpdatedAt: now, LastError: &Error{HTTPStatus: 500}}
	if blocked, _, _ := isAuthBlockedForModel(a, "", now.Add(time.Hour)); !blocked {
		t.Fatal("manual disable bypassed")
	}
	a.Disabled = false
	a.Quota = QuotaState{Exceeded: true}
	if blocked, _, _ := isAuthBlockedForModel(a, "", now.Add(time.Hour)); !blocked {
		t.Fatal("quota bypassed")
	}
}

func TestLegacyTransportRecoveryBounded(t *testing.T) {
	now := time.Date(2026, 10, 13, 18, 9, 13, 0, time.UTC)
	for _, model := range []string{"gpt-5.5", ""} {
		t.Run("model="+model, func(t *testing.T) {
			a := &Auth{ID: "fixture", Provider: "codex", Status: StatusError, Unavailable: true, UpdatedAt: now, LastError: &Error{HTTPStatus: 500, Message: "utls: TLS handshake: tls: failed to verify certificate: x509: certificate signed by unknown authority"}}
			if model != "" {
				a.ModelStates = map[string]*ModelState{model: {Status: StatusError, Unavailable: true, UpdatedAt: now, LastError: a.LastError}}
			}
			blocked, _, next := isAuthBlockedForModel(a, model, now)
			if !blocked || !next.After(now) || next.After(now.Add(time.Minute)) {
				t.Fatalf("initial legacy hold must be finite: blocked=%v next=%v", blocked, next)
			}
			if blocked, _, _ := isAuthBlockedForModel(a, model, now.Add(2*time.Minute)); blocked {
				t.Fatal("legacy transport remains blocked forever")
			}
			if !a.NextRetryAfter.IsZero() {
				t.Fatal("selection mutated persisted state")
			}
		})
	}
}
