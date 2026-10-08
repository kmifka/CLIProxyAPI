package handlers

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/config"
	"sync"
	"testing"
)

func TestConcurrentConfigReloadAndRequestReads(t *testing.T) {
	h := NewBaseAPIHandlers(&config.SDKConfig{}, nil)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			h.UpdateClients(&config.SDKConfig{RequestLog: i%2 == 0})
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			c, _ := gin.CreateTestContext(nil)
			_, cancel := h.GetContextWithCancel(nil, c, context.Background())
			cancel()
			stop := h.StartNonStreamingKeepAlive(c, context.Background())
			stop()
		}
	}()
	wg.Wait()
}
