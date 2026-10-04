package auth

import (
	"context"
	"os"
	"sync"
	"testing"
)

func TestPassiveModeSuppressesRefreshBeforeExecutor(t *testing.T) {
	previous, wasSet := os.LookupEnv(passiveEnvVar)
	t.Setenv(passiveEnvVar, "1")
	passiveOnce = sync.Once{}
	passiveMode = false
	t.Cleanup(func() {
		if wasSet {
			_ = os.Setenv(passiveEnvVar, previous)
		} else {
			_ = os.Unsetenv(passiveEnvVar)
		}
		passiveOnce = sync.Once{}
		passiveMode = false
	})

	manager := NewManager(nil, &RoundRobinSelector{}, nil)
	executor := &mockOAuthErrorExecutor{id: "test-provider"}
	manager.RegisterExecutor(executor)
	auth := &Auth{ID: "passive-auth", Provider: "test-provider", Metadata: map[string]any{"refresh_token": "test"}}
	if _, err := manager.Register(context.Background(), auth); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.refreshAuthForRequest(context.Background(), auth.ID, ""); err != errPassiveRefresh {
		t.Fatalf("refresh error = %v, want passive suppression", err)
	}
	if executor.refreshCalls.Load() != 0 {
		t.Fatalf("executor refresh calls = %d, want 0", executor.refreshCalls.Load())
	}
}
