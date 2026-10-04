package helps

import (
	"net/http"
	"strings"

	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// CodexPriorityRouteSelected describes the selected execution attempt, not the
// original client request. Check the final body and headers after payload and
// operator overrides. API-key and non-Codex routes must retain upstream tiers.
func CodexPriorityRouteSelected(auth *cliproxyauth.Auth, headers http.Header, body []byte) bool {
	if auth == nil || auth.Provider != "codex" || auth.AuthKind() != cliproxyauth.AuthKindOAuth || strings.TrimSpace(auth.Attributes["api_key"]) != "" {
		return false
	}
	model := gjson.GetBytes(body, "model")
	return model.Type == gjson.String && model.String() != "" &&
		gjson.GetBytes(body, "service_tier").String() == "priority" &&
		headers.Get("X-Codex-Routing-Hint") == "model="+model.String()+";tier=priority"
}

// ReportCodexPriorityRoute corrects the ChatGPT backend's default/omitted tier
// for a proven outbound OAuth priority attempt before protocol translation.
// This reports proxy route selection, not an upstream billing/latency guarantee.
// Explicit alternative upstream tiers and error events are left untouched.
func ReportCodexPriorityRoute(event []byte, selected bool) []byte {
	if !selected {
		return event
	}
	root := gjson.ParseBytes(event)
	switch root.Get("type").String() {
	case "response.created", "response.in_progress", "response.completed", "response.incomplete", "response.done":
	default:
		return event
	}
	if !root.Get("response").IsObject() {
		return event
	}
	tier := root.Get("response.service_tier")
	if tier.Exists() && (tier.Type != gjson.String || (tier.String() != "default" && tier.String() != "")) {
		return event
	}
	out, err := sjson.SetBytes(event, "response.service_tier", "priority")
	if err != nil {
		return event
	}
	return out
}
