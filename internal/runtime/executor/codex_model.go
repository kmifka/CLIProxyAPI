package executor

import (
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

