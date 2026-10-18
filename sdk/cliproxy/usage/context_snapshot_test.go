package usage

import (
	"context"
	internallogging "github.com/router-for-me/CLIProxyAPI/v8/internal/logging"
	"net/http"
	"testing"
	"time"
)

type custodyCapture func(context.Context, Record)

func (f custodyCapture) HandleUsage(c context.Context, r Record) { f(c, r) }

func TestQueuedContextMetadataSnapshot(t *testing.T) {
	manager := NewManager(1)
	defer manager.Stop()
	ready, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	manager.Register(custodyCapture(func(context.Context, Record) { close(ready); <-release }))
	ctx := context.WithValue(context.Background(), "gin", new(int))
	ctx = WithHTTPRequestOwnership(ctx)
	ctx = WithOriginalRequestID(ctx, "platform-id")
	ctx = WithExecutionRequestID(ctx, "execution")
	ctx = WithTraceID(ctx, "trace")
	ctx = WithRequestedModelAlias(ctx, "alias")
	ctx = WithReasoningEffort(ctx, "high")
	ctx = WithServiceTier(ctx, "ultrafast")
	ctx = WithStream(ctx, true)
	ctx = WithGenerate(ctx, false)
	ctx = internallogging.WithRequestID(ctx, "logging-id")
	ctx = internallogging.WithEndpoint(ctx, "POST /v1/responses")
	meta := internallogging.ClientRequestMetadata{ClientIP: "127.0.0.2", SessionID: "session", ParentSessionID: "parent", UserAgent: "original"}
	ctx = internallogging.WithClientRequestMetadata(ctx, meta)
	ctx = internallogging.WithResponseStatusHolder(ctx)
	internallogging.SetResponseStatus(ctx, 201)
	ctx = internallogging.WithResponseHeadersHolder(ctx)
	internallogging.SetResponseHeaders(ctx, http.Header{"X-Fixture": []string{"original"}})
	manager.Register(custodyCapture(func(c context.Context, r Record) {
		defer close(done)
		if c.Value("gin") != nil {
			t.Error("pooled parent retained")
		}
		if r.OriginalRequestID != "platform-id" || r.RequestID != "execution" || r.TraceID != "trace" {
			t.Errorf("wrong identity: %+v", r)
		}
		if internallogging.GetRequestID(c) != "logging-id" || internallogging.GetEndpoint(c) != "POST /v1/responses" || internallogging.GetClientRequestMetadata(c) != meta || internallogging.GetResponseStatus(c) != 201 || internallogging.GetResponseHeaders(c).Get("X-Fixture") != "original" {
			t.Error("logging metadata not snapshotted")
		}
		if RequestedModelAliasFromContext(c) != "alias" || ReasoningEffortFromContext(c) != "high" || ServiceTierFromContext(c) != "ultrafast" || !StreamFromContext(c) || GenerateFromContext(c) {
			t.Error("usage metadata lost")
		}
	}))
	manager.Publish(ctx, Record{})
	<-ready
	internallogging.SetResponseStatus(ctx, 500)
	internallogging.SetResponseHeaders(ctx, http.Header{"X-Fixture": []string{"next"}})
	close(release)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("dispatch timeout")
	}
}
