package pluginhost

import (
	"context"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executionregistry"
)

// inferenceScope binds registered audio to the SAME native lifetime registry,
// without inventing a credential/concurrency accounting event.
func inferenceScope(parent context.Context, registry *executionregistry.Registry) (context.Context, func(), error) {
	if registry == nil {
		return nil, nil, executionregistry.ErrRegistryNotAccepting
	}
	pending, err := registry.BeginDispatch()
	if err != nil {
		return nil, nil, err
	}
	defer pending.End()
	scope, err := registry.Install(pending, executionregistry.ScopeSpec{Kind: "audio"})
	if err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	if err = scope.Bind(func() error { cancel(); <-done; return nil }); err != nil {
		cancel()
		scope.End("bind-failed")
		return nil, nil, err
	}
	return ctx, func() { close(done); scope.End("audio-finished"); cancel() }, nil
}
