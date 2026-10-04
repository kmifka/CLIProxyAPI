package executor

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/runtime/executor/helps"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v8/sdk/translator"
	"github.com/tidwall/gjson"
)

func TestCodexPriorityResponseNegativeRoutes(t *testing.T) {
	for _, tc := range []struct {
		name, payload, hint, upstream, want string
		apiKey                              bool
		override                            string
	}{
		{name: "payload removes priority selection", payload: `{"model":"gpt-5.5","service_tier":"priority","input":"hi"}`, override: "default", upstream: "default", want: "default"},
		{name: "payload selects priority without client request", payload: `{"model":"gpt-5.5","input":"hi"}`, override: "priority", upstream: "default", want: "priority"},
		{name: "normal OAuth", payload: `{"model":"gpt-5.5","input":"hi"}`, upstream: "default", want: "default"},
		{name: "API key priority fallback", payload: `{"model":"gpt-5.5","service_tier":"priority","input":"hi"}`, apiKey: true, upstream: "default", want: "default"},
		{name: "operator selects default", payload: `{"model":"gpt-5.5","service_tier":"priority","input":"hi"}`, hint: "model=gpt-5.5;tier=default", upstream: "default", want: "default"},
		{name: "operator selects different model", payload: `{"model":"gpt-5.5","service_tier":"priority","input":"hi"}`, hint: "model=other;tier=priority", upstream: "default", want: "default"},
		{name: "upstream explicit flex fallback", payload: `{"model":"gpt-5.5","service_tier":"priority","input":"hi"}`, upstream: "flex", want: "flex"},
		{name: "ultrafast unchanged", payload: `{"model":"gpt-5.5","service_tier":"ultrafast","input":"hi"}`, upstream: "default", want: "default"},
	} {
		for _, stream := range []bool{false, true} {
			t.Run(tc.name+map[bool]string{false: "/json", true: "/sse"}[stream], func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					io.Copy(io.Discard, r.Body)
					io.WriteString(w, `data: {"type":"response.completed","response":{"id":"r","model":"gpt-5.5","service_tier":"`+tc.upstream+`","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`+"\n\n")
				}))
				defer server.Close()
				auth := codexOAuthTestAuth(server.URL)
				if tc.apiKey {
					auth = codexAPIKeyTestAuth(server.URL)
				}
				if tc.hint != "" {
					auth.Attributes["header:X-Codex-Routing-Hint"] = tc.hint
				}
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				cfg := &config.Config{}
				if tc.override != "" {
					cfg.Payload = config.PayloadConfig{Override: []config.PayloadRule{{Models: []config.PayloadModelRule{{Name: "gpt-5.5"}}, Params: map[string]any{"service_tier": tc.override}}}}
				}
				exec := NewCodexAutoExecutor(cfg)
				req := cliproxyexecutor.Request{Model: "gpt-5.5", Payload: []byte(tc.payload)}
				opts := cliproxyexecutor.Options{SourceFormat: sdktranslator.FormatOpenAIResponse, Stream: stream}
				var output []byte
				if !stream {
					resp, err := exec.Execute(ctx, auth, req, opts)
					if err != nil {
						t.Fatal(err)
					}
					output = resp.Payload
				} else {
					resp, err := exec.ExecuteStream(ctx, auth, req, opts)
					if err != nil {
						t.Fatal(err)
					}
					for chunk := range resp.Chunks {
						if chunk.Err != nil {
							t.Fatal(chunk.Err)
						}
						if strings.Contains(string(chunk.Payload), "response.completed") {
							raw := strings.TrimSpace(strings.TrimPrefix(string(chunk.Payload), "data:"))
							output = []byte(gjson.Get(raw, "response").Raw)
						}
					}
				}
				if got := gjson.GetBytes(output, "service_tier").String(); got != tc.want {
					t.Fatalf("got tier %q, want %q; %s", got, tc.want, output)
				}
			})
		}
	}
}

