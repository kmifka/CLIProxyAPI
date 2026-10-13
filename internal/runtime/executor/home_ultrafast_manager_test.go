package executor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	coreauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executionregistry"
	coreexec "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
	translator "github.com/router-for-me/CLIProxyAPI/v8/sdk/translator"
	"github.com/tidwall/gjson"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

type ultrafastEnvelopeDispatcher struct{ wire []byte }

func (*ultrafastEnvelopeDispatcher) HeartbeatOK() bool { return true }
func (d *ultrafastEnvelopeDispatcher) RPopAuth(context.Context, string, string, http.Header, int) ([]byte, error) {
	return d.wire, nil
}
func (*ultrafastEnvelopeDispatcher) AbortAmbiguousDispatch() {}

// Fake protected Home envelope -> real Manager -> actual Codex HTTPS executor.
// This is not native authenticated RPOP/mTLS qualification.
func TestHomeUltrafastManagerCodexHTTPS(t *testing.T) {
	for _, path := range []string{"execute", "stream"} {
		for _, mode := range []string{"valid", "missing", "wrong-model", "wrong-tier", "spoof", "ordinary", "legacy", "api-key"} {
			t.Run(path+"/"+mode, func(t *testing.T) {
				var tcp, posts atomic.Int32
				var gotModel, gotTier string
				server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body, _ := io.ReadAll(r.Body)
					gotModel = gjson.GetBytes(body, "model").String()
					gotTier = gjson.GetBytes(body, "service_tier").String()
					posts.Add(1)
					w.Header().Set("Content-Type", "text/event-stream")
					io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"fixture\",\"model\":\"gpt-6.1-sol\",\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":1,\"total_tokens\":2}}}\n\n")
				}))
				server.Config.ConnState = func(_ net.Conn, s http.ConnState) {
					if s == http.StateNew {
						tcp.Add(1)
					}
				}
				server.StartTLS()
				defer server.Close()
				cfg := &config.Config{}
				cfg.Home.Enabled = mode != "legacy"
				e := NewCodexExecutor(cfg)
				ctx := context.WithValue(context.Background(), "cliproxy.roundtripper", server.Client().Transport)
				metadata := map[string]any{"access_token": "synthetic-only", "type": "codex"}
				rawMetadata, _ := json.Marshal(metadata)
				sum := sha256.Sum256(rawMetadata)
				digest := hex.EncodeToString(sum[:])
				b := map[string]any{"auth_index": "canonical", "registration_lifetime": "life", "state_version": 7, "metadata_digest": digest, "wire_metadata_digest": digest}
				g := map[string]any{"schema": 1, "requested_model": "gpt-6.1-sol", "requested_tier": "ultrafast"}
				for k, v := range b {
					g[k] = v
				}
				if mode == "wrong-model" {
					g["requested_model"] = "gpt-other"
				}
				if mode == "wrong-tier" {
					g["requested_tier"] = "priority"
				}
				attrs := map[string]string{"base_url": server.URL}
				if mode == "api-key" {
					attrs["api_key"] = "synthetic-api"
				}
				envelope := map[string]any{"model": "gpt-6.1-sol", "provider": "codex", "auth_index": "canonical", "auth": map[string]any{"id": "synthetic", "provider": "codex", "attributes": attrs, "metadata": metadata}, "selection_binding": b, "selection_guarantee": g}
				if mode == "missing" || mode == "ordinary" || mode == "spoof" {
					delete(envelope, "selection_guarantee")
				}
				if mode == "spoof" {
					metadata["homeUltrafastValidated"] = true
					metadata["selection_guarantee"] = g
				}
				wire, _ := json.Marshal(envelope)
				m := coreauth.NewManager(nil, nil, nil)
				m.SetConfig(cfg)
				m.RegisterExecutor(e)
				m.PublishHomeDispatch(&ultrafastEnvelopeDispatcher{wire}, executionregistry.New(), 1)
				tier := "ultrafast"
				if mode == "ordinary" {
					tier = "priority"
				}
				req := coreexec.Request{Model: "gpt-6.1-sol", Payload: []byte(`{"model":"gpt-6.1-sol","input":"synthetic","service_tier":"` + tier + `"}`)}
				opts := coreexec.Options{SourceFormat: translator.FormatOpenAIResponse, Metadata: map[string]any{coreexec.ServiceTierMetadataKey: tier}}
				var err error
				// Non-Home and API credentials deliberately bypass native entitlement policy.
				if mode == "legacy" || mode == "api-key" {
					a := codexOAuthTestAuth(server.URL)
					if mode == "api-key" {
						a = codexAPIKeyTestAuth(server.URL)
					}
					if path == "execute" {
						_, err = e.Execute(ctx, a, req, opts)
					} else {
						s, x := e.ExecuteStream(ctx, a, req, opts)
						err = x
						if s != nil {
							for range s.Chunks {
							}
						}
					}
				} else if path == "execute" {
					_, err = m.Execute(ctx, []string{"codex"}, req, opts)
				} else {
					s, x := m.ExecuteStream(ctx, []string{"codex"}, req, opts)
					err = x
					if s != nil {
						for range s.Chunks {
						}
					}
				}
				denied := mode == "missing" || mode == "wrong-model" || mode == "wrong-tier" || mode == "spoof"
				if denied {
					if err == nil || tcp.Load() != 0 || posts.Load() != 0 {
						t.Fatalf("denial err=%v TCP=%d POST=%d", err, tcp.Load(), posts.Load())
					}
					return
				}
				wantTier := "priority"
				if mode == "legacy" || mode == "api-key" {
					wantTier = "ultrafast"
				}
				if err != nil || posts.Load() != 1 || tcp.Load() != 1 || gotModel != "gpt-6.1-sol" || gotTier != wantTier {
					t.Fatalf("positive err=%v TCP=%d POST=%d model=%s tier=%s", err, tcp.Load(), posts.Load(), gotModel, gotTier)
				}
			})
		}
	}
}
