package executor

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v8/sdk/translator"
	"github.com/tidwall/gjson"
)

// Priority request routing and provider-reported response tiers are independent.
func TestCodexNativePriorityWebsocketPreservesUpstreamTier(t *testing.T) {
	for _, tier := range []string{"default", "auto"} {
		for _, stream := range []bool{false, true} {
			t.Run(tier+map[bool]string{false: "/json", true: "/sse"}[stream], func(t *testing.T) {
				upgrader := websocket.Upgrader{}
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if got := r.Header.Get(codexRoutingHintHeader); got != "model=gpt-5.5;tier=priority" {
						t.Errorf("handshake hint = %q", got)
					}
					conn, err := upgrader.Upgrade(w, r, nil)
					if err != nil {
						t.Error(err)
						return
					}
					defer conn.Close()
					_, body, err := conn.ReadMessage()
					if err != nil {
						t.Error(err)
						return
					}
					if gjson.GetBytes(body, "service_tier").String() != "priority" {
						t.Errorf("outbound frame missing priority: %s", body)
					}
					for _, typ := range []string{"response.created", "response.in_progress", "response.completed"} {
						event := `{"type":"` + typ + `","response":{"id":"r","model":"gpt-5.5","status":"completed","service_tier":"` + tier + `","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`
						if err := conn.WriteMessage(websocket.TextMessage, []byte(event)); err != nil {
							t.Error(err)
							return
						}
					}
				}))
				defer server.Close()
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				exec := NewCodexWebsocketsExecutor(&config.Config{})
				req := cliproxyexecutor.Request{Model: "gpt-5.5", Payload: []byte(`{"model":"client-alias","service_tier":"priority","input":"hi"}`)}
				opts := cliproxyexecutor.Options{SourceFormat: sdktranslator.FormatOpenAIResponse, Stream: stream}
				if !stream {
					resp, err := exec.Execute(ctx, codexOAuthTestAuth(server.URL), req, opts)
					if err != nil {
						t.Fatal(err)
					}
					if got := gjson.GetBytes(resp.Payload, "service_tier").String(); got != tier {
						t.Fatalf("tier = %q, want %q", got, tier)
					}
				} else {
					resp, err := exec.ExecuteStream(ctx, codexOAuthTestAuth(server.URL), req, opts)
					if err != nil {
						t.Fatal(err)
					}
					count := 0
					for chunk := range resp.Chunks {
						if chunk.Err != nil {
							t.Fatal(chunk.Err)
						}
						raw := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(chunk.Payload)), "data:"))
						if got := gjson.Get(raw, "response.service_tier"); got.Exists() {
							count++
							if got.String() != tier {
								t.Errorf("tier = %s, want %s", got.Raw, tier)
							}
						}
					}
					if count != 3 {
						t.Fatalf("tier-bearing lifecycle events = %d, want 3", count)
					}
				}
			})
		}
	}
}

func TestCodexNativePriorityPreservesUpstreamTier(t *testing.T) {
	for _, upstreamTier := range []string{"default", "auto"} {
		for _, format := range []sdktranslator.Format{sdktranslator.FormatOpenAI, sdktranslator.FormatOpenAIResponse} {
			for _, mode := range []struct{ stream, buffered bool }{{}, {stream: true}, {stream: true, buffered: true}} {
				stream := mode.stream
				t.Run(upstreamTier+"/"+format.String()+map[bool]string{false: "/json", true: "/sse"}[stream]+map[bool]string{false: "", true: "/buffered"}[mode.buffered], func(t *testing.T) {
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						body, _ := io.ReadAll(r.Body)
						if gjson.GetBytes(body, "service_tier").String() != "priority" || r.Header.Get(codexRoutingHintHeader) != "model=gpt-5.5;tier=priority" {
							t.Error("missing final outbound priority route evidence")
						}
						w.Header().Set("Content-Type", "text/event-stream")
						io.WriteString(w, "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_priority\",\"model\":\"gpt-5.5\",\"service_tier\":\""+upstreamTier+"\"}}\n\n")
						io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\",\"output_index\":0,\"content_index\":0}\n\n")
						io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_priority\",\"object\":\"response\",\"status\":\"completed\",\"model\":\"gpt-5.5\",\"service_tier\":\""+upstreamTier+"\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"ok\"}]}],\"usage\":{\"input_tokens\":1,\"output_tokens\":1,\"total_tokens\":2}}}\n\n")
					}))
					defer server.Close()
					payload := `{"model":"client-alias","service_tier":"priority","messages":[{"role":"user","content":"hi"}]}`
					if format == sdktranslator.FormatOpenAIResponse {
						payload = `{"model":"client-alias","service_tier":"priority","input":"hi"}`
					}
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					cfg := &config.Config{}
					cfg.Codex.StreamBootstrapBuffering = mode.buffered
					exec := NewCodexAutoExecutor(cfg)
					req := cliproxyexecutor.Request{Model: "gpt-5.5", Payload: []byte(payload)}
					opts := cliproxyexecutor.Options{SourceFormat: format, Stream: stream}
					if !stream {
						resp, err := exec.Execute(ctx, codexOAuthTestAuth(server.URL), req, opts)
						if err != nil {
							t.Fatal(err)
						}
						if got := gjson.GetBytes(resp.Payload, "service_tier").String(); got != upstreamTier {
							t.Fatalf("service_tier = %q, want %s; %s", got, upstreamTier, resp.Payload)
						}
					} else {
						resp, err := exec.ExecuteStream(ctx, codexOAuthTestAuth(server.URL), req, opts)
						if err != nil {
							t.Fatal(err)
						}
						count := 0
						for chunk := range resp.Chunks {
							if chunk.Err != nil {
								t.Fatal(chunk.Err)
							}
							raw := strings.TrimSpace(string(chunk.Payload))
							raw = strings.TrimSpace(strings.TrimPrefix(raw, "data:"))
							event := gjson.Parse(raw)
							tier := event.Get("service_tier")
							if format == sdktranslator.FormatOpenAIResponse {
								tier = event.Get("response.service_tier")
							}
							if tier.Exists() {
								count++
								if tier.String() != upstreamTier {
									t.Errorf("tier = %s, want %s; %s", tier.Raw, upstreamTier, raw)
								}
							}
						}
						if count < 2 {
							t.Fatalf("got %d tier-bearing chunks, want created/delta and completed", count)
						}
					}
				})
			}
		}
	}
}