func TestCodexPriorityRouteEvidenceAndEventPreservation(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","service_tier":"priority"}`)
	headers := http.Header{"X-Codex-Routing-Hint": {"model=gpt-5.5;tier=priority"}}
	auth := codexOAuthTestAuth("")
	if !helps.CodexPriorityRouteSelected(auth, headers, body) {
		t.Fatal("OAuth final priority evidence not recognized")
	}
	for _, invalid := range []string{`{"model":"gpt-5.5"}`, `{"model":"gpt-5.5","service_tier":"default"}`, `{"model":"other","service_tier":"priority"}`} {
		if helps.CodexPriorityRouteSelected(auth, headers, []byte(invalid)) {
			t.Fatalf("false evidence: %s", invalid)
		}
	}
	if helps.CodexPriorityRouteSelected(nil, headers, body) {
		t.Fatal("nil auth trusted")
	}
	auth.Provider = "openrouter"
	if helps.CodexPriorityRouteSelected(auth, headers, body) {
		t.Fatal("fallback provider trusted")
	}
	for _, event := range []string{
		`{"type":"error","response":{"service_tier":"default"}}`,
		`{"type":"response.failed","response":{"service_tier":"default"}}`,
		`{"type":"response.created","response":{"service_tier":"scale"}}`,
		`{"type":"response.completed","response":{"service_tier":null}}`,
		`{"type":"response.created","response":{"service_tier":42}}`,
		`{"type":"response.output_text.delta","delta":"service_tier default"}`,
		`{"type":"response.created"}`,
		`invalid`,
	} {
		if got := helps.ReportCodexPriorityRoute([]byte(event), true); string(got) != event {
			t.Fatalf("unexpected rewrite: %s -> %s", event, got)
		}
	}
	for _, typ := range []string{"response.created", "response.in_progress", "response.completed", "response.incomplete", "response.done"} {
		event := []byte(`{"type":"` + typ + `","response":{"id":"r"}}`)
		if got := gjson.GetBytes(helps.ReportCodexPriorityRoute(event, true), "response.service_tier").String(); got != "priority" {
			t.Fatalf("%s tier=%s", typ, got)
		}
		if got := helps.ReportCodexPriorityRoute(event, false); string(got) != string(event) {
			t.Fatal("unselected attempt rewritten")
		}
	}
}

func TestCodexPriorityWebsocketResponseReporting(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(map[bool]string{false: "json", true: "stream"}[stream], func(t *testing.T) {
			var rec codexWebsocketHintRecorder
			server := newCodexWebsocketHintServer(t, &rec)
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			exec := NewCodexWebsocketsExecutor(&config.Config{})
			req := cliproxyexecutor.Request{Model: "gpt-5.5", Payload: []byte(`{"model":"gpt-5.5","service_tier":"priority","input":"hi"}`)}
			opts := cliproxyexecutor.Options{SourceFormat: sdktranslator.FormatOpenAIResponse, Stream: stream}
			if !stream {
				resp, err := exec.Execute(ctx, codexOAuthTestAuth(server.URL), req, opts)
				if err != nil {
					t.Fatal(err)
				}
				if gjson.GetBytes(resp.Payload, "service_tier").String() != "priority" {
					t.Fatalf("tier not priority: %s", resp.Payload)
				}
			} else {
				resp, err := exec.ExecuteStream(ctx, codexOAuthTestAuth(server.URL), req, opts)
				if err != nil {
					t.Fatal(err)
				}
				found := false
				for chunk := range resp.Chunks {
					if chunk.Err != nil {
						t.Fatal(chunk.Err)
					}
					raw := strings.TrimSpace(strings.TrimPrefix(string(chunk.Payload), "data:"))
					if gjson.Get(raw, "type").String() == "response.completed" {
						found = true
						if gjson.Get(raw, "response.service_tier").String() != "priority" {
							t.Fatalf("tier not priority: %s", raw)
						}
					}
				}
				if !found {
					t.Fatal("no completed event")
				}
			}
			defer exec.CloseExecutionSession("priority-unverified-session")
			for _, tier := range []string{"default", "priority"} {
				req.Payload = []byte(`{"model":"gpt-5.5","service_tier":"` + tier + `","input":"hi"}`)
				opts.Metadata = map[string]any{cliproxyexecutor.ExecutionSessionMetadataKey: "priority-unverified-session"}
				if !stream {
					resp, err := exec.Execute(ctx, codexOAuthTestAuth(server.URL), req, opts)
					if err != nil {
						t.Fatal(err)
					}
					if gjson.GetBytes(resp.Payload, "service_tier").Exists() {
						t.Fatalf("unverified persistent hint fabricated tier: %s", resp.Payload)
					}
				} else {
					resp, err := exec.ExecuteStream(ctx, codexOAuthTestAuth(server.URL), req, opts)
					if err != nil {
						t.Fatal(err)
					}
					for chunk := range resp.Chunks {
						if chunk.Err != nil {
							t.Fatal(chunk.Err)
						}
						raw := strings.TrimSpace(strings.TrimPrefix(string(chunk.Payload), "data:"))
						if gjson.Get(raw, "response.service_tier").Exists() {
							t.Fatalf("unverified persistent hint fabricated tier: %s", raw)
						}
					}
				}
			}
		})
	}
}
