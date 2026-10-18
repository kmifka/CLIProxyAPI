package usage

import (
 "context"
 "testing"
 "time"
)

type independentKey struct{}
type independentCallback func(context.Context, Record)
func (f independentCallback) HandleUsage(ctx context.Context, r Record) { f(ctx,r) }

func TestIndependentDirectSDKContextCompatibility(t *testing.T) {
 m:=NewManager(1);defer m.Stop()
 ctx,cancel:=context.WithCancel(context.WithValue(context.Background(),independentKey{},"caller-correlation"))
 cancel()
 done:=make(chan struct{})
 m.Register(independentCallback(func(c context.Context,r Record){
  defer close(done)
  if got:=c.Value(independentKey{});got!="caller-correlation" {t.Errorf("caller context.Value lost: got %v",got)}
  if got:=c.Err();got!=context.Canceled {t.Errorf("caller cancellation changed: got %v",got)}
 }))
 m.Publish(ctx,Record{RequestID:"direct-execution"})
 select {case <-done:case <-time.After(5*time.Second):t.Fatal("callback not delivered")}
}
