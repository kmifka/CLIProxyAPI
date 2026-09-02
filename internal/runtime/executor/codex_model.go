package executor

import (
	"net/http"
	"strings"

	"github.com/tidwall/gjson"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/thinking"
)

// parseCodexModel recognizes the explicit "-fast" catalog aliases. The
// alias is a client-facing name; Codex itself must receive the base model and
// a priority service tier instead.
func parseCodexModel(model string) (baseModel string, fast bool) {
	parsed := thinking.ParseSuffix(model)
	baseModel = parsed.ModelName
	if strings.HasSuffix(strings.ToLower(baseModel), "-fast") {
		baseModel = strings.TrimSuffix(baseModel, baseModel[len(baseModel)-5:])
		fast = true
	}
	return baseModel, fast
}

// parseCodexRequestModel recognizes all three ways a caller can ask for the
// fast path:
//
//   - the "-fast" catalog alias, our own invention
//   - the X-LLM-Proxy-Codex-Fast header, set by the front proxy after it has
//     already normalized the alias away
//   - service_tier: "priority" in the request body, which is what clients that
//     know nothing about our aliases send - Hermes' /fast toggle among them
//
// The third is the one worth having. It is the native OpenAI spelling, every
// client that supports Priority Processing already emits it, and it needs no
// agreement with us about model naming. The aliases exist only because nothing
// read this field before.
func parseCodexRequestModel(model string, headers http.Header, payload []byte) (baseModel string, fast bool) {
	baseModel, fast = parseCodexModel(model)
	if !fast && headers != nil && strings.TrimSpace(headers.Get("X-LLM-Proxy-Codex-Fast")) == "1" {
		fast = true
	}
	if !fast && codexPayloadRequestsPriority(payload) {
		fast = true
	}
	return baseModel, fast
}

// codexPayloadRequestsPriority reports whether the client itself asked for the
// priority service tier.
//
// Note what this does and does not buy: Codex ignores the tier field, so the
// speed-up comes entirely from this being the signal that routes the request
// over the websocket executor. The field is read as intent, not forwarded as
// configuration.
func codexPayloadRequestsPriority(payload []byte) bool {
	if len(payload) == 0 {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(gjson.GetBytes(payload, "service_tier").String()), "priority")
}
