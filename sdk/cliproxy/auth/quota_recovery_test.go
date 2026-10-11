package auth

import (
	"context"
	"testing"
	"time"
)

type verifiedQuotaReconciler interface {
	ReconcileVerifiedQuota(context.Context, *Auth, time.Time) (*Auth, []string, error)
}

func TestVerifiedQuotaRecoveryIsAtomicAndSelective(t *testing.T) {
	for _, scenario := range []string{"recovered", "newer_failure", "changed_credential", "stale", "disabled", "general429", "policy403", "manual", "registration", "newer_observation"} {
		t.Run(scenario, func(t *testing.T) {
			m := NewManager(nil, nil, nil)
			now := time.Now()
			failure := &Error{HTTPStatus: 429, Message: `{"error":{"type":"usage_limit_reached"}}`}
			if scenario == "general429" {
				failure = &Error{HTTPStatus: 429, Message: "rate limited"}
			}
			if scenario == "policy403" {
				failure = &Error{HTTPStatus: 403, Message: "content policy"}
			}
			if scenario == "manual" {
				failure = &Error{HTTPStatus: 429, Code: ErrorCodeForceCooldown, Message: `{"error":{"type":"usage_limit_reached"}}`}
			}
			a := &Auth{ID: "plus", Provider: "codex", Metadata: map[string]any{"access_token": "version1"}, Status: StatusError, LastError: failure, Unavailable: true, NextRetryAfter: now.Add(120 * time.Hour), Quota: QuotaState{Exceeded: true, Reason: "credential_quota", NextRecoverAt: now.Add(120 * time.Hour)}, ModelStates: map[string]*ModelState{
				"gpt-test": {Status: StatusError, LastError: failure, Unavailable: true, NextRetryAfter: now.Add(120 * time.Hour), Quota: QuotaState{Exceeded: true, Reason: "quota", NextRecoverAt: now.Add(120 * time.Hour)}, UpdatedAt: now.Add(-time.Minute)},
				"denied":   {Status: StatusError, LastError: &Error{HTTPStatus: 403, Message: "model permission"}, Unavailable: true, NextRetryAfter: now.Add(time.Hour), UpdatedAt: now.Add(-time.Minute)},
			}}
			if scenario == "disabled" {
				a.Disabled = true
			}
			m.Register(context.Background(), a)
			base, _ := m.GetByID("plus")
			observed := time.Now()
			if scenario == "stale" {
				observed = now.Add(-10 * time.Minute)
			}
			if scenario == "newer_failure" {
				m.MarkResult(context.Background(), Result{AuthID: "plus", Model: "gpt-test", Error: failure})
			}
			if scenario == "changed_credential" {
				changed := base.Clone()
				changed.Metadata["access_token"] = "version2"
				m.Update(context.Background(), changed)
			}
			if scenario == "registration" {
				m.Register(context.Background(), base.Clone())
			}
			if scenario == "newer_observation" {
				m.mu.Lock()
				m.auths[base.ID].Quota.ObservedAt = observed.Add(time.Millisecond)
				m.mu.Unlock()
			}
			r, ok := any(m).(verifiedQuotaReconciler)
			if !ok {
				t.Fatal("SDK has no atomic selective verified quota recovery")
			}
			_, models, err := r.ReconcileVerifiedQuota(context.Background(), base, observed)
			if err != nil {
				t.Fatal(err)
			}
			got, _ := m.GetByID("plus")
			if scenario == "recovered" {
				if got.Quota.Exceeded || got.ModelStates["gpt-test"].Unavailable || len(models) != 1 {
					t.Fatalf("quota remains blocked: models=%v quota=%v", models, got.Quota)
				}
				if !got.ModelStates["denied"].Unavailable || got.ModelStates["denied"].LastError.HTTPStatus != 403 {
					t.Fatal("unrelated model permission was cleared")
				}
			} else if len(models) != 0 || got.ModelStates["gpt-test"].Unavailable != base.ModelStates["gpt-test"].Unavailable {
				t.Fatal("unsafe recovery cleared hold")
			}
		})
	}
}
