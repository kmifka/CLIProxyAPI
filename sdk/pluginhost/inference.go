package pluginhost

import (
	"context"
	"errors"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executionregistry"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
	"net/http"
)

// ServeInferenceWithRegistry attaches cancellation and resource close to the
// native manager's current execution registry. Nil/draining registries fail closed.
func (h *Host) ServeInferenceWithRegistry(w http.ResponseWriter, r *http.Request, admit func(*http.Request) error, registry *executionregistry.Registry) error {
	ctx, finish, err := inferenceScope(r.Context(), registry)
	if err != nil {
		return err
	}
	defer finish()
	return h.ServeInference(w, r.WithContext(ctx), admit)
}

// InferenceRoutes returns safe metadata from active native registrations.
func (h *Host) InferenceRoutes(ctx context.Context) map[string][]pluginapi.InferenceRoute {
	out := map[string][]pluginapi.InferenceRoute{}
	for _, rec := range h.inner.RegisteredInference(ctx) {
		out[rec.ID] = rec.Routes
	}
	return out
}

// ServeInference requires a host-owned admission callback. It is invoked before
// any plugin request handling and cannot expose its identity to the plugin.
func (h *Host) ServeInference(w http.ResponseWriter, r *http.Request, admit func(*http.Request) error) error {
	if admit == nil {
		return errors.New("inference admission required")
	}
	if r.URL.RawQuery != "" || r.ContentLength > pluginapi.InferenceMaxBody {
		return errors.New("invalid inference request")
	}
	p, err := h.inner.FindInference(r.Context(), r.Method, r.URL.Path)
	if err != nil {
		return err
	}
	if err = admit(r); err != nil {
		return err
	}
	headers := http.Header{}
	for _, k := range []string{"Content-Type", "Accept", "X-Request-Id"} {
		if v := r.Header.Values(k); len(v) > 0 {
			headers[k] = append([]string(nil), v...)
		}
	}
	return p.ServeInference(r.Context(), pluginapi.InferenceRequest{Method: r.Method, Path: r.URL.Path, ContentLength: r.ContentLength, Headers: headers}, r.Body, w)
}
