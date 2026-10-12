package api

import (
	"github.com/router-for-me/CLIProxyAPI/v8/internal/home"
	"net/http"
)

type RegisteredRequestIdentity = home.RequestIdentity
type RegisteredRequestError = home.IdentityError

func RegisteredAudioRoute(method, path string) bool {
	if method != http.MethodPost {
		return false
	}
	switch path {
	case "/v1/audio/transcriptions", "/v1/audio/translations", "/v1/audio/speech":
		return true
	}
	return false
}

// AuthenticateRegisteredRequest requires original-route policy BEFORE native
// identity authentication, then binds the sensitive native principal without
// exposing it. The policy must consume its nonce exactly once, not in bind.
// This is identity-only Home validation, not Home capability authorization.
func AuthenticateRegisteredRequest(r *http.Request, authorize func(*http.Request) error, bind func(*RegisteredRequestIdentity) error) (*RegisteredRequestIdentity, error) {
	if r == nil || r.URL == nil || !RegisteredAudioRoute(r.Method, r.URL.Path) || r.URL.RawQuery != "" || r.URL.RawPath != "" {
		return nil, &RegisteredRequestError{Code: "unregistered_route", Status: 403}
	}
	return authenticateRegisteredIdentity(r, authorize, bind)
}

// authenticateRegisteredIdentity is shared only after each route's own fence.
func authenticateRegisteredIdentity(r *http.Request, authorize func(*http.Request) error, bind func(*RegisteredRequestIdentity) error) (*RegisteredRequestIdentity, error) {
	if authorize == nil || bind == nil {
		return nil, &RegisteredRequestError{Code: "route_policy_unavailable", Status: 503}
	}
	if err := authorize(r); err != nil {
		return nil, err
	}
	identity, err := home.Current().ValidateRequestIdentity(r.Context(), r.Header)
	if err != nil {
		return nil, err
	}
	if err = bind(identity); err != nil {
		return nil, err
	}
	return identity, nil
}
