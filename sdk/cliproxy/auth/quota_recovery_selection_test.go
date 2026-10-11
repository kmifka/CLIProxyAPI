package auth

import (
	"context"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/registry"
	executor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
	"testing"
	"time"
)

type recoverySelectionExecutor struct {
	compactTestExecutor
	selected []string
}

func (*recoverySelectionExecutor) Identifier() string { return "codex" }
func (e *recoverySelectionExecutor) Execute(ctx context.Context, a *Auth, r executor.Request, o executor.Options) (executor.Response, error) {
	e.selected = append(e.selected, a.ID)
	return executor.Response{Payload: []byte(`{"ok":true}`)}, nil
}

func TestVerifiedQuotaRecoveryRestoresPropagatedSiblingSelection(t *testing.T) {
	ctx := context.Background()
	m := NewManager(nil, &FillFirstSelector{}, nil)
	e := &recoverySelectionExecutor{}
	m.RegisterExecutor(e)
	routes := []string{"recovery-sol", "recovery-sibling", "recovery-third"}
	states := map[string]*ModelState{}
	for _, route := range routes {
		states[route] = &ModelState{Status: StatusActive}
	}
	states["permission"] = &ModelState{Status: StatusError, Unavailable: true, NextRetryAfter: time.Now().Add(48 * time.Hour), LastError: &Error{HTTPStatus: 403, Message: "permission denied"}}
	states["unrelated-quota"] = &ModelState{Status: StatusError, Unavailable: true, NextRetryAfter: time.Now().Add(72 * time.Hour), Quota: QuotaState{Exceeded: true, Reason: "quota", NextRecoverAt: time.Now().Add(72 * time.Hour)}, LastError: &Error{HTTPStatus: 429, Message: "unknown quota"}}
	for _, a := range []*Auth{{ID: "recovery-plus", Provider: "codex", Status: StatusActive, Attributes: map[string]string{"priority": "10"}, ModelStates: states}, {ID: "recovery-reserve", Provider: "codex", Status: StatusActive}} {
		if _, err := m.Register(ctx, a); err != nil {
			t.Fatal(err)
		}
		models := []*registry.ModelInfo{}
		for _, r := range routes {
			models = append(models, &registry.ModelInfo{ID: r})
		}
		registry.GetGlobalRegistry().RegisterClient(a.ID, "codex", models)
		id := a.ID
		t.Cleanup(func() { registry.GetGlobalRegistry().UnregisterClient(id) })
	}
	retry := 120 * time.Hour
	m.MarkResult(ctx, Result{AuthID: "recovery-plus", Provider: "codex", Model: routes[0], CredentialScope: true, RetryAfter: &retry, Error: &Error{HTTPStatus: 429, Message: `{"error":{"type":"usage_limit_reached"}}`}})
	for _, r := range routes {
		if _, err := m.Execute(ctx, []string{"codex"}, executor.Request{Model: r}, executor.Options{}); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range e.selected {
		if id != "recovery-reserve" {
			t.Fatal("pre-recovery did not fall back to reserve")
		}
	}
	base, _ := m.GetByID("recovery-plus")
	observed := time.Now()
	got, models, err := m.ReconcileVerifiedQuota(ctx, base, observed)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(models) != len(routes) {
		t.Fatalf("same incident sibling holds remain: recovered=%v want=%v", models, routes)
	}
	if !got.ModelStates["permission"].Unavailable || got.ModelStates["permission"].LastError.HTTPStatus != 403 || !got.ModelStates["unrelated-quota"].Unavailable {
		t.Fatal("unrelated holds erased")
	}
	e.selected = nil
	for _, r := range routes {
		if _, err := m.Execute(ctx, []string{"codex"}, executor.Request{Model: r}, executor.Options{}); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range e.selected {
		if id != "recovery-plus" {
			t.Fatalf("post-recovery selected %s instead of recovered Plus", id)
		}
	}
}
