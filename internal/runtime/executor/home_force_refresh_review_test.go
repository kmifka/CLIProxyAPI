package executor

import (
	"context"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	auth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	"net/http"
	"sync/atomic"
	"testing"
)

type homeReviewDenyTransport struct{ calls atomic.Int32 }

func (d *homeReviewDenyTransport) RoundTrip(*http.Request) (*http.Response, error) {
	d.calls.Add(1)
	panic("Home-unavailable attempted local provider HTTP")
}

func TestHomeReviewCodexUnavailableNeverLocalRefresh(t *testing.T) {
	d := &homeReviewDenyTransport{}
	old := http.DefaultTransport
	http.DefaultTransport = d
	defer func() { http.DefaultTransport = old }()
	cfg := &config.Config{Home: config.HomeConfig{Enabled: true}}
	_, err := NewCodexAutoExecutor(cfg).Refresh(context.Background(), &auth.Auth{ID: "isolated", Index: "isolated", Provider: "codex", Metadata: map[string]any{"refresh_token": "synthetic-only"}})
	if err == nil {
		t.Fatal("missing Home must fail")
	}
	status, ok := err.(interface{ StatusCode() int })
	if !ok || status.StatusCode() != 503 {
		t.Fatalf("Home unavailable did not return 503: %T", err)
	}
	if d.calls.Load() != 0 {
		t.Fatal("local provider attempted")
	}
}
