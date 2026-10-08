package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

type preferenceScheduler struct {
	preferred []string
	picks     int
}

func (*preferenceScheduler) SchedulerWantsAcrossPriorities() bool { return true }
func (*preferenceScheduler) SchedulerWantsPreference() bool       { return true }
func (s *preferenceScheduler) PickAuth(_ context.Context, req pluginapi.SchedulerPickRequest) (pluginapi.SchedulerPickResponse, bool, error) {
	raw, _ := json.Marshal(req)
	var stage struct{ PreferenceOnly bool }
	_ = json.Unmarshal(raw, &stage)
	if stage.PreferenceOnly {
		raw, _ = json.Marshal(map[string]any{"Handled": true, "EligibleAuthIDs": s.preferred})
		var resp pluginapi.SchedulerPickResponse
		_ = json.Unmarshal(raw, &resp)
		return resp, true, nil
	}
	s.picks++
	return pluginapi.SchedulerPickResponse{Handled: true, AuthID: s.preferred[0]}, true, nil
}

func TestPluginPreferenceReevaluatesBoundCredentialWithoutChurningPeers(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		t.Run(map[bool]string{false: "single", true: "mixed"}[mixed], func(t *testing.T) {
			ctx := context.Background()
			affinity := NewSessionAffinitySelector(nil)
			defer affinity.Stop()
			m := NewManager(nil, affinity, nil)
			m.executors["codex"] = schedulerTestExecutor{}
			for _, id := range []string{"plus-a", "plus-b", "reserve"} {
				m.Register(ctx, &Auth{ID: id, Provider: "codex"})
			}
			s := &preferenceScheduler{preferred: []string{"reserve"}}
			m.SetPluginScheduler(s)
			opts := cliproxyexecutor.Options{Headers: http.Header{"Session_id": {"stable"}}}
			pick := func() *Auth {
				t.Helper()
				var a *Auth
				var err error
				if mixed {
					a, _, _, err = m.pickNextMixed(ctx, []string{"codex"}, "", opts, nil)
				} else {
					a, _, err = m.pickNext(ctx, "codex", "", opts, nil)
				}
				if err != nil {
					t.Fatal(err)
				}
				return a
			}
			if a := pick(); a.ID != "reserve" {
				t.Fatalf("exhausted pool: %s", a.ID)
			}
			s.preferred = []string{"plus-a", "plus-b"}
			if a := pick(); a.ID != "plus-a" {
				t.Fatalf("recovered preferred pool must replace bound reserve: %s", a.ID)
			}
			s.preferred = []string{"plus-b", "plus-a"}
			if a := pick(); a.ID != "plus-a" {
				t.Fatalf("same preference tier must preserve binding: %s", a.ID)
			}
			if s.picks != 2 {
				t.Fatalf("cold/failback picks=%d want 2", s.picks)
			}
		})
	}
}
