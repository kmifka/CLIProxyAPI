package cliproxy

import (
	"context"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/config"
	"path/filepath"
	"testing"
)

func TestStaticHomeFingerprintIncludesBarrierAndCallbackPanic(t *testing.T) {
	cfg, _ := config.ParseConfigBytes([]byte("request-retry: 7\n"))
	cfg.Home.Enabled = true
	s, err := NewBuilder().WithStaticHomeConfig(cfg, func(StaticHomeRejection) { panic("callback") }).WithConfigPath(filepath.Join(t.TempDir(), "c.yaml")).Build()
	if err != nil {
		t.Fatal(err)
	}
	if err = s.checkStaticHomeConfig(cfg); err != nil {
		t.Fatal(err)
	}
	cfg.CredentialConcurrency.ObservationBarrierRevision++
	if err = s.checkStaticHomeConfig(cfg); err == nil {
		t.Fatal("barrier revision omitted")
	}
	s.applyWatcherConfigUpdate(cfg)
	if s.cfg.CredentialConcurrency.ObservationBarrierRevision != 0 {
		t.Fatal("watcher mutated config")
	}
}

func TestStaticHomeFreezesBeforeBuildAndRejectsOverlay(t *testing.T) {
	cfg, err := config.ParseConfigBytes([]byte("request-retry: 7\n"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Home.Enabled = true
	b := NewBuilder().WithStaticHomeConfig(cfg, nil).WithConfigPath(filepath.Join(t.TempDir(), "config.yaml"))
	cfg.RequestRetry = 9
	s, err := b.Build()
	if err != nil {
		t.Fatal(err)
	}
	if s.cfg.RequestRetry != 7 {
		t.Fatal("supplied config was not frozen")
	}
	remote, _ := config.ParseConfigBytes([]byte("request-retry: 8\n"))
	if _, err := s.stageHomeOverlayWithClient(context.Background(), remote, nil); err == nil {
		t.Fatal("changed overlay accepted")
	}
	if s.cfg.RequestRetry != 7 {
		t.Fatal("rejection mutated config")
	}
}
