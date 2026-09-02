package auth

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"
)

// setPassive forces the once-guarded flag for a test and restores it after.
func setPassive(t *testing.T, value string) {
	t.Helper()
	prev := os.Getenv(passiveEnvVar)
	t.Setenv(passiveEnvVar, value)
	passiveOnce = sync.Once{}
	passiveMode = false
	t.Cleanup(func() {
		os.Setenv(passiveEnvVar, prev)
		passiveOnce = sync.Once{}
		passiveMode = false
	})
}

func TestPassiveModeEnvParsing(t *testing.T) {
	for _, tc := range []struct {
		env  string
		want bool
	}{
		{"1", true}, {"true", true}, {"TRUE", true}, {"yes", true}, {"on", true},
		{" true ", true},
		{"", false}, {"0", false}, {"false", false}, {"maybe", false},
	} {
		setPassive(t, tc.env)
		if got := PassiveMode(); got != tc.want {
			t.Errorf("PassiveMode() with %q = %v, want %v", tc.env, got, tc.want)
		}
	}
}

// The guard that matters: a passive instance must never call the refresh
// endpoint, because a rotation cannot be undone locally.
func TestPassiveModeSuppressesRefresh(t *testing.T) {
	setPassive(t, "1")
	m := NewManager(nil, nil, nil)
	_, err := m.refreshAuthForRequest(context.Background(), "some-auth", "")
	if !errors.Is(err, errPassiveRefresh) {
		t.Fatalf("refreshAuthForRequest = %v, want errPassiveRefresh", err)
	}
}

func TestActiveModeDoesNotSuppressRefresh(t *testing.T) {
	setPassive(t, "")
	m := NewManager(nil, nil, nil)
	_, err := m.refreshAuthForRequest(context.Background(), "some-auth", "")
	if errors.Is(err, errPassiveRefresh) {
		t.Fatal("refresh suppressed although passive mode is off")
	}
}

func TestPassiveModeStartsNoRefreshLoop(t *testing.T) {
	setPassive(t, "1")
	m := NewManager(nil, nil, nil)
	m.StartAutoRefresh(context.Background(), time.Minute)

	m.mu.Lock()
	loop, cancel := m.refreshLoop, m.refreshCancel
	m.mu.Unlock()
	if loop != nil || cancel != nil {
		t.Fatal("passive mode started a refresh loop")
	}
}

// A passive instance shares the auth directory with the maintaining instance,
// so it must not write into it either.
func TestPassiveModeSuppressesPersist(t *testing.T) {
	setPassive(t, "1")
	store := &recordingStore{}
	m := NewManager(store, nil, nil)
	auth := &Auth{ID: "a", Metadata: map[string]any{"k": "v"}}
	if err := m.persist(context.Background(), auth); err != nil {
		t.Fatalf("persist returned %v", err)
	}
	if store.saves != 0 {
		t.Fatalf("passive mode wrote %d times to the auth store", store.saves)
	}
}

type recordingStore struct {
	Store
	saves int
}

func (s *recordingStore) Save(_ context.Context, auth *Auth) (string, error) {
	s.saves++
	return auth.ID, nil
}
