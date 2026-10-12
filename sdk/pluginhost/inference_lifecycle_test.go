package pluginhost

import (
	"context"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executionregistry"
	"testing"
	"time"
)

func TestInferenceScopeForcedDrain(t *testing.T) {
	registry := executionregistry.New()
	ctx, finish, err := inferenceScope(context.Background(), registry)
	if err != nil {
		t.Fatal(err)
	}
	stopped := make(chan struct{})
	go func() { <-ctx.Done(); finish(); close(stopped) }()
	deadline, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err = registry.Drain(deadline); err != nil {
		t.Fatal(err)
	}
	<-stopped
	freeze := registry.FreezeInFlight(time.Now())
	if len(freeze.Executions) != 0 {
		t.Fatal("scope leak")
	}
}
