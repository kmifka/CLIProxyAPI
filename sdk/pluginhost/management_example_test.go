package pluginhost_test

import (
	"context"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginhost"
	"net/http"
	"strings"
)

// An embedder supplies its existing shared session, role and CSRF authority.
// No Service.Run, auth store, executors, refresh loop or router cache is needed.
func ExampleHost_RegisterManagementRoutes() {
	h := pluginhost.New()
	defer h.ShutdownAllContext(context.Background()) // Close/unload
	cfg := pluginhost.RuntimeConfig{Enabled: false}  // supply verified plugins/Dir
	h.ApplyConfig(context.Background(), cfg)         // Init, or Reconfigure on later calls
	h.RegisterManagementRoutes(context.Background(), map[string]struct{}{"GET /v0/management/host-status": {}})
	authorize := func(r *http.Request) bool { return false } // fail closed; bind real authority
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v0/management/") {
			// Real authority checks session, scopes and CSRF for writes before dispatch.
			if !authorize(r) {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
			if h.ServeManagementHTTP(w, r) {
				return
			}
		} else if h.ServeResourceHTTP(w, r) {
			return
		} // static assets only, never auth data
		http.NotFound(w, r)
	})
	_ = handler               // mount in the application's existing listener
	_ = h.RegisteredPlugins() // native metadata and resource Menus
}
