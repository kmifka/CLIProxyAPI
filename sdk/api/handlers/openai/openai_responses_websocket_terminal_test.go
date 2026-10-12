package openai

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/interfaces"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/api/handlers"
	sdkconfig "github.com/router-for-me/CLIProxyAPI/v8/sdk/config"
)

// Both terminal channels are ready before the forwarder starts, without sleeps.
func TestForwardResponsesWebsocketPendingTerminalError(t *testing.T) {
	for _, duplex := range []bool{false, true} {
		t.Run(map[bool]string{false: "response", true: "duplex"}[duplex], func(t *testing.T) {
			done := make(chan struct{})
			expected := &interfaces.ErrorMessage{StatusCode: http.StatusUpgradeRequired, Error: responsesWebsocketHTTPReplayRequiredError()}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(done)
				conn, err := responsesWebsocketUpgrader.Upgrade(w, r, nil)
				if err != nil {
					t.Error(err)
					return
				}
				defer conn.Close()
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = r
				data := make(chan []byte)
				close(data)
				errs := make(chan *interfaces.ErrorMessage, 1)
				errs <- expected
				close(errs)
				h := NewOpenAIResponsesAPIHandler(handlers.NewBaseAPIHandlers(&sdkconfig.SDKConfig{}, nil))
				_, _, _, got, err := h.forwardResponsesWebsocket(c, newResponsesWebsocketWriter(conn), func(...interface{}) {}, data, errs, nil, "terminal-test", responsesWebsocketForwardOptions{duplexStream: func() bool { return duplex }})
				if got != expected {
					t.Errorf("terminal error = %v, want original %v", got, expected)
				}
				if !errors.Is(err, websocket.ErrCloseSent) {
					t.Errorf("forward error = %v, want ErrCloseSent", err)
				}
			}))
			defer server.Close()
			conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			_, payload, err := conn.ReadMessage()
			var closed *websocket.CloseError
			if !errors.As(err, &closed) || closed.Code != websocket.CloseServiceRestart || closed.Text != wsHTTPReplayRequiredCloseReason {
				t.Errorf("close = %v, payload=%s; want exact 1012 replay reason, never 1006", err, payload)
			}
			<-done
		})
	}
}

func TestForwardResponsesWebsocketCleanEOF(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/", nil)
	data := make(chan []byte, 1)
	data <- []byte(`{"type":"response.completed","response":{"id":"clean","output":[]}}`)
	close(data)
	errs := make(chan *interfaces.ErrorMessage)
	close(errs)
	// A closed writer is avoided by a completion suppressed by the existing test writer contract:
	serverDone := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := responsesWebsocketUpgrader.Upgrade(w, r, nil)
		if err != nil {
			serverDone <- err
			return
		}
		defer conn.Close()
		c.Request = r
		_, id, _, msg, err := (*OpenAIResponsesAPIHandler)(nil).forwardResponsesWebsocket(c, newResponsesWebsocketWriter(conn), func(...interface{}) {}, data, errs, nil, "clean")
		if msg != nil || id != "clean" {
			err = errors.New("clean EOF became terminal failure")
		}
		serverDone <- err
	}))
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, _, err = conn.ReadMessage(); err != nil {
		t.Fatal(err)
	}
	if err = <-serverDone; err != nil {
		t.Fatal(err)
	}
}

func TestForwardResponsesWebsocketCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/", nil).WithContext(ctx)
	data := make(chan []byte)
	close(data)
	errs := make(chan *interfaces.ErrorMessage, 1)
	errs <- &interfaces.ErrorMessage{StatusCode: 426, Error: responsesWebsocketHTTPReplayRequiredError()}
	close(errs)
	calls := 0
	h := NewOpenAIResponsesAPIHandler(handlers.NewBaseAPIHandlers(&sdkconfig.SDKConfig{}, nil))
	_, _, _, msg, err := h.forwardResponsesWebsocket(c, nil, func(...interface{}) { calls++ }, data, errs, nil, "cancel")
	if !errors.Is(err, context.Canceled) || msg != nil || calls != 1 {
		t.Fatalf("cancel = %v, msg=%v, calls=%d; want canceled without replay", err, msg, calls)
	}
}
