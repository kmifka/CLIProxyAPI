package auth

import (
	"context"
	"fmt"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/registry"
	"net/http"
	"testing"
	"time"

	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v8/sdk/translator"
)

type affinityPolicyFunc func(context.Context, pluginapi.SchedulerPickRequest) (pluginapi.SchedulerPickResponse, bool, error)

func (f affinityPolicyFunc) PickAuth(ctx context.Context, req pluginapi.SchedulerPickRequest) (pluginapi.SchedulerPickResponse, bool, error) {
	return f(ctx, req)
}

func TestPluginSchedulerConcurrentDisableDoesNotBind(t *testing.T) {
	ctx := context.Background()
	affinity := NewSessionAffinitySelector(nil)
	defer affinity.Stop()
	manager := NewManager(nil, affinity, nil)
	manager.executors["codex"] = schedulerTestExecutor{}
	for _, id := range []string{"a", "b"} {
		if _, err := manager.Register(ctx, &Auth{ID: id, Provider: "codex"}); err != nil {
			t.Fatal(err)
		}
	}
	manager.SetPluginScheduler(affinityPolicyFunc(func(context.Context, pluginapi.SchedulerPickRequest) (pluginapi.SchedulerPickResponse, bool, error) {
		if _, err := manager.Update(ctx, &Auth{ID: "b", Provider: "codex", Disabled: true}); err != nil {
			t.Fatal(err)
		}
		return pluginapi.SchedulerPickResponse{Handled: true, AuthID: "b"}, true, nil
	}))
	got, err := manager.SelectAuth(ctx, "codex", "", cliproxyexecutor.Options{Headers: http.Header{"Session_id": {"concurrent"}}})
	if err != nil || got == nil || got.ID != "a" {
		t.Fatalf("stale selection must rotate: %v %v", got, err)
	}
}

func TestPluginSchedulerAffinityFailover(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		for _, reason := range []string{"retry", "disabled", "cooldown", "model", "kind"} {
			t.Run(fmt.Sprintf("mixed=%v/%s", mixed, reason), func(t *testing.T) {
				ctx := context.Background()
				affinity := NewSessionAffinitySelector(nil)
				defer affinity.Stop()
				manager := NewManager(nil, affinity, nil)
				manager.executors["codex"] = schedulerTestExecutor{}
				model := "composition-model"
				ids := []string{t.Name() + "-a", t.Name() + "-b"}
				for _, id := range ids {
					manager.Register(ctx, &Auth{ID: id, Provider: "codex", Attributes: map[string]string{AttributeAuthKind: AuthKindOAuth}})
					registry.GetGlobalRegistry().RegisterClient(id, "codex", []*registry.ModelInfo{{ID: model}})
					t.Cleanup(func() { registry.GetGlobalRegistry().UnregisterClient(id) })
				}
				scheduler := &fakePluginScheduler{resp: pluginapi.SchedulerPickResponse{Handled: true, AuthID: ids[1]}, handled: true}
				manager.SetPluginScheduler(scheduler)
				opts := cliproxyexecutor.Options{Headers: http.Header{"Session_id": {"rotation"}}, Metadata: map[string]any{}}
				pick := func(tried map[string]struct{}) (*Auth, error) {
					if mixed {
						a, _, _, err := manager.pickNextMixed(ctx, []string{"codex"}, model, opts, tried)
						return a, err
					}
					a, _, err := manager.pickNext(ctx, "codex", model, opts, tried)
					return a, err
				}
				got, err := pick(nil)
				if err != nil || got.ID != ids[1] {
					t.Fatalf("cold %v %v", got, err)
				}
				bound, status := manager.LookupSessionAffinity("codex", model, CanonicalSessionID(opts.Headers, nil, opts.Metadata))
				if status != "bound" || bound.ID != ids[1] {
					t.Fatalf("lookup %v %s", bound, status)
				}
				scheduler.resp.AuthID = ids[0]
				got, err = pick(nil)
				if err != nil || got.ID != ids[1] || scheduler.calls != 1 {
					t.Fatalf("reuse %v %v calls=%d", got, err, scheduler.calls)
				}
				var tried map[string]struct{}
				a := &Auth{ID: ids[1], Provider: "codex"}
				switch reason {
				case "retry":
					tried = map[string]struct{}{ids[1]: {}}
				case "disabled":
					a.Disabled = true
					manager.Update(ctx, a)
				case "cooldown":
					a.ModelStates = map[string]*ModelState{model: {Unavailable: true, NextRetryAfter: time.Now().Add(time.Hour), Quota: QuotaState{Exceeded: true}}}
					manager.Update(ctx, a)
				case "model":
					registry.GetGlobalRegistry().UnregisterClient(ids[1])
				case "kind":
					a.Attributes = map[string]string{"api_key": "fake"}
					manager.Update(ctx, a)
					ctx = withRequiredAuthKind(ctx, AuthKindOAuth)
				}
				got, err = pick(tried)
				if err != nil || got == nil || got.ID != ids[0] {
					t.Fatalf("rotation %v %v", got, err)
				}
			})
		}
	}
}

