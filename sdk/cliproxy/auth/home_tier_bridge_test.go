package auth

import (
	"context"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
	"net/http"
	"testing"
)

func TestTrustedHomeTierBridge(t *testing.T) {
	for _, tier := range []string{"priority", "ultrafast", "auto"} {
		h := http.Header{"X-Home-Service-Tier": []string{"spoofed"}, "x-home-service-tier": []string{"spoofed-lower"}}
		got := trustedHomeDispatchHeaders(context.Background(), executor.Options{Headers: h, Metadata: map[string]any{executor.ServiceTierMetadataKey: tier}})
		if got.Get("X-Home-Service-Tier") != tier {
			t.Fatalf("got %v want %s", got, tier)
		}
		if len(got) != 1 {
			t.Fatalf("spoof variant survived %v", got)
		}
		if h.Get("X-Home-Service-Tier") != "spoofed" {
			t.Fatal("mutated caller headers")
		}
	}
	got := trustedHomeDispatchHeaders(context.Background(), executor.Options{Headers: http.Header{"X-Home-Service-Tier": []string{"spoof"}}})
	if got.Get("X-Home-Service-Tier") != "" {
		t.Fatal("spoof without metadata survived")
	}
}
