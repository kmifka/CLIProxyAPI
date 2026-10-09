//go:build static_home_probe

package cliproxy

import (
	"context"
	"net"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/config"
)

// Investigation probes deliberately assert the existing unsafe observations.
// They do not implement or qualify a static-config policy.
func TestStaticHomeProbeAfterStartRunsAfterBindFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	cfg := &config.Config{Host: "127.0.0.1", Port: listener.Addr().(*net.TCPAddr).Port}
	cfg.Home.Enabled = true
	var notified atomic.Bool
	service, err := NewBuilder().WithConfig(cfg).WithConfigPath(filepath.Join(t.TempDir(), "config.yaml")).WithHooks(Hooks{OnAfterStart: func(*Service) { notified.Store(true) }}).Build()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err = service.Run(ctx)
	if err == nil {
		t.Fatal("expected actual occupied-listener failure")
	}
	if notified.Load() {
		t.Fatalf("startup receipt fired although Run failed: %v", err)
	}
}

func TestStaticHomeProbeBuilderUsesConfigBeforeInitialHomeGET(t *testing.T) {
	cfg := &config.Config{RequestRetry: 7}
	cfg.Home.Enabled = true
	service, err := NewBuilder().WithConfig(cfg).WithConfigPath(filepath.Join(t.TempDir(), "config.yaml")).Build()
	if err != nil {
		t.Fatal(err)
	}
	if service.coreManager == nil || service.cfg != cfg {
		t.Fatal("unexpected builder construction")
	}
	if service.homeClient != nil {
		t.Fatal("Home client unexpectedly fetched before Build")
	}
	t.Log("BLOCKER: manager and plugin configuration happen in Build before any initial Home GET; startup input must be frozen before Build, not on first subscriber update")
}
