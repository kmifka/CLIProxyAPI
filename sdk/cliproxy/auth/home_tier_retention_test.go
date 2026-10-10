package auth

import (
	"context"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
	"testing"
)

func TestTierRequestDoesNotRetainUnvalidatedHomeSelection(t *testing.T) {
	m := NewManager(nil, nil, nil)
	opts := executor.Options{Metadata: map[string]any{executor.ServiceTierMetadataKey: "ultrafast"}}
	if !homeTierRequiresMembership(opts) {
		t.Fatal("tier request could use retained selection")
	}
	if homeTierRequiresMembership(executor.Options{}) {
		t.Fatal("legacy request behavior changed")
	}
	if _, err := m.pickHomeDispatchSelection(executor.WithDownstreamWebsocket(context.Background()), "gpt", opts); err == nil || err.Error() != "auth_unavailable: tier membership requires fresh Home dispatch" {
		t.Fatalf("retention fence: %v", err)
	}
}
