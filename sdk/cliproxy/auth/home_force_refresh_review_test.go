package auth

import (
	"context"
	internalconfig "github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestHomeForceReviewQueuedReplacement(t *testing.T) {
	for _, kind := range []string{"epoch", "index", "access", "refresh-only", "non-home"} {
		t.Run(kind, func(t *testing.T) {
			t.Setenv("CLIPROXY_AUTH_PASSIVE", "0")
			m := NewManager(nil, &RoundRobinSelector{}, nil)
			m.runtimeConfig.Store(&internalconfig.Config{Home: internalconfig.HomeConfig{Enabled: kind != "non-home"}})
			e := &concurrencyTrackingRefreshExecutor{id: "codex"}
			m.RegisterExecutor(e)
			a, _ := m.Register(context.Background(), &Auth{ID: "queued", Provider: "codex", Metadata: map[string]any{"access_token": "v1", "refresh_token": "r1"}})
			l := &authRefreshLock{}
			l.mu.Lock()
			var once sync.Once
			unlock := func() { once.Do(l.mu.Unlock) }
			defer unlock()
			m.refreshLocks.Store(a.ID, l)
			out := make(chan error, 1)
			go homeReviewForceCall(m, a.ID, out)
			waitHomeRefreshEntry(t, 1)
			a.Metadata["refresh_token"] = "r2"
			if kind == "access" {
				a.Metadata["access_token"] = "v2"
			}
			if kind == "index" {
				a.Index = "replacement-index"
			}
			if kind == "epoch" {
				_, _ = m.Register(context.Background(), a)
			} else {
				_, _ = m.Update(context.Background(), a)
			}
			unlock()
			select {
			case err := <-out:
				if (kind == "epoch" || kind == "index") && err == nil {
					t.Fatal("replacement epoch bypassed")
				}
				if kind != "epoch" && kind != "index" && err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("queued completion timed out")
			}
			want := int32(0)
			if kind == "non-home" {
				want = 1
			}
			if e.totalEntered.Load() != want {
				t.Fatalf("refresh calls=%d want=%d", e.totalEntered.Load(), want)
			}
		})
	}
}

// waitHomeRefreshEntry is an observed lock-entry barrier, not a sleep window.
// Every worker stack must be inside the manager refresh before release.
func waitHomeRefreshEntry(t *testing.T, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		b := make([]byte, 1<<20)
		b = b[:runtime.Stack(b, true)]
		count := 0
		for _, stack := range strings.Split(string(b), "\n\n") {
			if strings.Contains(stack, "homeReviewForceCall(") && strings.Contains(stack, "refreshAuthForRequestAtEpoch(") && (strings.Contains(stack, "Mutex).lockSlow(") || strings.Contains(stack, "concurrencyTrackingRefreshExecutor).Refresh(")) {
				count++
			}
		}
		if count == n {
			return
		}
		runtime.Gosched()
	}
	t.Fatal("bounded all-entry barrier timed out")
}

func homeReviewForceCall(m *Manager, id string, out chan error) {
	_, err := m.ForceRefreshAuth(context.Background(), id)
	out <- err
}

func TestHomeForceReviewEntryCoalesces(t *testing.T) {
	t.Setenv("CLIPROXY_AUTH_PASSIVE", "0")
	m := NewManager(nil, &RoundRobinSelector{}, nil)
	m.runtimeConfig.Store(&internalconfig.Config{Home: internalconfig.HomeConfig{Enabled: true}})
	e := &concurrencyTrackingRefreshExecutor{id: "codex", enteredCh: make(chan struct{}, 20), releaseCh: make(chan struct{})}
	m.RegisterExecutor(e)
	_, err := m.Register(context.Background(), &Auth{ID: "home-review", Provider: "codex", Metadata: map[string]any{"access_token": "v1", "refresh_token": "r1"}})
	if err != nil {
		t.Fatal(err)
	}
	out := make(chan error, 20)
	for i := 0; i < 20; i++ {
		go homeReviewForceCall(m, "home-review", out)
	}
	waitHomeRefreshEntry(t, 20)
	close(e.releaseCh)
	for i := 0; i < 20; i++ {
		select {
		case err := <-out:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("completion timed out")
		}
	}
	if got := e.totalEntered.Load(); got != 1 {
		t.Fatalf("Home forced contenders rotated %d times; want 1", got)
	}
	// A later explicit force request observes the current version and must execute.
	_, err = m.ForceRefreshAuth(context.Background(), "home-review")
	if err != nil {
		t.Fatal(err)
	}
	if got := e.totalEntered.Load(); got != 2 {
		t.Fatalf("later explicit force calls=%d; want 2", got)
	}
}
