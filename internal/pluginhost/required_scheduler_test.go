package pluginhost

import (
	"context"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
	"testing"
)

func TestRequiredSchedulerPanicAndFuseFailClosed(t *testing.T) {
	host := newHostWithRecords(capabilityRecord{id: "required", plugin: pluginapi.Plugin{Capabilities: pluginapi.Capabilities{SchedulerPreference: true, Scheduler: schedulerFunc(func(context.Context, pluginapi.SchedulerPickRequest) (pluginapi.SchedulerPickResponse, error) {
		panic("fixture")
	})}}})
	for i := 0; i < 2; i++ {
		if !host.SchedulerWantsPreference() {
			t.Fatalf("required capability disappeared after fuse at %d", i)
		}
		r, handled, err := host.PickAuth(context.Background(), schedulerRequest("auth-1"))
		if err == nil && (!handled || !r.Handled || !r.Reject) {
			t.Fatalf("required failure delegated at %d: %+v handled=%v err=%v", i, r, handled, err)
		}
	}
}

func TestRequiredSchedulerUnhandledFailsClosed(t *testing.T) {
	host := newHostWithRecords(capabilityRecord{id: "required", plugin: pluginapi.Plugin{Capabilities: pluginapi.Capabilities{SchedulerPreference: true, Scheduler: schedulerFunc(func(context.Context, pluginapi.SchedulerPickRequest) (pluginapi.SchedulerPickResponse, error) {
		return pluginapi.SchedulerPickResponse{}, nil
	})}}})
	r, handled, err := host.PickAuth(context.Background(), schedulerRequest("auth-1"))
	if err == nil && (!handled || !r.Handled || !r.Reject) {
		t.Fatalf("required unhandled delegated: %+v handled=%v err=%v", r, handled, err)
	}
}
