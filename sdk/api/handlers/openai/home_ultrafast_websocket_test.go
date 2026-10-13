package openai

import (
	"maps"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	internalconfig "github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/registry"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/api/handlers"
	coreauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executionregistry"
	coreexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
	sdkconfig "github.com/router-for-me/CLIProxyAPI/v8/sdk/config"
	"github.com/tidwall/gjson"
)

func TestResponsesWebsocketExplicitUltrafastRetainedLeaseDenial(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dispatcher := &homeResponsesWebsocketDispatcher{}
	executor := &homeResponsesWebsocketExecutor{}
	manager := coreauth.NewManager(nil, nil, nil)
	manager.SetConfig(&internalconfig.Config{Home: internalconfig.HomeConfig{Enabled: true}})
	manager.PublishHomeDispatch(dispatcher, executionregistry.New(), 1)
	manager.RegisterExecutor(executor)
	registry.GetGlobalRegistry().RegisterClient("home-responses-websocket-auth", "codex", []*registry.ModelInfo{{ID: "gpt-5.4"}})
	defer registry.GetGlobalRegistry().UnregisterClient("home-responses-websocket-auth")
	h := NewOpenAIResponsesAPIHandler(handlers.NewBaseAPIHandlers(&sdkconfig.SDKConfig{}, manager))
	router := gin.New()
	router.GET("/v1/responses", h.ResponsesWebsocket)
	server := httptest.NewServer(router)
	defer server.Close()
	conn, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/v1/responses", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if resp.StatusCode != 101 {
		t.Fatalf("handshake=%d", resp.StatusCode)
	}
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	ordinary := []byte(`{"type":"response.create","model":"gpt-5.4","input":[{"type":"message","role":"user","content":"synthetic only"}]}`)
	if err = conn.WriteMessage(websocket.TextMessage, ordinary); err != nil {
		t.Fatal(err)
	}
	_, payload, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if gjson.GetBytes(payload, "type").String() != "response.completed" {
		t.Fatalf("ordinary=%s", payload)
	}
	executor.mu.Lock()
	metadata := maps.Clone(executor.metadata[0])
	executor.mu.Unlock()
	sessionID, _ := metadata[coreexecutor.ExecutionSessionMetadataKey].(string)
	if _, ok := manager.GetExecutionSessionAuthByID(sessionID, "home-responses-websocket-auth"); !ok {
		t.Fatal("lease was not retained")
	}
	denied := []byte(`{"type":"response.create","model":"gpt-5.4","input":[{"type":"message","role":"user","content":"synthetic only"}],"service_tier":"ultrafast"}`)
	if err = conn.WriteMessage(websocket.TextMessage, denied); err != nil {
		t.Fatal(err)
	}
	_, payload, err = conn.ReadMessage()
	if err != nil {
		t.Fatalf("missing structured membership denial: %v", err)
	}
	if gjson.GetBytes(payload, "type").String() != "error" || gjson.GetBytes(payload, "status").Int() != 503 || !strings.Contains(string(payload), "tier membership requires fresh Home dispatch") {
		t.Fatalf("denial=%s", payload)
	}
	if executor.calls.Load() != 1 || dispatcher.calls.Load() != 1 {
		t.Fatalf("denial executed/replayed: executor=%d dispatch=%d", executor.calls.Load(), dispatcher.calls.Load())
	}
}
