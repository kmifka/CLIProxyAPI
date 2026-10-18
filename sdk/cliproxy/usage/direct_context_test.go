package usage

import (
	"context"
	"testing"
	"time"
)

// A string named gin is not an HTTP ownership marker. Preserve caller identity,
// arbitrary private typed values, live/dead deadlines and cancellation channels.
func TestDirectSDKContextIdentityAndDeadline(t *testing.T) {
	for _, expired := range []bool{false, true} {
		t.Run(map[bool]string{false: "live", true: "expired"}[expired], func(t *testing.T) {
			deadline := time.Now().Add(time.Hour)
			if expired {
				deadline = time.Now().Add(-time.Hour)
			}
			parent := context.WithValue(context.Background(), independentKey{}, new(int))
			parent = context.WithValue(parent, "gin", new(int))
			ctx, cancel := context.WithDeadline(parent, deadline)
			defer cancel()
			m := NewManager(1)
			defer m.Stop()
			done := make(chan struct{})
			m.Register(independentCallback(func(c context.Context, r Record) {
				defer close(done)
				if c != ctx {
					t.Error("SDK caller context identity changed")
				}
				if c.Value(independentKey{}) != ctx.Value(independentKey{}) {
					t.Error("private value changed")
				}
				if c.Done() != ctx.Done() || c.Err() != ctx.Err() {
					t.Error("cancellation changed")
				}
				got, ok := c.Deadline()
				if !ok || !got.Equal(deadline) {
					t.Error("deadline changed")
				}
			}))
			m.Publish(ctx, Record{})
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("callback not delivered")
			}
		})
	}
}
