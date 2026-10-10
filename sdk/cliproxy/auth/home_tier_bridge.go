package auth

import (
	"context"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
	"net/http"
	"strings"
)

// Header is trusted only on the authenticated node -> Home connection. All
// downstream caller variants are removed; public handlers derive metadata from
// the parsed request body before this boundary.
func homeTierRequiresMembership(opts executor.Options) bool {
	tier, _ := opts.Metadata[executor.ServiceTierMetadataKey].(string)
	return tier != "" && tier != "auto" && tier != "default" && tier != "standard"
}

func trustedHomeDispatchHeaders(ctx context.Context, opts executor.Options) http.Header {
	out := homeDispatchHeaders(ctx, opts.Headers).Clone()
	if out == nil {
		out = http.Header{}
	}
	for key := range out {
		if strings.EqualFold(key, "X-Home-Service-Tier") {
			delete(out, key)
		}
	}
	if tier, ok := opts.Metadata[executor.ServiceTierMetadataKey].(string); ok && tier != "" {
		out.Set("X-Home-Service-Tier", tier)
	}
	return out
}
