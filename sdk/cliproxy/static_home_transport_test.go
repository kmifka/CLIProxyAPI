package cliproxy

import (
	"bufio"
	"context"
	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/config"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"

	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const staticPayload = "request-retry: 7\ncredential-concurrency:\n  lifecycle-config-revision: 1\n  cpa-heartbeat-timeout: 2s\n  cpa-cancel-bound: 100ms\n"

func TestStaticHomeTransportRejectReconnectAndCredentialDispatch(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	updates := make(chan struct{}, 4)
	drop := make(chan struct{}, 4)
	stop := make(chan struct{})
	var changed atomic.Bool
	var subscriptions, pluginGets, dispatches atomic.Int32
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			conn, e := listener.Accept()
			if e != nil {
				return
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer conn.Close()
				reader := bufio.NewReader(conn)
				for {
					args, e := readRegistryTestRedisCommand(reader)
					if e != nil {
						return
					}
					switch strings.ToUpper(args[0]) {
					case "HELLO":
						io.WriteString(conn, "%6\r\n$6\r\nserver\r\n$5\r\nredis\r\n$5\r\nproto\r\n:3\r\n$2\r\nid\r\n:1\r\n$4\r\nmode\r\n$10\r\nstandalone\r\n$4\r\nrole\r\n$6\r\nmaster\r\n$7\r\nmodules\r\n*0\r\n")
					case "GET":
						switch args[1] {
						case "config":
							payload := staticPayload
							if changed.Load() {
								payload = strings.Replace(payload, "revision: 1", "revision: 2", 1)
							}
							writeRegistryTestConfig(conn, payload)
						case "plugin-tasks":
							pluginGets.Add(1)
							writeRegistryTestConfig(conn, "[]")
						default:
							io.WriteString(conn, "$-1\r\n")
						}
					case "PING":
						io.WriteString(conn, "+PONG\r\n")
					case "RPOP":
						dispatches.Add(1)
						writeRegistryTestConfig(conn, `{"access_token":"dynamic-token"}`)
					case "SUBSCRIBE":
						subscriptions.Add(1)
						io.WriteString(conn, "*3\r\n$9\r\nsubscribe\r\n$6\r\nconfig\r\n:1\r\n")
						ticker := time.NewTicker(50 * time.Millisecond)
						defer ticker.Stop()
						for {
							select {
							case <-updates:
								writeRegistryTestMessage(conn, strings.Replace(staticPayload, "revision: 1", "revision: 2", 1))
							case <-drop:
								return
							case <-ticker.C:
								if _, e := io.WriteString(conn, "*2\r\n$4\r\npong\r\n$0\r\n\r\n"); e != nil {
									return
								}
							case <-stop:
								return
							}
						}
					default:
						io.WriteString(conn, "+OK\r\n")
					}
				}
			}()
		}
	}()
	t.Cleanup(func() { close(stop); listener.Close(); wg.Wait() })
	cfg, _ := config.ParseConfigBytes([]byte(staticPayload))
	cfg.Home.Enabled = true
	cfg.Home.Host = "127.0.0.1"
	cfg.Home.Port = listener.Addr().(*net.TCPAddr).Port
	cfg.Home.DisableClusterDiscovery = true
	rejected := make(chan StaticHomeRejection, 16)
	s, err := NewBuilder().WithStaticHomeConfig(cfg, func(r StaticHomeRejection) { rejected <- r }).WithConfigPath(filepath.Join(t.TempDir(), "config.yaml")).Build()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	defer s.Shutdown(context.Background())
	if err = s.validateInitialStaticHome(ctx); err != nil {
		t.Fatal(err)
	}
	s.startHomeSubscriber(ctx)
	first := waitForServiceRegistry(t, s, 3*time.Second)
	s.homeMu.Lock()
	client := s.homeClient
	s.homeMu.Unlock()
	before := pluginGets.Load()
	updates <- struct{}{}
	select {
	case r := <-rejected:
		if r.LifecycleConfigRevision != 2 {
			t.Fatal(r)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("missing rejection")
	}
	if !client.HeartbeatOK() {
		t.Fatal("rejection killed heartbeat")
	}
	raw, err := client.RPopAuth(ctx, "test-model", "", http.Header{}, 1)
	if err != nil || !strings.Contains(string(raw), "dynamic-token") || dispatches.Load() != 1 {
		t.Fatalf("dispatch: %s %v", raw, err)
	}
	if pluginGets.Load() != before {
		t.Fatal("rejected config reached plugin work")
	}
	s.cfgMu.RLock()
	revision := s.cfg.CredentialConcurrency.LifecycleConfigRevision
	s.cfgMu.RUnlock()
	if revision != 1 {
		t.Fatal("revision mutated")
	}
	changed.Store(true)
	drop <- struct{}{}
	select {
	case <-rejected:
	case <-time.After(3 * time.Second):
		t.Fatal("reconnect repinned config")
	}
	deadline := time.Now().Add(3 * time.Second)
	for subscriptions.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if subscriptions.Load() < 2 {
		t.Fatal("reconnect failed")
	}
	next := waitForServiceRegistry(t, s, 3*time.Second)
	_ = first
	_ = next
	s.homeMu.Lock()
	replacementClient := s.homeClient
	s.homeMu.Unlock()
	if replacementClient == client || replacementClient.LimiterConfig().LifecycleConfigRevision != 1 {
		t.Fatal("reconnect lifecycle was not initialized from pin")
	}
	raw, err = replacementClient.RPopAuth(ctx, "test-model", "", http.Header{}, 1)
	if err != nil || !strings.Contains(string(raw), "dynamic-token") {
		t.Fatalf("reconnected dispatch: %s %v", raw, err)
	}
	s.cfgMu.RLock()
	revision = s.cfg.CredentialConcurrency.LifecycleConfigRevision
	s.cfgMu.RUnlock()
	if revision != 1 {
		t.Fatal("reconnect changed revision")
	}
}

