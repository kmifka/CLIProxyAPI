package executor

import (
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
	"github.com/tidwall/gjson"

	"net/http"
	"strings"
)

func (e *CodexExecutor) homeUltrafastEnabled() bool {
	return e != nil && e.cfg != nil && e.cfg.Home.Enabled
}

// Check the incoming request before alternate dispatch, then check the final
// translated body in cacheHelper. The private Auth marker survives Clone; no
// header, metadata map, context key or callback grants admission.
func (e *CodexExecutor) prepareHomeUltrafast(auth *cliproxyauth.Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (cliproxyexecutor.Request, error) {
	if !e.homeUltrafastEnabled() || (auth != nil && strings.TrimSpace(auth.Attributes["api_key"]) != "") {
		return req, nil
	}
	tier := strings.ToLower(strings.TrimSpace(gjson.GetBytes(req.Payload, "service_tier").String()))
	metaTier, _ := opts.Metadata[cliproxyexecutor.ServiceTierMetadataKey].(string)
	originalTier := gjson.GetBytes(opts.OriginalRequest, "service_tier").String()
	explicit := tier == "ultrafast" || strings.EqualFold(strings.TrimSpace(metaTier), "ultrafast") || strings.EqualFold(strings.TrimSpace(originalTier), "ultrafast")
	if !explicit {
		return req, nil
	}
	if isCodexOpenAIImageRequest(opts) {
		return req, statusErr{code: http.StatusServiceUnavailable, msg: "Home ultrafast image dispatch is not qualified"}
	}
	if !auth.HomeUltrafastValidatedFor(req.Model) || auth.Provider != "codex" || strings.TrimSpace(auth.Attributes["api_key"]) != "" {
		return req, statusErr{code: http.StatusServiceUnavailable, msg: "Home ultrafast selection guarantee unavailable"}
	}
	if model := gjson.GetBytes(req.Payload, "model").String(); model != "" && model != req.Model {
		return req, statusErr{code: http.StatusServiceUnavailable, msg: "Home ultrafast exact model mismatch"}
	}
	if tier != "" && tier != "ultrafast" {
		return req, statusErr{code: http.StatusServiceUnavailable, msg: "Home ultrafast tier mismatch"}
	}
	// Translation happens later; retain the explicit tier until the final body
	// guard so configuration cannot silently downgrade an admitted request.
	return req, nil
}

func (e *CodexExecutor) finalizeHomeUltrafast(auth *cliproxyauth.Auth, model string, body []byte) ([]byte, error) {
	if !e.homeUltrafastEnabled() || (auth != nil && strings.TrimSpace(auth.Attributes["api_key"]) != "") {
		return body, nil
	}
	tier := strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, "service_tier").String()))
	if tier != "ultrafast" && !auth.HomeUltrafastValidated() {
		return body, nil
	}
	if !auth.HomeUltrafastValidatedFor(model) || auth.Provider != "codex" || gjson.GetBytes(body, "model").String() != model || tier != "ultrafast" {
		return nil, statusErr{code: http.StatusServiceUnavailable, msg: "Home ultrafast final model/tier guarantee mismatch"}
	}
	// An admitted entitlement must reach the provider unchanged. A successful
	// priority response cannot satisfy an explicit ultrafast request.
	return body, nil
}
