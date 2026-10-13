package executor

import (
	"context"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v8/sdk/translator"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestHomeUltrafastMissingVerifierPreProvider(t *testing.T) {
	for _, path := range []string{"execute", "stream", "compact", "count", "http"} {
		t.Run(path, func(t *testing.T) {
			var accepts, posts atomic.Int32
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				posts.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"synthetic\",\"object\":\"response\",\"status\":\"completed\",\"model\":\"gpt-6.1-sol\",\"output\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":1,\"total_tokens\":2}}}\n\n")
			}))
			server.Config.ConnState = func(_ net.Conn, s http.ConnState) {
				if s == http.StateNew {
					accepts.Add(1)
				}
			}
			server.StartTLS()
			defer server.Close()
			ctx := context.WithValue(context.Background(), "cliproxy.roundtripper", server.Client().Transport)
			a := codexOAuthTestAuth(server.URL)
			a.Metadata["homeUltrafastValidated"] = true
			a.Attributes["home_ultrafast_validated"] = "true"
			cfg := &config.Config{}
			cfg.Home.Enabled = true
			e := NewCodexExecutor(cfg)
			payload := []byte(`{"model":"gpt-6.1-sol","input":"synthetic","service_tier":"ultrafast"}`)
			req := cliproxyexecutor.Request{Model: "gpt-6.1-sol", Payload: payload}
			opts := cliproxyexecutor.Options{SourceFormat: sdktranslator.FormatOpenAIResponse, Headers: http.Header{"X-Home-Ultrafast-Validated": []string{"true"}}, Metadata: map[string]any{cliproxyexecutor.ServiceTierMetadataKey: "ultrafast", "homeUltrafastValidated": true}}
			var err error
			switch path {
			case "execute":
				_, err = e.Execute(ctx, a, req, opts)
			case "stream":
				var s *cliproxyexecutor.StreamResult
				s, err = e.ExecuteStream(ctx, a, req, opts)
				if s != nil {
					for range s.Chunks {
					}
				}
			case "compact":
				opts.Alt = "responses/compact"
				_, err = e.Execute(ctx, a, req, opts)
			case "count":
				_, err = e.CountTokens(ctx, a, req, opts)
			case "http":
				r, _ := http.NewRequest("POST", server.URL+"/responses", strings.NewReader(string(payload)))
				_, err = e.HttpRequest(ctx, a, r)
			}
			if err == nil || accepts.Load() != 0 || posts.Load() != 0 {
				t.Fatalf("missing verifier did not fail preprovider: error=%v TCP_accepts=%d POSTs=%d", err, accepts.Load(), posts.Load())
			}
		})
	}
}
