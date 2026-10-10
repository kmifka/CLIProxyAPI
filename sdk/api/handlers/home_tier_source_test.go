package handlers

import (
	"context"
	coreexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
	sdkconfig "github.com/router-for-me/CLIProxyAPI/v8/sdk/config"
	"net/http"
	"testing"
)

func TestPublicAdapterTierFromBody(t *testing.T) {
	for _, tier := range []string{"priority", "ultrafast", "auto"} {
		e := &modelExecutionCaptureExecutor{}
		h := newModelExecutionHandler(t, "tier-source", e, &sdkconfig.SDKConfig{})
		body := []byte(`{"model":"tier-source"}`)
		if tier != "auto" {
			body = []byte(`{"model":"tier-source","service_tier":"` + tier + `"}`)
		}
		_, err := h.ExecuteProtocolWithAuthManager(context.Background(), ProtocolExecutionRequest{EntryProtocol: "openai", ExitProtocol: "openai", Model: "tier-source", Body: body, Headers: http.Header{"X-Home-Service-Tier": []string{"spoofed"}}})
		if err != nil {
			t.Fatal(err)
		}
		_, opts := e.captured()
		if opts.Metadata[coreexecutor.ServiceTierMetadataKey] != tier {
			t.Fatalf("body-derived tier %v want %s", opts.Metadata, tier)
		}
		if string(opts.OriginalRequest) != string(body) {
			t.Fatal("source body changed")
		}
	}
}
