package auth

import (
	"context"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
	"testing"
)

func TestHomeUltrafastMissingGuaranteeDeniesSelection(t *testing.T) {
	d := &fixtureHomeDispatcher{payload: []byte(`{"model":"gpt","provider":"codex","auth_index":"canonical","auth":{"id":"fixture","provider":"codex"}}`)}
	m := newHomeSelectionTestManager(t, d)
	m.RegisterExecutor(schedulerTestExecutor{provider: "codex"})
	s, err := m.pickHomeDispatchSelection(context.Background(), "gpt", cliproxyexecutor.Options{Metadata: map[string]any{cliproxyexecutor.ServiceTierMetadataKey: "ultrafast"}})
	if s != nil {
		s.End("test")
		t.Fatal("ultrafast accepted without Home guarantee")
	}
	if err == nil {
		t.Fatal("missing guarantee did not deny")
	}
}