func TestStaticHomeInitialMismatchBeforeRunMutation(t *testing.T) {
	// Reuse the native GET transport fixture with a different supplied config.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			conn, e := listener.Accept()
			if e != nil {
				return
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer conn.Close()
				reader := bufio.NewReader(conn)
				for {
					args, e := readRegistryTestRedisCommand(reader)
					if e != nil {
						return
					}
					if strings.EqualFold(args[0], "GET") {
						writeRegistryTestConfig(conn, "request-retry: 8\n")
					} else if strings.EqualFold(args[0], "HELLO") {
						io.WriteString(conn, "-ERR unknown command\r\n")
					} else {
						io.WriteString(conn, "+OK\r\n")
					}
				}
			}()
		}
	}()
	defer func() { close(stop); listener.Close(); wg.Wait() }()
	cfg, _ := config.ParseConfigBytes([]byte("request-retry: 7\n"))
	cfg.Home.Enabled = true
	cfg.Home.Host = "127.0.0.1"
	cfg.Home.Port = listener.Addr().(*net.TCPAddr).Port
	cfg.Home.DisableClusterDiscovery = true
	s, err := NewBuilder().WithStaticHomeConfig(cfg, nil).WithConfigPath(filepath.Join(t.TempDir(), "c.yaml")).Build()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err = s.Run(ctx); err == nil || !strings.Contains(err.Error(), "static Home configuration rejected") {
		t.Fatalf("Run error: %v", err)
	}
	if s.server != nil || s.runCancel != nil || s.homeSupervisor != nil {
		t.Fatal("initial mismatch mutated Run state")
	}
}

func TestStaticHomeManagementFailClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(StaticHomeReadOnlyManagement)
	var writes int
	for _, method := range []string{"GET", "PUT", "PATCH", "POST", "DELETE"} {
		router.Handle(method, "/v0/management/future-config", func(c *gin.Context) { writes++; c.Status(200) })
	}
	for _, method := range []string{"PUT", "PATCH", "POST", "DELETE"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(method, "/v0/management/future-config", nil))
		if w.Code != 403 {
			t.Fatal(method, w.Code)
		}
	}
	if writes != 0 {
		t.Fatal("write reached handler")
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/v0/management/future-config", nil))
	if w.Code != 200 {
		t.Fatal(w.Code)
	}

}
