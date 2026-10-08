package pluginhost

import (
	"context"
	"net/http"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/registry"
	coreauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	executor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

func TestSchedulerAffinityRegistrationReload(t *testing.T) {
	ctx := context.Background()
	affinity := coreauth.NewSessionAffinitySelector(nil)
	defer affinity.Stop()
	manager := coreauth.NewManager(nil, affinity, nil)
	model := "reload-composition-model"
	for _, provider := range []string{"codex", "gemini"} {
		manager.RegisterExecutor(&stubCompatExecutor{id: provider})
		for _, suffix := range []string{"a", "b"} {
			id := t.Name() + "-" + provider + "-" + suffix
			if _, err := manager.Register(ctx, &coreauth.Auth{ID: id, Provider: provider}); err != nil {
				t.Fatal(err)
			}
			registry.GetGlobalRegistry().RegisterClient(id, provider, []*registry.ModelInfo{{ID: model}})
			t.Cleanup(func() { registry.GetGlobalRegistry().UnregisterClient(id) })
		}
	}
	calls := 0
	policy := schedulerFunc(func(_ context.Context, req pluginapi.SchedulerPickRequest) (pluginapi.SchedulerPickResponse, error) {
		if req.Provider != "codex" {
			return pluginapi.SchedulerPickResponse{}, nil
		}
		calls++
		return pluginapi.SchedulerPickResponse{Handled: true, AuthID: t.Name() + "-codex-b"}, nil
	})
	result := validTestPlugin("alpha")
	result.Capabilities.Scheduler = policy
	loader := newTestSymbolLoader()
	plugin := &testPlugin{registerResult: result, reconfigureResult: result}
	loader.lookups["alpha"] = newTestSymbolLookup(plugin)
	host := NewForTest(loader)
	defer host.ShutdownAll()
	host.SetAuthManager(manager)
	cfg := &config.Config{Plugins: config.PluginsConfig{Enabled: true, Dir: makePluginDir(t, "alpha"), Configs: enabledPluginConfigs("alpha")}}
	host.ApplyConfig(ctx, cfg)
	manager.SetPluginScheduler(host)
	pick := func(provider, session string) *coreauth.Auth {
		t.Helper()
		got, err := manager.SelectAuth(ctx, provider, model, executor.Options{Headers: http.Header{"Session_id": {session}}, Metadata: map[string]any{}})
		if err != nil || got == nil {
			t.Fatalf("pick %s: %v %v", provider, got, err)
		}
		return got
	}
	if got := pick("codex", "before-reload"); got.ID != t.Name()+"-codex-b" {
		t.Fatal(got.ID)
	}
	host.ApplyConfig(ctx, cfg)
	manager.SetPluginScheduler(host) // Service sync re-injects the same host on reload.
	if got := pick("codex", "before-reload"); got.ID != t.Name()+"-codex-b" {
		t.Fatal(got.ID)
	}
	if calls != 1 {
		t.Fatalf("reload bypassed native binding: calls=%d", calls)
	}
	if got := pick("codex", "after-reload"); got.ID != t.Name()+"-codex-b" {
		t.Fatal(got.ID)
	}
	// A routing change replaces the native selector, but not the registered plugin policy.
	replacement := coreauth.NewSessionAffinitySelector(&coreauth.FillFirstSelector{})
	manager.SetSelector(replacement)
	defer replacement.Stop()
	if got := pick("codex", "new-selector"); got.ID != t.Name()+"-codex-b" {
		t.Fatal(got.ID)
	}
	if calls != 3 {
		t.Fatalf("policy did not survive reload: %d", calls)
	}
	if plugin.registerCalls != 1 || plugin.reconfigureCalls != 1 {
		t.Fatalf("lifecycle register=%d reconfigure=%d", plugin.registerCalls, plugin.reconfigureCalls)
	}
	if got := pick("gemini", "non-codex"); got.ID != t.Name()+"-gemini-a" {
		t.Fatalf("native non-codex changed: %s", got.ID)
	}
	if calls != 3 {
		t.Fatal("non-codex handled by application policy")
	}
}
