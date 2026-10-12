package pluginhost

import (
	"context"
	"errors"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

type RegisteredInference struct {
	ID     string
	Routes []pluginapi.InferenceRoute
}

func (h *Host) RegisteredInference(ctx context.Context) []RegisteredInference {
	var out []RegisteredInference
	for _, r := range h.activeRecords() {
		p := r.plugin.Capabilities.Inference
		if p == nil || h.isPluginFused(r.id) {
			continue
		}
		routes, e := p.InferenceRoutes(ctx)
		if e != nil {
			continue
		}
		valid := []pluginapi.InferenceRoute{}
		for _, v := range routes {
			if pluginapi.ValidInferenceRoute(v) {
				valid = append(valid, v)
			}
		}
		if len(valid) > 0 {
			out = append(out, RegisteredInference{r.id, valid})
		}
	}
	return out
}
func (h *Host) FindInference(ctx context.Context, method, path string) (pluginapi.InferencePlugin, error) {
	if !pluginapi.ValidInferenceRoute(pluginapi.InferenceRoute{Method: method, Path: path, AuthRequired: true}) {
		return nil, errors.New("inference route denied")
	}
	for _, r := range h.activeRecords() {
		p := r.plugin.Capabilities.Inference
		if p == nil || h.isPluginFused(r.id) {
			continue
		}
		routes, e := p.InferenceRoutes(ctx)
		if e != nil {
			continue
		}
		for _, v := range routes {
			if pluginapi.ValidInferenceRoute(v) && v.Method == method && v.Path == path {
				return p, nil
			}
		}
	}
	return nil, errors.New("inference route unavailable")
}
