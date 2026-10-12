package api

import (
	"net/http"
	"net/url"
	"strings"
)

// RegisteredRealtimeRoute reports the exact registered GET WebSocket paths.
// It does not authorize a model or validate query/body/identity.
func RegisteredRealtimeRoute(method, path string) bool {
	return method == http.MethodGet && (path == "/v1/realtime" || path == "/live/gemini")
}

// AuthenticateRegisteredRealtimeRequest authenticates an extension without
// inference dispatch. authorize must approve the original route and model,
// verify the empty-body grant, audience and durable one-use nonce before Home;
// bind must compare its expected opaque principal using MatchesPrincipal.
// Native Home validation is identity-only, not model/capability RBAC. No
// provider permission claims are inferred. The request is never rewritten.
func AuthenticateRegisteredRealtimeRequest(r *http.Request, authorize func(*http.Request) error, bind func(*RegisteredRequestIdentity) error) (*RegisteredRequestIdentity, error) {
	if r == nil || r.URL == nil || !RegisteredRealtimeRoute(r.Method, r.URL.Path) || r.URL.RawPath != "" || r.URL.Fragment != "" || r.URL.RawFragment != "" || r.URL.User != nil {
		return nil, &RegisteredRequestError{Code: "unregistered_route", Status: 403}
	}
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(q) != 1 || len(q["model"]) != 1 || strings.TrimSpace(q.Get("model")) == "" || strings.Contains(r.URL.RawQuery, "&") || !strings.HasPrefix(r.URL.RawQuery, "model=") {
		return nil, &RegisteredRequestError{Code: "unregistered_query", Status: 400}
	}
	// RequestURI is retained by HTTP servers; reject divergent/invalid targets
	// rather than validating URL while the grant observes another request target.
	if r.RequestURI != "" {
		parsed, e := url.ParseRequestURI(r.RequestURI)
		if e != nil || parsed.Path != r.URL.Path || parsed.RawPath != "" || parsed.RawQuery != r.URL.RawQuery || strings.Contains(r.RequestURI, "#") {
			return nil, &RegisteredRequestError{Code: "unregistered_route", Status: 403}
		}
	}
	if r.ContentLength > 0 || len(r.TransferEncoding) > 0 || (r.Body != nil && r.Body != http.NoBody) {
		return nil, &RegisteredRequestError{Code: "unregistered_body", Status: 400}
	}
	return authenticateRegisteredIdentity(r, authorize, bind)
}
