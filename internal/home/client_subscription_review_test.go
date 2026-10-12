package home

import (
	"bufio"
	"context"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
)

type reviewSubscription struct {
	conn   net.Conn
	closed chan struct{}
}

// All handlers are joined. Only a loopback ephemeral listener is used.
func reviewClient(t *testing.T, subscribe func(net.Conn, int)) (*Client, <-chan reviewSubscription) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	events := make(chan reviewSubscription, 8)
	var count atomic.Int32
	var handlers sync.WaitGroup
	var mu sync.Mutex
	var connections []net.Conn
	accepted := make(chan struct{})
	go func() {
		defer close(accepted)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			connections = append(connections, conn)
			mu.Unlock()
			handlers.Add(1)
			go func() {
				defer handlers.Done()
				defer conn.Close()
				closed := make(chan struct{})
				defer close(closed)
				reader := bufio.NewReader(conn)
				for {
					args, err := readRedisCommand(reader)
					if err != nil {
						return
					}
					response := "+OK\r\n"
					switch strings.ToUpper(args[0]) {
					case "HELLO":
						response = "%6\r\n$6\r\nserver\r\n$5\r\nredis\r\n$5\r\nproto\r\n:3\r\n$2\r\nid\r\n:1\r\n$4\r\nmode\r\n$10\r\nstandalone\r\n$4\r\nrole\r\n$6\r\nmaster\r\n$7\r\nmodules\r\n*0\r\n"
					case "GET":
						response = "$15\r\nhost: 127.0.0.1\r\n"
					case "PING":
						response = "+PONG\r\n"
					case "LPUSH":
						response = ":1\r\n"
					case "SUBSCRIBE":
						events <- reviewSubscription{conn, closed}
						subscribe(conn, int(count.Add(1)))
						continue
					}
					if _, err := io.WriteString(conn, response); err != nil {
						return
					}
				}
			}()
		}
	}()
	host, portText, _ := net.SplitHostPort(listener.Addr().String())
	port, _ := strconv.Atoi(portText)
	c := New(config.HomeConfig{Enabled: true, Host: host, Port: port, DisableClusterDiscovery: true})
	c.SetManagedLifetime(true)
	c.testOperationTimeout = 10 * time.Second
	t.Cleanup(func() {
		c.Close()
		listener.Close()
		<-accepted
		mu.Lock()
		for _, conn := range connections {
			conn.Close()
		}
		mu.Unlock()
		handlers.Wait()
	})
	return c, events
}

func reviewWait[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(time.Second):
		t.Fatal("deadline: lifetime/socket did not complete")
		var zero T
		return zero
	}
}

func TestReviewSubscriptionCancelBeforeACK(t *testing.T) {
	c, events := reviewClient(t, func(net.Conn, int) {}) // Home receives SUBSCRIBE but sends no ACK.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- c.RunConfigSubscriberLifetime(ctx, func([]byte) error { return nil }, nil) }()
	old := reviewWait(t, events)
	cancel()
	if err := reviewWait(t, done); err == nil {
		t.Fatal("missing cancellation failure")
	}
	reviewWait(t, old.closed) // Actual peer EOF, not merely goroutine completion.
	if c.HeartbeatOK() {
		t.Fatal("canceled ACK lifetime healthy")
	}
	drain, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	if _, err := c.GetConfig(drain); err != nil {
		t.Fatal(err)
	}
	if err := c.PushConcurrencyRelease(drain, ConcurrencyReleaseFrame{CredentialID: "fixture", Model: "fixture", ReleaseSeq: 1}); err != nil {
		t.Fatal(err)
	}
}

func TestReviewSubscriptionAlreadyCanceled(t *testing.T) {
	c, events := reviewClient(t, func(conn net.Conn, _ int) { io.WriteString(conn, cancellationACK) })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan error, 1)
	var calls atomic.Int32
	go func() {
		done <- c.RunConfigSubscriberLifetime(ctx, func([]byte) error { calls.Add(1); return nil }, nil)
	}()
	if err := reviewWait(t, done); err == nil {
		t.Fatal("canceled lifetime succeeded")
	}
	if calls.Load() != 0 {
		t.Fatal("canceled initial config applied")
	}
	select {
	case <-events:
		t.Fatal("already canceled lifetime subscribed")
	default:
	}
}

func TestReviewSubscriptionCanceledDuringInitialCallback(t *testing.T) {
	c, events := reviewClient(t, func(net.Conn, int) {})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered := make(chan struct{})
	resume := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- c.RunConfigSubscriberLifetime(ctx, func([]byte) error { close(entered); <-resume; return nil }, nil)
	}()
	reviewWait(t, entered)
	cancel()
	close(resume)
	if err := reviewWait(t, done); err == nil {
		t.Fatal("canceled handshake succeeded")
	}
	select {
	case old := <-events:
		reviewWait(t, old.closed)
	default:
	}
}

func TestReviewSubscriptionReplacementOwnsDistinctSocket(t *testing.T) {
	c, events := reviewClient(t, func(conn net.Conn, _ int) { io.WriteString(conn, cancellationACK) })
	firstCtx, firstCancel := context.WithCancel(context.Background())
	defer firstCancel()
	firstDone := make(chan error, 1)
	firstReady := make(chan struct{})
	go func() {
		firstDone <- c.RunConfigSubscriberLifetime(firstCtx, func([]byte) error { return nil }, func() { close(firstReady) })
	}()
	first := reviewWait(t, events)
	reviewWait(t, firstReady)
	firstCancel()
	reviewWait(t, firstDone)
	reviewWait(t, first.closed)
	next := c.NewLifetime()
	next.SetManagedLifetime(true)
	t.Cleanup(next.Close)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	ready := make(chan struct{})
	payload := make(chan string, 1)
	go func() {
		done <- next.RunConfigSubscriberLifetime(ctx, func(raw []byte) error {
			if string(raw) == "fresh" {
				payload <- string(raw)
			}
			return nil
		}, func() { close(ready) })
	}()
	replacement := reviewWait(t, events)
	reviewWait(t, ready)
	if replacement.conn == first.conn {
		t.Fatal("replacement reused old socket")
	}
	firstCancel()
	c.Close() // Old cancellation and owner cleanup must not close replacement.
	io.WriteString(replacement.conn, "*3\r\n$7\r\nmessage\r\n$6\r\nconfig\r\n$5\r\nfresh\r\n")
	if reviewWait(t, payload) != "fresh" {
		t.Fatal("replacement callback missing")
	}
	cancel()
	reviewWait(t, done)
	reviewWait(t, replacement.closed)
}

func TestReviewSubscriptionQueuedHeartbeatAndConfigAfterCancel(t *testing.T) {
	c, _ := reviewClient(t, func(conn net.Conn, _ int) {
		io.WriteString(conn, cancellationACK+"*2\r\n$4\r\npong\r\n$0\r\n\r\n*3\r\n$7\r\nmessage\r\n$6\r\nconfig\r\n$6\r\nqueued\r\n")
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var calls atomic.Int32
	done := make(chan error, 1)
	go func() {
		done <- c.RunConfigSubscriberLifetime(ctx, func([]byte) error { calls.Add(1); return nil }, cancel)
	}()
	reviewWait(t, done)
	if calls.Load() != 1 || c.HeartbeatOK() {
		t.Fatalf("stale queue applied: calls=%d healthy=%v", calls.Load(), c.HeartbeatOK())
	}
}
