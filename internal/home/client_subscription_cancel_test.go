package home

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func newCancellationTestClient(t *testing.T, subscribe string) *Client {
	t.Helper()
	c, _ := newRedisCommandTestClient(t, func(a []string) string {
		switch strings.ToUpper(a[0]) {
		case "HELLO":
			return "%6\r\n$6\r\nserver\r\n$5\r\nredis\r\n$5\r\nproto\r\n:3\r\n$2\r\nid\r\n:1\r\n$4\r\nmode\r\n$10\r\nstandalone\r\n$4\r\nrole\r\n$6\r\nmaster\r\n$7\r\nmodules\r\n*0\r\n"
		case "GET":
			return "$15\r\nhost: 127.0.0.1\r\n"
		case "SUBSCRIBE":
			return subscribe
		case "LPUSH":
			return ":1\r\n"
		default:
			return "+OK\r\n"
		}
	})
	c.SetManagedLifetime(true)
	c.mu.Lock()
	c.testOperationTimeout = 10 * time.Second
	c.mu.Unlock()
	return c
}

const cancellationACK = "*3\r\n$9\r\nsubscribe\r\n$6\r\nconfig\r\n:1\r\n"

func TestSubscriptionCancellationPreservesDrainTransport(t *testing.T) {
	c := newCancellationTestClient(t, cancellationACK)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- c.RunConfigSubscriberLifetime(ctx, func([]byte) error { return nil }, func() { close(ready) })
	}()
	select {
	case <-ready:
	case <-time.After(3 * time.Second):
		c.Close()
		<-done
		t.Fatal("subscription not ready")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(300 * time.Millisecond):
		c.Close()
		<-done
		t.Fatal("canceled subscription still blocked")
	}
	drainCtx, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	if _, err := c.GetConfig(drainCtx); err != nil {
		t.Fatalf("command transport closed before drain: %v", err)
	}
	if err := c.PushConcurrencyRelease(drainCtx, ConcurrencyReleaseFrame{CredentialID: "fixture", Model: "fixture", ReleaseSeq: 1}); err != nil {
		t.Fatalf("release transport closed before drain: %v", err)
	}
}

func TestSubscriptionCanceledReadyDoesNotApplyQueuedConfig(t *testing.T) {
	c := newCancellationTestClient(t, cancellationACK+"*3\r\n$7\r\nmessage\r\n$6\r\nconfig\r\n$6\r\nqueued\r\n")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var calls atomic.Int32
	done := make(chan error, 1)
	go func() {
		done <- c.RunConfigSubscriberLifetime(ctx, func([]byte) error { calls.Add(1); return nil }, cancel)
	}()
	select {
	case <-done:
	case <-time.After(300 * time.Millisecond):
		c.Close()
		<-done
		t.Fatal("canceled queued lifetime blocked")
	}
	if calls.Load() != 1 {
		t.Fatalf("callbacks = %d, want initial config only", calls.Load())
	}
}

func TestSubscriptionPayloadCanceledScopeDoesNotApply(t *testing.T) {
	c := newCancellationTestClient(t, cancellationACK)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	err := c.handleSubscriptionPayload(ctx, redisChannelConfig, "queued", func([]byte) error { called = true; return nil })
	if called || !errors.Is(err, context.Canceled) {
		t.Fatal("canceled scope applied queued config")
	}
}
