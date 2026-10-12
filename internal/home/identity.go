package home

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
)

// RequestIdentity authenticates key existence only. It grants no method, path,
// model, channel or capability permission. Principal is sensitive key material
// at Home5687: callers MUST NOT log or publish it.
type RequestIdentity struct {
	Authenticated bool   `json:"authenticated"`
	Provider      string `json:"provider,omitempty"`
	principal     string
}

// MatchesPrincipal is the only public principal operation. It discloses no key
// or derived identifier; callers must already possess the expected native key.
func (i *RequestIdentity) MatchesPrincipal(expected string) bool {
	return i != nil && expected != "" && subtle.ConstantTimeCompare([]byte(i.principal), []byte(expected)) == 1
}
func (i RequestIdentity) String() string   { return "HomeIdentity[redacted]" }
func (i RequestIdentity) GoString() string { return i.String() }

type IdentityError struct {
	Code   string
	Status int
}

func (e *IdentityError) Error() string { return "home identity validation: " + e.Code }

func decodeIdentityResponse(raw []byte) (*RequestIdentity, error) {
	var wire struct {
		Authenticated bool   `json:"authenticated"`
		Provider      string `json:"provider"`
		Principal     string `json:"principal"`
		Error         *struct {
			Type string `json:"type"`
		} `json:"error,omitempty"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &wire) != nil {
		return nil, &IdentityError{Code: "invalid_response", Status: 503}
	}
	if wire.Error != nil {
		status := 503
		switch wire.Error.Type {
		case "no_credentials", "invalid_credential":
			status = 401
		}
		return nil, &IdentityError{Code: wire.Error.Type, Status: status}
	}
	if !wire.Authenticated || wire.Provider == "" || wire.Principal == "" {
		return nil, &IdentityError{Code: "invalid_response", Status: 503}
	}
	return &RequestIdentity{Authenticated: true, Provider: wire.Provider, principal: wire.Principal}, nil
}

// ValidateRequestIdentity uses the enrolled native command client and Home's
// legacy auth-validate RPC. Home intentionally authenticates canonical GET /;
// do NOT send audio method/path/capability or call this route authorization.
// It performs no inference dispatch and returns no provider credentials.
func (c *Client) ValidateRequestIdentity(ctx context.Context, headers http.Header) (*RequestIdentity, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cmd, err := c.commandClient()
	if err != nil {
		return nil, &IdentityError{Code: "auth_unavailable", Status: 503}
	}
	payload, err := json.Marshal(struct {
		Type    string            `json:"type"`
		Headers map[string]string `json:"headers"`
	}{Type: "auth-validate", Headers: headersToLowerMap(headers)})
	if err != nil {
		return nil, fmt.Errorf("identity request: %w", err)
	}
	raw, err := cmd.RPop(ctx, string(payload)).Bytes()
	if err != nil {
		return nil, &IdentityError{Code: "auth_unavailable", Status: 503}
	}
	return decodeIdentityResponse(raw)
}
