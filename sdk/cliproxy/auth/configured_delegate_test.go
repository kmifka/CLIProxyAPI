package auth

import (
	"context"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
	"testing"
	"time"
)

func TestConfiguredDelegatePreservesWeightedPolicyAndNarrowing(t *testing.T) {
	ctx := context.Background()
	selector := &WeightedRoundRobinSelector{}
	m := NewManager(nil, selector, nil)
	m.executors["codex"] = schedulerTestExecutor{}
	for _, a := range []*Auth{{ID: "a", Provider: "codex", Attributes: map[string]string{AttributeWeight: "3"}}, {ID: "b", Provider: "codex", Attributes: map[string]string{AttributeWeight: "1"}}, {ID: "excluded", Provider: "codex", Attributes: map[string]string{AttributeWeight: "100"}}} {
		if _, e := m.Register(ctx, a); e != nil {
			t.Fatal(e)
		}
	}
	m.SetPluginScheduler(affinityPolicyFunc(func(context.Context, pluginapi.SchedulerPickRequest) (pluginapi.SchedulerPickResponse, bool, error) {
		return pluginapi.SchedulerPickResponse{Handled: true, DelegateBuiltin: "configured", EligibleAuthIDs: []string{"a", "b"}}, true, nil
	}))
	counts := map[string]int{}
	for i := 0; i < 40; i++ {
		a, e := m.SelectAuth(ctx, "codex", "", cliproxyexecutor.Options{})
		if e != nil {
			t.Fatal(e)
		}
		counts[a.ID]++
	}
	if counts["a"] != 30 || counts["b"] != 10 || counts["excluded"] != 0 {
		t.Fatalf("configured weighted narrowed distribution=%v", counts)
	}
	for _, kind := range []string{"manual", "quota"} {
		m.SetPluginScheduler(affinityPolicyFunc(func(_ context.Context, req pluginapi.SchedulerPickRequest) (pluginapi.SchedulerPickResponse, bool, error) {
			m.mu.Lock()
			a := m.auths["a"]
			a.Unavailable = true
			a.NextRetryAfter = time.Now().Add(time.Hour)
			if kind == "quota" {
				a.Quota.Exceeded = true
			} else {
				a.LastError = &Error{Code: "manual_hold", Message: "operator hold"}
			}
			m.mu.Unlock()
			ids := []string{}
			for _, c := range req.Candidates {
				if c.ID != "excluded" {
					ids = append(ids, c.ID)
				}
			}
			return pluginapi.SchedulerPickResponse{Handled: true, DelegateBuiltin: "configured", EligibleAuthIDs: ids}, true, nil
		}))
		a, e := m.SelectAuth(ctx, "codex", "", cliproxyexecutor.Options{})
		if e != nil || a.ID != "b" {
			t.Fatalf("%s hold not retained: %v %v", kind, a, e)
		}
		m.mu.RLock()
		held := m.auths["a"].Unavailable && m.auths["a"].NextRetryAfter.After(time.Now())
		m.mu.RUnlock()
		if !held {
			t.Fatalf("%s hold cleared", kind)
		}
	}
}
