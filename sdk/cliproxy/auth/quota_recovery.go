package auth

import (
	"context"
	"encoding/json"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/registry"
)

func isVerifiedUsageLimitFailure(e *Error) bool {
	if e == nil || e.HTTPStatus != 429 || e.Code == ErrorCodeForceCooldown {
		return false
	}
	var body struct {
		Error struct {
			Type string `json:"type"`
		} `json:"error"`
	}
	return json.Unmarshal([]byte(e.Message), &body) == nil && body.Error.Type == "usage_limit_reached"
}

// ReconcileVerifiedQuota releases only a subscription usage-limit incident.
// The caller must positively verify fresh upstream quota on the exact base
// credential before calling. Unlike ResetQuota this is a compare-and-clear:
// concurrent results, refresh, reload and operator changes invalidate base.
func (m *Manager) ReconcileVerifiedQuota(ctx context.Context, base *Auth, observed time.Time) (*Auth, []string, error) {
	if m == nil || base == nil {
		return nil, nil, nil
	}
	now := time.Now()
	if observed.IsZero() || observed.After(now) || now.Sub(observed) > time.Minute {
		return nil, nil, nil
	}
	m.mu.Lock()
	current := m.auths[base.ID]
	if current == nil || current.Provider != "codex" || current.Disabled || current.Status == StatusDisabled || current.Generation != base.Generation || current.RegistrationEpoch != base.RegistrationEpoch || CredentialsChanged(base, current) || current.UpdatedAt.After(observed) || current.Quota.ObservedAt.After(observed) || !current.Quota.Exceeded || current.Quota.Reason != "credential_quota" || !isVerifiedUsageLimitFailure(current.LastError) {
		m.mu.Unlock()
		return nil, nil, nil
	}
	incidentReset := current.Quota.NextRecoverAt
	var models []string
	for name, s := range current.ModelStates {
		if s == nil || !s.Quota.Exceeded || s.Quota.Reason != "quota" || !s.Quota.NextRecoverAt.Equal(incidentReset) || s.UpdatedAt.After(observed) || !isVerifiedUsageLimitFailure(s.LastError) {
			continue
		}
		resetModelState(s, now)
		models = append(models, name)
	}
	applyCooldownFields(&current.Quota, QuotaState{})
	current.Unavailable = false
	current.NextRetryAfter = time.Time{}
	current.LastError = nil
	current.StatusMessage = ""
	current.Status = StatusActive
	updateAggregatedAvailability(current, now)
	current.Generation++
	current.UpdatedAt = now
	snapshot := current.Clone()
	err := m.persist(ctx, current)
	m.mu.Unlock()
	supported, epoch := registry.GetGlobalRegistry().GetModelsAndEpochForClient(base.ID)
	projections := make([]registry.ClientModelProjection, 0, len(supported))
	for _, model := range supported {
		if model != nil {
			projections = append(projections, m.clientModelProjectionForAuth(snapshot, model.ID, now))
		}
	}
	registry.GetGlobalRegistry().ApplyClientModelProjections(base.ID, epoch, snapshot.Generation, projections)
	if m.scheduler != nil {
		m.scheduler.upsertAuth(snapshot)
	}
	m.persistCooldownStates(context.Background())
	return snapshot, models, err
}
