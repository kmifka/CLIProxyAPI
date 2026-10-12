package pluginhost

import (
	"context"
	"net/http"
)

// RegisterManagementRoutes rebuilds native plugin management and resource route
// tables. Call after ApplyConfig (which performs plugin Init or Reconfigure).
// reserved contains host-owned route keys in "METHOD /v0/management/path" form.
// This does not register an HTTP router, start a Service, or authenticate requests.
// RegisteredPlugins includes resource menu metadata after registration.
func (h *Host) RegisterManagementRoutes(ctx context.Context, reserved map[string]struct{}) {
	if h == nil || h.inner == nil {
		return
	}
	h.inner.RegisterManagementRoutes(ctx, reserved)
}

// ServeManagementHTTP dispatches a native plugin management request and reports
// whether it handled the route. The caller MUST authenticate and authorize the
// request, enforce write scopes and CSRF, and bound request bodies BEFORE calling
// this method. It intentionally does not install SDK middleware or create a
// second authentication realm. False leaves the response untouched.
func (h *Host) ServeManagementHTTP(w http.ResponseWriter, r *http.Request) bool {
	if h == nil || h.inner == nil {
		return false
	}
	return h.inner.ServeManagementHTTP(w, r)
}

// ServeResourceHTTP dispatches a native GET-only plugin browser resource.
// Resources may be public static assets: never register credentials, auth data,
// or protected management data as resources. Protected data belongs behind the
// host's authenticated management API. False leaves the response untouched.
func (h *Host) ServeResourceHTTP(w http.ResponseWriter, r *http.Request) bool {
	if h == nil || h.inner == nil {
		return false
	}
	return h.inner.ServeResourceHTTP(w, r)
}
