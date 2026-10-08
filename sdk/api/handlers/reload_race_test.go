package handlers

import (
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/config"
)

func TestReloadConcurrentRequestConfig(t *testing.T) {
	h := NewBaseAPIHandlers(&config.SDKConfig{}, nil)
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < 10000; i++ {
			h.UpdateClients(&config.SDKConfig{})
			h.SetPluginHost(nil)
			h.SetModelRouterHost(nil)
		}
	}()
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < 10000; i++ {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			h.StartNonStreamingKeepAlive(c, nil)()
			h.interceptorHost()
			h.modelRouterHost()
			h.pluginExecutorHost()
		}
	}()
	close(start)
	wg.Wait()
}