func TestPluginSchedulerAffinityDelegateAndInvalidResponses(t *testing.T) {
	for _, mode := range []string{"delegate", "invalid", "unhandled", "reject", "error"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			affinity := NewSessionAffinitySelector(nil)
			defer affinity.Stop()
			manager := NewManager(nil, affinity, nil)
			manager.executors["codex"] = schedulerTestExecutor{}
			for _, id := range []string{"a", "b"} {
				manager.Register(ctx, &Auth{ID: id, Provider: "codex"})
			}
			scheduler := &fakePluginScheduler{handled: true, resp: pluginapi.SchedulerPickResponse{Handled: true}}
			switch mode {
			case "delegate":
				scheduler.resp.DelegateBuiltin = pluginapi.SchedulerBuiltinFillFirst
			case "invalid":
				scheduler.resp.AuthID = "absent"
			case "unhandled":
				scheduler.handled = false
			case "reject":
				scheduler.resp.Reject = true
			case "error":
				scheduler.err = fmt.Errorf("policy unavailable")
			}
			manager.SetPluginScheduler(scheduler)
			opts := cliproxyexecutor.Options{Headers: http.Header{"Session_id": {"responses"}}}
			got, err := manager.SelectAuth(ctx, "codex", "", opts)
			if mode == "reject" || mode == "error" {
				if err == nil {
					t.Fatal("explicit rejection must propagate")
				}
				return
			}
			if err != nil || got == nil || got.ID != "a" {
				t.Fatalf("fallback %v %v", got, err)
			}
			scheduler.resp = pluginapi.SchedulerPickResponse{Handled: true, AuthID: "b"}
			scheduler.handled = true
			got, err = manager.SelectAuth(ctx, "codex", "", opts)
			if err != nil || got.ID != "a" || scheduler.calls != 1 {
				t.Fatalf("delegate did not bind: %v %v calls=%d", got, err, scheduler.calls)
			}
		})
	}
}

func TestPluginSchedulerLCPBinding(t *testing.T) {
	ctx := context.Background()
	affinity := NewSessionAffinitySelector(nil)
	defer affinity.Stop()
	manager := NewManager(nil, affinity, nil)
	manager.executors["codex"] = schedulerTestExecutor{}
	for _, id := range []string{"a", "b"} {
		manager.Register(ctx, &Auth{ID: id, Provider: "codex"})
	}
	scheduler := &fakePluginScheduler{handled: true, resp: pluginapi.SchedulerPickResponse{Handled: true, AuthID: "b"}}
	manager.SetPluginScheduler(scheduler)
	for _, body := range []string{
		`{"messages":[{"role":"system","content":"stable"},{"role":"user","content":"first"}]}`,
		`{"messages":[{"role":"system","content":"stable"},{"role":"user","content":"first"},{"role":"assistant","content":"answer"},{"role":"user","content":"continue"}]}`,
	} {
		opts := cliproxyexecutor.Options{SourceFormat: sdktranslator.FormatOpenAI, OriginalRequest: []byte(body), Metadata: map[string]any{cliproxyexecutor.CallerScopeMetadataKey: "caller"}}
		got, err := manager.SelectAuth(ctx, "codex", "", opts)
		if err != nil || got == nil || got.ID != "b" {
			t.Fatalf("LCP selection %v %v", got, err)
		}
		if opts.Metadata[cliproxyexecutor.LCPAffinitySessionIDMetadataKey] == nil {
			t.Fatal("plugin selection did not create native LCP binding")
		}
		scheduler.resp.AuthID = "a"
	}
	if scheduler.calls != 1 {
		t.Fatalf("LCP reuse called policy: %d", scheduler.calls)
	}
}

func TestPluginSchedulerNativeAffinityComposition(t *testing.T) {
	ctx := context.Background()
	affinity := NewSessionAffinitySelector(nil)
	defer affinity.Stop()
	manager := NewManager(nil, affinity, nil)
	manager.executors["codex"] = schedulerTestExecutor{}
	for _, id := range []string{"auth-a", "auth-b"} {
		if _, err := manager.Register(ctx, &Auth{ID: id, Provider: "codex"}); err != nil {
			t.Fatal(err)
		}
	}
	opts := cliproxyexecutor.Options{Headers: http.Header{"Session_id": {"compose-session"}}, Metadata: map[string]any{}}
	scheduler := &fakePluginScheduler{resp: pluginapi.SchedulerPickResponse{Handled: true, AuthID: "auth-b"}, handled: true}
	manager.SetPluginScheduler(scheduler)
	got, err := manager.SelectAuth(ctx, "codex", "", opts)
	if err != nil || got.ID != "auth-b" {
		t.Fatalf("cold selection = %v, %v", got, err)
	}
	// Pick with a non-empty model to observe the binding through the public contract.
	opts2 := cliproxyexecutor.Options{Headers: http.Header{"Session_id": {"compose-session"}}, Metadata: map[string]any{}}
	got, err = affinity.Pick(ctx, "codex", "test-model", opts2, []*Auth{got})
	if err != nil {
		t.Fatal(err)
	}
	bound, status := manager.LookupSessionAffinity("codex", "test-model", CanonicalSessionID(opts2.Headers, nil, opts2.Metadata))
	if status != "bound" || bound == nil || bound.ID != "auth-b" {
		t.Fatalf("binding with scheduler: %v, %s", bound, status)
	}
	scheduler.resp.AuthID = "auth-a"
	got, err = manager.SelectAuth(ctx, "codex", "", opts)
	if err != nil || got.ID != "auth-b" {
		t.Fatalf("native binding bypassed = %v, %v", got, err)
	}
	if scheduler.calls != 1 {
		t.Fatalf("scheduler called for bound session: %d", scheduler.calls)
	}
}
