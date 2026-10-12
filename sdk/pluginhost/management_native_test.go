package pluginhost_test

import (
	"context"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginhost"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Opt-in native acceptance uses the actual Costs and Fleet dynamic libraries.
// The backend below is synthetic transport, not PostgreSQL valuation evidence.
func TestPublicManagementNativeCostsFleet(t *testing.T) {
	dir := os.Getenv("SDK_MANAGEMENT_NATIVE_PLUGINS")
	if dir == "" {
		t.Skip("set SDK_MANAGEMENT_NATIVE_PLUGINS to exact Costs/Fleet libraries")
	}
	// Export synthetic reader settings before launching: plugins use C getenv.
	if os.Getenv("LLM_REFERENCE_MANAGEMENT_URL") == "" {
		t.Fatal("synthetic reader URL required")
	}
	h := pluginhost.New()
	defer h.ShutdownAll()
	other := pluginhost.New()
	defer other.ShutdownAll()
	authDir := filepath.Join(t.TempDir(), "must-not-exist")
	enabled := true
	cfg := pluginhost.RuntimeConfig{Enabled: true, Dir: dir, AuthDir: authDir, Configs: map[string]pluginhost.PluginInstanceConfig{"reference-pricing": {Enabled: &enabled}, "fleet": {Enabled: &enabled}}}
	h.ApplyConfig(context.Background(), cfg)
	h.RegisterManagementRoutes(context.Background(), nil)
	plugins := h.RegisteredPlugins()
	if len(plugins) != 2 {
		t.Fatalf("actual Costs/Fleet metadata: %+v", plugins)
	}
	for _, p := range plugins {
		if p.Metadata.Name == "" {
			t.Fatalf("missing proper metadata: %+v", p)
		}
	}
	h.ApplyConfig(context.Background(), cfg) // actual plugin.reconfigure
	h.RegisterManagementRoutes(context.Background(), nil)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v0/management/") {
			if r.Header.Get("Authorization") != "Bearer synthetic-viewer" {
				http.Error(w, "unauthorized", 401)
				return
			}
			if r.Method != "GET" {
				http.Error(w, "viewer write / CSRF denied", 403)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 1024)
			if h.ServeManagementHTTP(w, r) {
				return
			}
		} else if h.ServeResourceHTTP(w, r) {
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	for _, c := range []struct {
		method, path, key string
		status            int
	}{
		{"GET", "/v0/management/plugins/reference-pricing/overview", "", 401},
		{"GET", "/v0/management/plugins/reference-pricing/overview", "synthetic-viewer", 200},
		{"POST", "/v0/management/plugins/reference-pricing/prices", "synthetic-viewer", 403},
		{"GET", "/v0/resource/plugins/reference-pricing/monitor", "", 200},
		{"GET", "/v0/resource/plugins/fleet/fleet", "", 200},
		{"POST", "/v0/resource/plugins/fleet/fleet", "", 404},
		{"GET", "/v0/resource/plugins/reference-pricing/overview", "", 404},
	} {
		r, _ := http.NewRequest(c.method, server.URL+c.path, nil)
		if c.key != "" {
			r.Header.Set("Authorization", "Bearer "+c.key)
		}
		resp, err := server.Client().Do(r)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != c.status {
			t.Fatalf("%s %s: %d want %d: %s", c.method, c.path, resp.StatusCode, c.status, body)
		}
	}
	if other.ServeResourceHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/v0/resource/plugins/fleet/fleet", nil)) {
		t.Fatal("host scopes leaked")
	}
	h.ShutdownAllContext(context.Background())
	if len(h.RegisteredPlugins()) != 0 || h.PluginBusy("fleet") || h.PluginBusy("reference-pricing") {
		t.Fatal("shutdown retained plugins")
	}
	for _, p := range []string{"/v0/resource/plugins/fleet/fleet", "/v0/management/plugins/reference-pricing/overview"} {
		r := httptest.NewRequest("GET", p, nil)
		w := httptest.NewRecorder()
		if h.ServeResourceHTTP(w, r) || h.ServeManagementHTTP(w, r) {
			t.Fatal("shutdown retained routes")
		}
	}
	if _, err := os.Stat(authDir); !os.IsNotExist(err) {
		t.Fatalf("management lifecycle wrote AuthDir: %v", err)
	}
	t.Log("actual Costs/Fleet ABI Init, Reconfigure, protected HTTP401/200/403, resources, host scopes and Shutdown PASS; no Service/executors/auto-refresh started")
}
