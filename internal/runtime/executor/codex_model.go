package executor

import (
	"net/http"
	"strings"

	"github.com/tidwall/gjson"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/thinking"
)

// parseCodexModel strips any thinking suffix from the requested model.
//
// It used to also recognise "<model>-fast" catalog aliases. Those are gone: the
// alias was a name only this proxy understood, so every client had to be taught
// it, and clients that already spoke the native OpenAI spelling got nothing.
// Speed is now asked for with service_tier in the request body, which is what
// parseCodexRequestModel reads.
func parseCodexModel(model string) (baseModel string) {
	return thinking.ParseSuffix(model).ModelName
}

// parseCodexRequestModel reports whether the caller asked for the fast path.
//
// The signal is service_tier: "priority" in the request body - the native
// OpenAI spelling, which every client supporting Priority Processing already
// emits and which needs no agreement with us about model naming.
func parseCodexRequestModel(model string, headers http.Header, payload []byte) (baseModel string, fast bool) {
	baseModel = parseCodexModel(model)
	// The header is kept as an escape hatch for a front proxy that has already
	// consumed the body and cannot pass the field through.
	if headers != nil && strings.TrimSpace(headers.Get("X-LLM-Proxy-Codex-Fast")) == "1" {
		return baseModel, true
	}
	return baseModel, codexPayloadRequestsPriority(payload)
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
