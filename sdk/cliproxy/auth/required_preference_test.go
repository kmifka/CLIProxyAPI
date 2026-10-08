package auth

import (
	"context"
	"errors"
	executor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
	"testing"
)

type unavailablePreference struct{ mode string }

func (s unavailablePreference) HasScheduler() bool             { return s.mode != "unloaded" }
func (s unavailablePreference) SchedulerWantsPreference() bool { return s.mode != "no-capability" }
func (s unavailablePreference) PickAuth(context.Context, pluginapi.SchedulerPickRequest) (pluginapi.SchedulerPickResponse, bool, error) {
	switch s.mode {
	case "error":
		return pluginapi.SchedulerPickResponse{}, true, errors.New("scheduler failed")
	case "invalid":
		return pluginapi.SchedulerPickResponse{Handled: true, EligibleAuthIDs: []string{"unknown"}}, true, nil
	default:
		return pluginapi.SchedulerPickResponse{}, false, nil
	}
}
func TestRequiredPreferenceNeverFallsBack(t *testing.T) {
	for _, mode := range []string{"missing", "unloaded", "no-capability", "declined", "invalid", "error"} {
		t.Run(mode, func(t *testing.T) {
			m := NewManager(nil, nil, nil)
			m.executors["codex"] = schedulerTestExecutor{}
			if _, err := m.Register(context.Background(), &Auth{ID: "plus", Provider: "codex"}); err != nil {
				t.Fatal(err)
			}
			if mode != "missing" {
				m.SetPluginScheduler(unavailablePreference{mode})
			}
			ctx := WithRequiredSchedulerPreference(context.Background())
			if a, err := m.SelectAuth(ctx, "codex", "", executor.Options{}); err == nil || a != nil {
				t.Fatalf("native fallback: %v %v", a, err)
			}
			// Ordinary traffic retains the existing permissive native fallback.
			if mode != "error" {
				if a, err := m.SelectAuth(context.Background(), "codex", "", executor.Options{}); err != nil || a == nil {
					t.Fatalf("ordinary request: %v %v", a, err)
				}
			}
		})
	}
}
