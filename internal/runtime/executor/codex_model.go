package executor

import (
	"net/http"
	"strings"

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

// parseCodexRequestModel also honors the front-proxy marker. The proxy
// normalizes public "-fast" aliases to the base model before the SDK sees the
// request, so the marker is the only remaining signal that priority service
// tier was requested.
func parseCodexRequestModel(model string, headers http.Header) (baseModel string, fast bool) {
	baseModel, fast = parseCodexModel(model)
	if !fast && headers != nil && strings.TrimSpace(headers.Get("X-LLM-Proxy-Codex-Fast")) == "1" {
		fast = true
	}
	return baseModel, fast
}
