package home

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
)

// Exercise two independent initializers on the SAME command client: the pooled
// logging path and the dedicated connection used by native credential dispatch.
// Neither can finish HELLO until both have reached the server.
func TestHomeConcurrentFreshConnectionInitialization(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var handlers sync.WaitGroup
	var helloCount, maintenanceCount atomic.Int32
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	var releaseOnce sync.Once
	acceptDone := make(chan struct{})
	go func() {
		defer close(acceptDone)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			handlers.Add(1)
			go func() {
				defer handlers.Done()
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
				reader := bufio.NewReader(conn)
				for {
					args, err := readRedisCommand(reader)
					if err != nil {
						return
					}
					reply := "+OK\r\n"
					switch strings.ToUpper(args[0]) {
					case "HELLO":
						if helloCount.Add(1) <= 2 {
							entered <- struct{}{}
							select {
							case <-release:
							case <-ctx.Done():
								return
							}
						}
						reply = "%6\r\n$6\r\nserver\r\n$5\r\nredis\r\n$5\r\nproto\r\n:3\r\n$2\r\nid\r\n:1\r\n$4\r\nmode\r\n$10\r\nstandalone\r\n$4\r\nrole\r\n$6\r\nmaster\r\n$7\r\nmodules\r\n*0\r\n"
					case "CLIENT":
						if len(args) > 1 && strings.EqualFold(args[1], "MAINT_NOTIFICATIONS") {
							maintenanceCount.Add(1)
						}
					case "RPUSH":
						reply = ":1\r\n"
					case "PING":
						reply = "+PONG\r\n"
					case "RPOP":
						token := `{"access_token":"fresh-token"}`
						reply = fmt.Sprintf("$%d\r\n%s\r\n", len(token), token)
					}
					if _, err := io.WriteString(conn, reply); err != nil {
						return
					}
				}
			}()
		}
	}()
	client := New(config.HomeConfig{Enabled: true, Host: "127.0.0.1", Port: listener.Addr().(*net.TCPAddr).Port, DisableClusterDiscovery: true})
	results := make(chan error, 2)
	t.Cleanup(func() {
		cancel()
		releaseOnce.Do(func() { close(release) })
		client.Close()
		listener.Close()
		<-acceptDone
		handlers.Wait()
	})
	go func() { results <- client.RPushAppLog(ctx, []byte(`{"message":"fixture"}`)) }()
	go func() {
		raw, err := client.RPopAuth(ctx, "fixture-model", "", http.Header{}, 1)
		if err == nil && !strings.Contains(string(raw), "fresh-token") {
			err = fmt.Errorf("stale or missing credential: %s", raw)
		}
		results <- err
	}()
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-ctx.Done():
			t.Fatal("two independent initializers did not reach HELLO")
		}
	}
	releaseOnce.Do(func() { close(release) })
	for i := 0; i < 2; i++ {
		select {
		case err := <-results:
			if err != nil {
				t.Fatal(err)
			}
		case <-ctx.Done():
			t.Fatal("initialization deadlocked")
		}
	}
	if maintenanceCount.Load() != 0 {
		t.Fatalf("Home attempted unsupported Redis maintenance negotiation %d times", maintenanceCount.Load())
	}
}
