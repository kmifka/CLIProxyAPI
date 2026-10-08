package auth

import (
	"context"
	executor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
	"net/http"
	"testing"
)

type rejectingPreference struct{ t *testing.T }

func (rejectingPreference) SchedulerWantsAcrossPriorities() bool { return true }
func (rejectingPreference) SchedulerWantsPreference() bool       { return true }
func (s rejectingPreference) PickAuth(_ context.Context, r pluginapi.SchedulerPickRequest) (pluginapi.SchedulerPickResponse, bool, error) {
	if r.Options.Metadata[executor.ServiceTierMetadataKey] != "ultrafast" {
		s.t.Fatal("tier not forwarded")
	}
	return pluginapi.SchedulerPickResponse{Handled: true, Reject: true, RejectCode: "entitlement_unavailable", RejectReason: "no eligible reserve"}, true, nil
}
func TestPreferenceRejectPrecedesBoundNativeAffinity(t *testing.T) {
	ctx := context.Background()
	sel := NewSessionAffinitySelector(nil)
	defer sel.Stop()
	m := NewManager(nil, sel, nil)
	m.executors["codex"] = schedulerTestExecutor{}
	if _, err := m.Register(ctx, &Auth{ID: "plus", Provider: "codex"}); err != nil {
		t.Fatal(err)
	}
	opts := executor.Options{Headers: http.Header{"Session_id": {"stable"}}, Metadata: map[string]any{executor.ServiceTierMetadataKey: "ultrafast"}}
	if _, err := m.SelectAuth(ctx, "codex", "", opts); err != nil {
		t.Fatal(err)
	}
	m.SetPluginScheduler(rejectingPreference{t})
	if a, err := m.SelectAuth(ctx, "codex", "", opts); err == nil || a != nil {
		t.Fatalf("reject bypassed by affinity: %v %v", a, err)
	}
}
