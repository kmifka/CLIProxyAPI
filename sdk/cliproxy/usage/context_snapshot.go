package usage

import (
	"context"
	internallogging "github.com/router-for-me/CLIProxyAPI/v8/internal/logging"
)

type originalRequestIDContextKey struct{}
type httpRequestOwnershipContextKey struct{}

// WithHTTPRequestOwnership marks an owned HTTP request synchronously, before its
// context reaches executors. Publish will copy the documented usage/logging
// metadata into a parent-free context before queueing. Arbitrary caller values
// are not part of that HTTP transfer; direct SDK contexts remain untouched.
func WithHTTPRequestOwnership(ctx context.Context) context.Context {
	return context.WithValue(ctx, httpRequestOwnershipContextKey{}, true)
}

func hasHTTPRequestOwnership(ctx context.Context) bool {
	owned, _ := ctx.Value(httpRequestOwnershipContextKey{}).(bool)
	return owned
}

// WithOriginalRequestID captures platform correlation while the HTTP request is owned.
func WithOriginalRequestID(ctx context.Context, id string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, originalRequestIDContextKey{}, id)
}

func OriginalRequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(originalRequestIDContextKey{}).(string)
	return id
}

// Construct a metadata-only context rather than retaining a Gin context through
// a parent chain. Logging's status/header holders are concurrency-safe, but copy
// their observed values here so each queued record has its own immutable view.
func snapshotUsageContext(ctx context.Context) context.Context {
	snapshot := context.Background()
	snapshot = internallogging.WithRequestID(snapshot, internallogging.GetRequestID(ctx))
	snapshot = internallogging.WithEndpoint(snapshot, internallogging.GetEndpoint(ctx))
	snapshot = internallogging.WithClientRequestMetadata(snapshot, internallogging.GetClientRequestMetadata(ctx))
	snapshot = internallogging.WithResponseStatusHolder(snapshot)
	internallogging.SetResponseStatus(snapshot, internallogging.GetResponseStatus(ctx))
	snapshot = internallogging.WithResponseHeadersHolder(snapshot)
	internallogging.SetResponseHeaders(snapshot, internallogging.GetResponseHeaders(ctx))
	snapshot = WithOriginalRequestID(snapshot, OriginalRequestIDFromContext(ctx))
	snapshot = WithExecutionRequestID(snapshot, ExecutionRequestIDFromContext(ctx))
	snapshot = WithTraceID(snapshot, TraceIDFromContext(ctx))
	snapshot = WithRequestedModelAlias(snapshot, RequestedModelAliasFromContext(ctx))
	snapshot = WithReasoningEffort(snapshot, ReasoningEffortFromContext(ctx))
	snapshot = WithServiceTier(snapshot, ServiceTierFromContext(ctx))
	snapshot = WithGenerate(snapshot, GenerateFromContext(ctx))
	return WithStream(snapshot, StreamFromContext(ctx))
}
