package pluginhost_test

import (
	"context"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginhost"
	"net/http/httptest"
	"testing"
)

func TestPublicManagementDispatchEmptyAndNil(t *testing.T) {
	for _, h := range []*pluginhost.Host{pluginhost.New(), nil, {}} {
		h.RegisterManagementRoutes(context.Background(), map[string]struct{}{"GET /v0/management/host": {}})
		for _, path := range []string{"/v0/management/costs", "/v0/resource/plugins/fleet/index.html"} {
			r := httptest.NewRequest("GET", path, nil)
			w := httptest.NewRecorder()
			if h.ServeManagementHTTP(w, r) || h.ServeResourceHTTP(w, r) {
				t.Fatal("empty host handled request")
			}
			if w.Body.Len() != 0 {
				t.Fatal("unhandled request wrote response")
			}
		}
	}
}
